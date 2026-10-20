package realtime

import (
	"context"
	"log"
	"time"

	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
)

// Retention reclaims storage that would otherwise grow without bound. Two kinds
// of data were never cleaned:
//
//   - published outbox rows. Publishing only means "handed to this process's
//     queue", so a row is disposable once published; devices recover from the
//     messages table, not from the outbox.
//   - the bytes of a withdrawn attachment. A recall hides the content, and the
//     file is now unreachable through the API, so keeping it serves nobody.
//
// Nothing here touches unpublished rows or attachments that are still live.
type Retention struct {
	Store *model.Store
	Files *media.Store
	// OutboxAge is how long a published row is kept before removal.
	OutboxAge time.Duration
	// BatchSize bounds each pass so one run never holds a long lock.
	BatchSize int
}

// Defaults keep a week of published outbox history, which is far longer than any
// device needs to recover through the outbox.
const (
	DefaultOutboxAge  = 7 * 24 * time.Hour
	DefaultBatchSize  = 500
	retentionInterval = 5 * time.Minute
)

func (retention *Retention) Run(ctx context.Context) {
	timer := time.NewTicker(retentionInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			attempt, cancel := context.WithTimeout(ctx, 60*time.Second)
			_, _, err := retention.Sweep(attempt)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("retention sweep: %v", err)
			}
		}
	}
}

func (retention *Retention) age() time.Duration {
	if retention.OutboxAge <= 0 {
		return DefaultOutboxAge
	}
	return retention.OutboxAge
}

func (retention *Retention) batch() int {
	if retention.BatchSize <= 0 {
		return DefaultBatchSize
	}
	return retention.BatchSize
}

// Sweep runs both cleanups. It reports how many rows and files were reclaimed.
func (retention *Retention) Sweep(ctx context.Context) (int64, int64, error) {
	rows, err := retention.purgeOutbox(ctx)
	if err != nil {
		return rows, 0, err
	}
	files, err := retention.purgeRecalledAttachments(ctx)
	return rows, files, err
}

// purgeOutbox removes published rows older than the retention age. Ordered by id
// and bounded, so it makes steady progress without a long transaction.
func (retention *Retention) purgeOutbox(ctx context.Context) (int64, error) {
	tag, err := retention.Store.DB.Exec(ctx, `DELETE FROM message_outbox WHERE id IN (
		SELECT id FROM message_outbox WHERE published_at IS NOT NULL AND published_at < $1
		ORDER BY id LIMIT $2)`, time.Now().Add(-retention.age()), retention.batch())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// purgeRecalledAttachments deletes files whose only referencing message was
// recalled, then marks the rows. The file is removed before the marker, so a
// crash between the two simply retries: removing an absent file is a no-op.
func (retention *Retention) purgeRecalledAttachments(ctx context.Context) (int64, error) {
	rows, err := retention.Store.DB.Query(ctx, `SELECT m.id,m.storage_key FROM message_media m
		WHERE m.bytes_removed_at IS NULL
			AND EXISTS (SELECT 1 FROM messages msg WHERE msg.media_id=m.id AND msg.recalled_at IS NOT NULL)
		ORDER BY m.created_at LIMIT $1`, retention.batch())
	if err != nil {
		return 0, err
	}
	type candidate struct{ id, key string }
	candidates := []candidate{}
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.id, &item.key); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var removed int64
	for _, item := range candidates {
		if err := retention.Files.Remove(item.key); err != nil {
			// Keep the marker unset so the next sweep retries this file.
			log.Printf("retention could not remove media %s: %v", item.id, err)
			continue
		}
		if _, err := retention.Store.DB.Exec(ctx, "UPDATE message_media SET bytes_removed_at=now() WHERE id=$1", item.id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
