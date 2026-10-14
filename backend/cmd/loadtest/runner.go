package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"websocket_chat_go/backend/internal/model"
)

func pause(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(max(0, duration))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func load(parent context.Context, settings options, rates []int) error {
	payload, err := os.ReadFile(settings.fixture)
	if err != nil {
		return err
	}
	var data fixture
	if err = json.Unmarshal(payload, &data); err != nil || len(data.Accounts) != settings.members {
		return fmt.Errorf("fixture must match member count")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dialGate := make(chan struct{}, 50)
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{MaxIdleConns: 3000, MaxIdleConnsPerHost: 2000, IdleConnTimeout: 60 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			select {
			case dialGate <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			defer func() { <-dialGate }()
			return dialer.DialContext(ctx, network, address)
		},
	}
	httpClient := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	stats := &statistics{messages: make(map[string]sentMessage)}
	peers := make([]*client, settings.members)
	gate := make(chan struct{}, 50)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
		shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := administrative(shutdown, httpClient, &data, "POST", "stop", nil); err != nil {
			log.Printf("test server shutdown: %v", err)
		}
		transport.CloseIdleConnections()
	}()
	started := time.Now()
	for index, account := range data.Accounts {
		peer := &client{account: account, fixture: &data, http: httpClient, stats: stats, pending: make(map[string]chan envelope), notify: make(chan struct{}, 1), ready: make(chan struct{})}
		peers[index] = peer
		workers.Add(2)
		go func() { defer workers.Done(); peer.connect(ctx, gate) }()
		go func() { defer workers.Done(); peer.receive(ctx) }()
	}
	for deadline := time.Now().Add(90 * time.Second); stats.connections.Load() != int64(settings.members); {
		if time.Now().After(deadline) || !pause(ctx, 100*time.Millisecond) {
			return fmt.Errorf("could not establish all %d real sockets; online=%d", settings.members, stats.connections.Load())
		}
	}
	rampSeconds := time.Since(started).Seconds()
	log.Printf("%d real sockets online in %.2fs; idle baseline %s", settings.members, rampSeconds, settings.idle)
	baseline := &stage{StartedAt: time.Now().UTC(), MinOnline: stats.connections.Load()}
	stats.stages = append(stats.stages, baseline)
	for end := time.Now().Add(settings.idle); time.Now().Before(end); {
		baseline.MinOnline = min(baseline.MinOnline, stats.connections.Load())
		if !pause(ctx, min(time.Second, time.Until(end))) {
			return ctx.Err()
		}
	}
	baseline.FinishedAt = time.Now().UTC()
	for _, rate := range rates {
		current := &stage{Rate: rate, StartedAt: time.Now().UTC(), MinOnline: stats.connections.Load()}
		stats.stages = append(stats.stages, current)
		log.Printf("stage: %d group messages/s for %s (%d recipients/message)", rate, settings.duration, settings.members)
		var sends sync.WaitGroup
		attempts := int(settings.duration.Seconds() * float64(rate))
		for index := 0; index < attempts; index++ {
			if !pause(ctx, time.Until(current.StartedAt.Add(time.Duration(index)*time.Second/time.Duration(rate)))) {
				return ctx.Err()
			}
			current.MinOnline = min(current.MinOnline, stats.connections.Load())
			id := model.NewID()
			begin := time.Now()
			stats.mutex.Lock()
			stats.messages[id] = sentMessage{started: begin, stage: current}
			current.Attempts++
			stats.mutex.Unlock()
			peer := peers[index%len(peers)]
			sends.Add(1)
			go func() {
				defer sends.Done()
				err := peer.send(ctx, model.SendInput{ConversationID: data.Group, ClientMsgID: id, Content: strings.Repeat("x", 128)})
				stats.mutex.Lock()
				if err == nil {
					current.Sent++
					current.sends = append(current.sends, time.Since(begin))
				} else {
					current.SendErrors++
				}
				stats.mutex.Unlock()
				if err != nil {
					stats.failure("send", err)
				}
			}()
		}
		pause(ctx, time.Until(current.StartedAt.Add(settings.duration)))
		sends.Wait()
		current.FinishedAt = time.Now().UTC()
	}
	var final verification
	verificationError := ""
	drainStarted := time.Now()
	for {
		if err = administrative(ctx, httpClient, &data, "GET", "verify", &final); err != nil {
			verificationError = err.Error()
			stats.failure("verification", err)
			break
		}
		if final.Confirmed == settings.members && final.Online == settings.members && final.PendingOutbox == 0 {
			break
		}
		if time.Since(drainStarted) >= settings.drain || !pause(ctx, time.Second) {
			break
		}
	}
	cancel()
	workers.Wait()
	stats.mutex.Lock()
	totalSent, sendErrors, deliveries := 0, 0, 0
	for _, current := range stats.stages {
		current.SendLatency = summarize(current.sends)
		current.PushLatency = summarize(current.pushFrames)
		current.ReceiveLatency = summarize(current.receives)
		totalSent += current.Sent
		sendErrors += current.SendErrors
		deliveries += current.Deliveries
	}
	passed := verificationError == "" && final.PendingOutbox == 0 && final.Online == settings.members && final.Confirmed == settings.members && final.LastSeq == int64(totalSent) && deliveries == totalSent*settings.members && final.ReadMembers == 0 && sendErrors == 0 && stats.protocolErrors == 0 && stats.syncErrors == 0 && stats.disconnects.Load() == 0 && stats.dialErrors.Load() == 0 && stats.httpErrors == 0
	result := map[string]any{
		"passed": passed, "started_at": started.UTC(), "finished_at": time.Now().UTC(), "go_version": runtime.Version(),
		"members": settings.members, "ramp_seconds": rampSeconds, "drain_seconds": time.Since(drainStarted).Seconds(), "stages": stats.stages,
		"payload_bytes": 128, "profile": "real WS send/push + HTTP ordered sync/receive ACK + 5s cursor polling; no DOM/read receipts/metadata refresh/TLS/login hashing",
		"tcp_dial_concurrency": 50,
		"server":               final, "total_acknowledged_sends": totalSent, "total_processed_deliveries": deliveries,
		"verification_error": verificationError,
		"push_frames":        stats.pushes.Load(), "compensation_frames": stats.compensations.Load(),
		"disconnects": stats.disconnects.Load(), "reconnects": stats.reconnects.Load(), "dial_errors": stats.dialErrors.Load(),
		"http_errors": stats.httpErrors, "protocol_errors": stats.protocolErrors, "http_latency": summarize(stats.httpDurations), "error_samples": stats.errorSamples,
		"sync_errors": stats.syncErrors,
	}
	stats.mutex.Unlock()
	if err = writeJSON(settings.output, result); err != nil {
		return err
	}
	log.Printf("result: passed=%v, online=%d, confirmed=%d, persisted=%d, deliveries=%d, disconnects=%d; %s", passed, final.Online, final.Confirmed, final.LastSeq, deliveries, stats.disconnects.Load(), settings.output)
	if !passed {
		return fmt.Errorf("load criteria failed; inspect result JSON")
	}
	return nil
}
