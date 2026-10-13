package main

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type distribution struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
	Max   float64 `json:"max_ms"`
}

func summarize(values []time.Duration) distribution {
	if len(values) == 0 {
		return distribution{}
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(first, second int) bool { return ordered[first] < ordered[second] })
	percentile := func(fraction float64) float64 {
		return float64(ordered[int(math.Ceil(float64(len(ordered))*fraction))-1]) / float64(time.Millisecond)
	}
	return distribution{Count: len(ordered), P50: percentile(0.5), P95: percentile(0.95), P99: percentile(0.99), Max: percentile(1)}
}

type stage struct {
	Rate           int          `json:"target_messages_per_second"`
	StartedAt      time.Time    `json:"started_at"`
	FinishedAt     time.Time    `json:"finished_at"`
	Attempts       int          `json:"attempts"`
	Sent           int          `json:"acknowledged_sends"`
	SendErrors     int          `json:"send_errors"`
	Deliveries     int          `json:"processed_deliveries"`
	MinOnline      int64        `json:"min_online"`
	SendLatency    distribution `json:"send_ack_latency"`
	PushLatency    distribution `json:"push_frame_latency"`
	ReceiveLatency distribution `json:"receive_latency"`
	sends          []time.Duration
	pushFrames     []time.Duration
	receives       []time.Duration
}

type sentMessage struct {
	started time.Time
	stage   *stage
}

type statistics struct {
	mutex          sync.Mutex
	messages       map[string]sentMessage
	stages         []*stage
	httpDurations  []time.Duration
	httpErrors     int
	protocolErrors int
	syncErrors     int
	errorSamples   []string
	connections    atomic.Int64
	disconnects    atomic.Int64
	reconnects     atomic.Int64
	dialErrors     atomic.Int64
	pushes         atomic.Int64
	compensations  atomic.Int64
}

func (stats *statistics) failure(kind string, err error) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	if kind == "http" {
		stats.httpErrors++
	} else if kind == "protocol" {
		stats.protocolErrors++
	} else if kind == "sync" {
		stats.syncErrors++
	}
	if len(stats.errorSamples) < 12 {
		stats.errorSamples = append(stats.errorSamples, time.Now().UTC().Format(time.RFC3339Nano)+" "+kind+": "+err.Error())
	}
}

func (stats *statistics) delivery(id string, received time.Time) bool {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	message, exists := stats.messages[id]
	if exists {
		message.stage.Deliveries++
		message.stage.receives = append(message.stage.receives, received.Sub(message.started))
	}
	return exists
}

func (stats *statistics) pushed(id string) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	if message, exists := stats.messages[id]; exists {
		message.stage.pushFrames = append(message.stage.pushFrames, time.Since(message.started))
	}
}
