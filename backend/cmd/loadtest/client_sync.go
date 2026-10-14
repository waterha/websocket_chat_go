package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"websocket_chat_go/backend/internal/model"
)

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
