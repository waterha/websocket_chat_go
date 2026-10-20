package realtime

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"websocket_chat_go/backend/internal/model"
)

// Broadcast carries a persisted event to connections held by *other* instances.
//
// The outbox dispatcher elects a single drainer through a database advisory
// lock, so exactly one process turns outbox rows into pushes. That drainer only
// knows its own local Hub, which meant that in a multi-instance deployment the
// events reached whichever instance happened to win the lock and no other: an
// audit measured pushes split roughly evenly between two instances with none
// reaching both.
//
// The fix keeps a single drainer (so the database is still read once per event)
// and adds one Redis publish per event to a fan-out channel. Every instance
// subscribes; a subscriber ignores messages that it published itself, because
// the drainer already delivered those locally. Redis failure is non-fatal: the
// event is still delivered locally and other instances recover through
// /sync reconciliation, which is the existing safety net.
type Broadcast struct {
	// Publisher is used by the drainer.
	Publisher *redis.Client
	// Subscriber must be a separate client: a subscribed go-redis client holds a
	// dedicated connection for the whole subscription.
	Subscriber *redis.Client
	Hub        *Hub
	// InstanceID is included in every payload so a subscriber can skip its own
	// publications and avoid delivering an event twice.
	InstanceID string
	// Channel is the fan-out channel name.
	Channel string
}

const DefaultBroadcastChannel = "im:events"

type broadcastPayload struct {
	Origin     string      `json:"origin"`
	Recipients []string    `json:"recipients"`
	Event      model.Event `json:"event"`
}

func (broadcast *Broadcast) channel() string {
	if broadcast.Channel == "" {
		return DefaultBroadcastChannel
	}
	return broadcast.Channel
}

// enabled reports whether cross-instance fan-out is available. Without Redis the
// service degrades to single-instance behaviour rather than failing.
func (broadcast *Broadcast) enabled() bool {
	return broadcast != nil && broadcast.Publisher != nil && broadcast.Subscriber != nil
}

// Publish sends an event to every instance except this one. It is called after
// the local Hub has already been given the event.
func (broadcast *Broadcast) Publish(ctx context.Context, recipients []string, event model.Event) error {
	if !broadcast.enabled() || len(recipients) == 0 {
		return nil
	}
	payload, err := json.Marshal(broadcastPayload{Origin: broadcast.InstanceID, Recipients: recipients, Event: event})
	if err != nil {
		return err
	}
	return broadcast.Publisher.Publish(ctx, broadcast.channel(), payload).Err()
}

// Run subscribes until the context is cancelled and delivers remote events to
// this instance's local clients.
func (broadcast *Broadcast) Run(ctx context.Context) {
	if !broadcast.enabled() {
		return
	}
	subscription := broadcast.Subscriber.Subscribe(ctx, broadcast.channel())
	defer subscription.Close()
	// Confirm the subscription so a misconfiguration is visible in logs instead
	// of silently dropping events.
	if _, err := subscription.Receive(ctx); err != nil {
		if ctx.Err() == nil {
			log.Printf("event subscription failed: %v", err)
		}
		return
	}
	log.Printf("cross-instance event fan-out ready on %s (instance %s)", broadcast.channel(), broadcast.InstanceID)
	for {
		message, err := subscription.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("event subscription receive: %v", err)
			// Back off briefly so a persistent Redis error does not spin.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		var payload broadcastPayload
		if err := json.Unmarshal([]byte(message.Payload), &payload); err != nil {
			log.Printf("event subscription payload: %v", err)
			continue
		}
		// The publisher already delivered locally, so delivering again here would
		// duplicate the frame on that instance's connections.
		if payload.Origin == broadcast.InstanceID {
			continue
		}
		broadcast.Hub.Publish(payload.Recipients, payload.Event)
	}
}
