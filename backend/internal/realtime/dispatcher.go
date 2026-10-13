package realtime

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"websocket_chat_go/backend/internal/model"
)

type Dispatcher struct {
	Store *model.Store
	Hub   *Hub
}

func (dispatcher *Dispatcher) Run(ctx context.Context) {
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := dispatcher.Drain(ctx); err != nil && ctx.Err() == nil {
				log.Printf("outbox dispatch: %v", err)
			}
		}
	}
}

func (dispatcher *Dispatcher) Drain(ctx context.Context) error {
	tx, err := dispatcher.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner bool
	if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(73182402)").Scan(&owner); err != nil || !owner {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT id,recipients,payload FROM message_outbox WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE")
	if err != nil {
		return err
	}
	type delivery struct {
		id         int64
		recipients []string
		event      model.Event
	}
	deliveries := []delivery{}
	for rows.Next() {
		var item delivery
		var payload []byte
		if err = rows.Scan(&item.id, &item.recipients, &payload); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(payload, &item.event); err != nil {
			rows.Close()
			return err
		}
		deliveries = append(deliveries, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range deliveries {
		dispatcher.Hub.Publish(item.recipients, item.event)
		if _, err = tx.Exec(ctx, "UPDATE message_outbox SET published_at=now() WHERE id=$1", item.id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
