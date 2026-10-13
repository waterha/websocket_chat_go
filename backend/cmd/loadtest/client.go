package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"websocket_chat_go/backend/internal/model"
)

type envelope struct {
	Event     string          `json:"event"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Error     *model.Error    `json:"error"`
}

type client struct {
	account   credentials
	fixture   *fixture
	http      *http.Client
	stats     *statistics
	mutex     sync.Mutex
	socket    *websocket.Conn
	pending   map[string]chan envelope
	notify    chan struct{}
	ready     chan struct{}
	received  atomic.Int64
	confirmed atomic.Int64
	target    atomic.Int64
}

func (peer *client) signal(seq int64) {
	for previous := peer.target.Load(); seq > previous; previous = peer.target.Load() {
		if peer.target.CompareAndSwap(previous, seq) {
			break
		}
	}
	select {
	case peer.notify <- struct{}{}:
	default:
	}
}

func (peer *client) request(ctx context.Context, method, path string, input, result any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, peer.fixture.URL+"/api/v1"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Cookie", "session="+peer.account.Token)
	request.Header.Set("Origin", peer.fixture.Origin)
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := peer.http.Do(request)
	if err == nil {
		defer response.Body.Close()
		var body struct {
			Data  json.RawMessage `json:"data"`
			Error *model.Error    `json:"error"`
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		if err == nil && response.StatusCode >= 400 {
			err = fmt.Errorf("%s %s: status %d (%v)", method, path, response.StatusCode, body.Error)
		}
		if err == nil && result != nil {
			err = json.Unmarshal(body.Data, result)
		}
	}
	peer.stats.mutex.Lock()
	peer.stats.httpDurations = append(peer.stats.httpDurations, time.Since(started))
	peer.stats.mutex.Unlock()
	if err != nil && ctx.Err() == nil {
		peer.stats.failure("http", err)
	}
	return err
}

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

func (peer *client) connect(ctx context.Context, gate chan struct{}) {
	first := true
	for ctx.Err() == nil {
		select {
		case gate <- struct{}{}:
		case <-ctx.Done():
			return
		}
		dialContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		connection, response, err := websocket.Dial(dialContext, "ws"+strings.TrimPrefix(peer.fixture.URL, "http")+"/ws?device_id=load", &websocket.DialOptions{HTTPClient: peer.http, HTTPHeader: http.Header{"Origin": {peer.fixture.Origin}, "Cookie": {"session=" + peer.account.Token}}})
		if response != nil && err != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			_, payload, readErr := connection.Read(dialContext)
			var event envelope
			err = readErr
			if err == nil {
				err = json.Unmarshal(payload, &event)
			}
			if err == nil && event.Event != "connection.ready" {
				err = fmt.Errorf("first frame was %q", event.Event)
			}
		}
		cancel()
		<-gate
		if err != nil {
			if connection != nil {
				_ = connection.CloseNow()
			}
			if ctx.Err() == nil {
				peer.stats.dialErrors.Add(1)
				peer.stats.failure("dial", err)
			}
			pause(ctx, time.Second)
			continue
		}
		peer.mutex.Lock()
		peer.socket = connection
		peer.mutex.Unlock()
		peer.stats.connections.Add(1)
		if first {
			close(peer.ready)
			first = false
		} else {
			peer.stats.reconnects.Add(1)
		}
		peer.signal(peer.target.Load())
		peer.read(ctx, connection)
		peer.mutex.Lock()
		peer.socket = nil
		for id, waiter := range peer.pending {
			waiter <- envelope{Error: &model.Error{Code: "connection_closed"}}
			delete(peer.pending, id)
		}
		peer.mutex.Unlock()
		_ = connection.CloseNow()
		peer.stats.connections.Add(-1)
		if ctx.Err() == nil {
			peer.stats.disconnects.Add(1)
			pause(ctx, time.Second)
		}
	}
}

func (peer *client) read(ctx context.Context, connection *websocket.Conn) {
	for {
		_, payload, err := connection.Read(ctx)
		if err != nil {
			return
		}
		var event envelope
		if err = json.Unmarshal(payload, &event); err != nil {
			peer.stats.failure("protocol", err)
			continue
		}
		if event.RequestID != "" {
			peer.mutex.Lock()
			if waiter := peer.pending[event.RequestID]; waiter != nil {
				waiter <- event
				delete(peer.pending, event.RequestID)
			}
			peer.mutex.Unlock()
			continue
		}
		switch event.Event {
		case "message.created":
			var message model.Message
			if err := json.Unmarshal(event.Data, &message); err != nil || message.ConversationID != peer.fixture.Group {
				peer.stats.failure("protocol", fmt.Errorf("invalid message push"))
				continue
			}
			peer.stats.pushes.Add(1)
			peer.stats.pushed(message.ClientMsgID)
			peer.signal(message.Seq)
		case "message.sync_required":
			var body struct {
				Cursors []model.DeviceCursor `json:"cursors"`
			}
			if err := json.Unmarshal(event.Data, &body); err != nil {
				peer.stats.failure("protocol", err)
				continue
			}
			peer.stats.compensations.Add(1)
			for _, cursor := range body.Cursors {
				peer.signal(cursor.LastSeq)
			}
		}
	}
}

func (peer *client) synchronize(ctx context.Context) error {
	for {
		after := peer.received.Load()
		var page model.SyncPage
		if err := peer.request(ctx, "GET", fmt.Sprintf("/conversations/%s/sync?after_seq=%d&limit=100", peer.fixture.Group, after), nil, &page); err != nil {
			return err
		}
		for index, message := range page.Messages {
			if message.ConversationID != peer.fixture.Group || message.Seq != after+int64(index)+1 {
				return fmt.Errorf("non-contiguous or unknown message at seq %d", message.Seq)
			}
		}
		if page.NextSeq != after+int64(len(page.Messages)) || page.HasMore && len(page.Messages) == 0 {
			return fmt.Errorf("invalid sync cursor")
		}
		for _, message := range page.Messages {
			if !peer.stats.delivery(message.ClientMsgID, time.Now()) {
				return fmt.Errorf("unknown message at seq %d", message.Seq)
			}
		}
		peer.received.Store(page.NextSeq)
		if page.NextSeq > peer.confirmed.Load() {
			var cursor model.DeviceCursor
			if err := peer.request(ctx, "POST", "/conversations/"+peer.fixture.Group+"/received", map[string]any{"device_id": "load", "seq": page.NextSeq}, &cursor); err != nil {
				return err
			}
			if cursor.ReceivedSeq != page.NextSeq {
				return fmt.Errorf("invalid receive acknowledgement")
			}
			peer.confirmed.Store(cursor.ReceivedSeq)
		}
		if !page.HasMore && peer.target.Load() <= page.NextSeq {
			return nil
		}
	}
}

func (peer *client) receive(ctx context.Context) {
	select {
	case <-peer.ready:
	case <-ctx.Done():
		return
	}
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-peer.notify:
		case <-timer.C:
			var cursors []model.DeviceCursor
			if err := peer.request(ctx, "GET", "/devices/load/cursors", nil, &cursors); err != nil {
				continue
			}
			for _, cursor := range cursors {
				if cursor.LastSeq > peer.received.Load() || peer.confirmed.Load() < peer.received.Load() {
					peer.signal(cursor.LastSeq)
				}
			}
			continue
		}
		if err := peer.synchronize(ctx); err != nil && ctx.Err() == nil {
			if !strings.Contains(err.Error(), "status") {
				peer.stats.failure("sync", err)
			}
			if pause(ctx, time.Second) {
				peer.signal(peer.target.Load())
			}
		}
	}
}

func (peer *client) send(ctx context.Context, input model.SendInput) error {
	waiter := make(chan envelope, 1)
	peer.mutex.Lock()
	connection := peer.socket
	if connection != nil {
		peer.pending[input.ClientMsgID] = waiter
	}
	peer.mutex.Unlock()
	if connection == nil {
		return fmt.Errorf("sender offline")
	}
	defer func() {
		peer.mutex.Lock()
		delete(peer.pending, input.ClientMsgID)
		peer.mutex.Unlock()
	}()
	request, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	payload, _ := json.Marshal(model.Event{Event: "message.send", RequestID: input.ClientMsgID, Data: input})
	if err := connection.Write(request, websocket.MessageText, payload); err != nil {
		return err
	}
	select {
	case event := <-waiter:
		if event.Error != nil {
			return event.Error
		}
		var message model.Message
		if err := json.Unmarshal(event.Data, &message); err != nil || event.Event != "message.ack" || message.ClientMsgID != input.ClientMsgID {
			return fmt.Errorf("invalid send acknowledgement")
		}
		return nil
	case <-request.Done():
		return request.Err()
	}
}

func administrative(ctx context.Context, transport *http.Client, data *fixture, method, path string, result any) error {
	request, err := http.NewRequestWithContext(ctx, method, data.URL+"/_loadtest/"+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Loadtest-Token", data.AdminToken)
	response, err := transport.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("admin %s: %d %s", path, response.StatusCode, body)
	}
	if result != nil {
		return json.NewDecoder(response.Body).Decode(result)
	}
	return nil
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
