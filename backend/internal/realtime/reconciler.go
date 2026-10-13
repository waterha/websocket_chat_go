package realtime

import (
	"context"
	"log"
	"time"

	"websocket_chat_go/backend/internal/model"
)

type Reconciler struct {
	Store *model.Store
	Hub   *Hub
}

func (reconciler *Reconciler) Run(ctx context.Context) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			attempt, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := reconciler.Retry(attempt)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("delivery reconciliation: %v", err)
			}
		}
	}
}

func (reconciler *Reconciler) Retry(ctx context.Context) error {
	clients := reconciler.Hub.Clients()
	for offset := 0; offset < len(clients); offset += 100 {
		batch := clients[offset:min(offset+100, len(clients))]
		users, devices := make([]string, len(batch)), make([]string, len(batch))
		for index, client := range batch {
			users[index], devices[index] = client.UserID, client.DeviceID
		}
		rows, err := reconciler.Store.DB.Query(ctx, `SELECT online.position,cm.conversation_id,c.next_seq,COALESCE(dc.received_seq,0)
			FROM unnest($1::text[],$2::text[]) WITH ORDINALITY AS online(user_id,device_id,position)
			JOIN conversation_members cm ON cm.user_id=online.user_id
			JOIN conversations c ON c.id=cm.conversation_id
			LEFT JOIN device_cursors dc ON dc.user_id=online.user_id AND dc.device_id=online.device_id AND dc.conversation_id=cm.conversation_id
			WHERE c.next_seq>COALESCE(dc.received_seq,0) ORDER BY online.position,cm.conversation_id`, users, devices)
		if err != nil {
			return err
		}
		hints := make([][]model.DeviceCursor, len(batch))
		for rows.Next() {
			var position int
			var cursor model.DeviceCursor
			if err = rows.Scan(&position, &cursor.ConversationID, &cursor.LastSeq, &cursor.ReceivedSeq); err != nil {
				rows.Close()
				return err
			}
			cursor.DeviceID = batch[position-1].DeviceID
			hints[position-1] = append(hints[position-1], cursor)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for index, cursors := range hints {
			for offset := 0; offset < len(cursors); offset += 100 {
				if !batch[index].Reply(model.Event{Event: "message.sync_required", Data: map[string]any{"cursors": cursors[offset:min(offset+100, len(cursors))]}}) {
					break
				}
			}
		}
	}
	return nil
}
