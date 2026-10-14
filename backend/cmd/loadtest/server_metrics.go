package main

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"time"
	"websocket_chat_go/backend/internal/svc"
)

type verification struct {
	Online        int   `json:"online_connections"`
	Members       int   `json:"members"`
	LastSeq       int64 `json:"last_seq"`
	Confirmed     int   `json:"confirmed_members"`
	PendingOutbox int   `json:"pending_outbox"`
	ReadMembers   int   `json:"read_members"`
	MinReceived   int64 `json:"min_received_seq"`
	MaxReceived   int64 `json:"max_received_seq"`
}

func verify(ctx context.Context, services *svc.ServiceContext, group string) (verification, error) {
	result := verification{Online: len(services.Hub.Clients())}
	err := services.Store.DB.QueryRow(ctx, `SELECT c.next_seq,count(*),count(*) FILTER (WHERE COALESCE(dc.received_seq,0)=c.next_seq),
		COALESCE(min(dc.received_seq),0),COALESCE(max(dc.received_seq),0),count(*) FILTER (WHERE cm.last_read_seq>0),
		(SELECT count(*) FROM message_outbox WHERE conversation_id=$1 AND published_at IS NULL)
		FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id
		LEFT JOIN device_cursors dc ON dc.conversation_id=c.id AND dc.user_id=cm.user_id AND dc.device_id='load'
		WHERE c.id=$1 GROUP BY c.id`, group).Scan(&result.LastSeq, &result.Members, &result.Confirmed, &result.MinReceived, &result.MaxReceived, &result.ReadMembers, &result.PendingOutbox)
	return result, err
}

func sample(ctx context.Context, services *svc.ServiceContext, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			pool := services.Store.DB.Stat()
			err = encoder.Encode(map[string]any{
				"time": time.Now().UTC(), "online": len(services.Hub.Clients()), "heap_bytes": memory.HeapAlloc,
				"go_sys_bytes": memory.Sys, "goroutines": runtime.NumGoroutine(), "gc_count": memory.NumGC,
				"pool_acquired": pool.AcquiredConns(), "pool_max": pool.MaxConns(),
				"pool_empty_acquire_count": pool.EmptyAcquireCount(), "pool_acquire_duration_ms": float64(pool.AcquireDuration()) / float64(time.Millisecond),
			})
			if err != nil {
				return err
			}
		}
	}
}
