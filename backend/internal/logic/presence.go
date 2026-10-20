package logic

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Presence reads the short-lived connection markers that the WebSocket write
// loop refreshes. The markers exist so "who is online right now" can be
// answered without scanning process memory, which would be wrong as soon as a
// second process served a connection.
//
// The answer is deliberately narrow: it reports whether at least one live
// connection exists per user within the last heartbeat window. It is not a
// delivery guarantee and not an audit trail, and an expired marker is reported
// as offline.
type Presence struct{ Redis *redis.Client }

const presencePrefix = "im:presence:"

// Online reports which of the given users currently have at least one live
// connection. Unknown or expired users are absent from the result.
func (presence *Presence) Online(ctx context.Context, userIDs []string) (map[string]bool, error) {
	online := map[string]bool{}
	if presence == nil || presence.Redis == nil || len(userIDs) == 0 {
		return online, nil
	}
	// A single SCAN per user prefix is cheaper than reading every device key,
	// because the device count per user is bounded at eight.
	for _, userID := range dedupeNonEmpty(userIDs) {
		pattern := presencePrefix + userID + ":*"
		keys, err := presence.scan(ctx, pattern)
		if err != nil {
			return online, err
		}
		online[userID] = len(keys) > 0
	}
	return online, nil
}

func (presence *Presence) scan(ctx context.Context, pattern string) ([]string, error) {
	keys := []string{}
	cursor := uint64(0)
	for {
		batch, next, err := presence.Redis.Scan(ctx, cursor, pattern, 64).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 || len(keys) > 0 {
			// The device count is capped at eight, so one non-empty page is
			// already enough to answer "is this user online".
			break
		}
	}
	return keys, nil
}

// Count reports how many of the given users are online, for a compact summary.
func (presence *Presence) Count(ctx context.Context, userIDs []string) (int, error) {
	online, err := presence.Online(ctx, userIDs)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, isOnline := range online {
		if isOnline {
			count++
		}
	}
	return count, nil
}

func dedupeNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	unique := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
