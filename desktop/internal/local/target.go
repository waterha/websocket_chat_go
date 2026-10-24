// Package local starts the whole chat stack for the desktop launcher: a
// private PostgreSQL cluster, a portable Redis and the chat server itself,
// which runs in-process through the backend's localsvc package. Services the
// launcher did not start are reused when their ports already answer.
package local

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"websocket_chat_go/desktop/internal/client"
)

// Target is the loopback chat server the launcher can manage.
type Target struct {
	Host string
	Port int
}

func (target Target) BaseURL() string { return fmt.Sprintf("http://%s:%d", target.Host, target.Port) }

func (target Target) Address() string { return fmt.Sprintf("127.0.0.1:%d", target.Port) }

// Plan reports the local chat server behind a saved configuration. Remote
// and HTTPS servers are outside the launcher's reach, so the second result is
// false and no service is ever started for them.
func Plan(config client.Config) (Target, bool) {
	parsed, err := url.Parse(config.ServerURL)
	if err != nil || parsed.Scheme != "http" {
		return Target{}, false
	}
	host := parsed.Hostname()
	// Only IPv4 loopback names are accepted: the launcher binds its own chat
	// server to 127.0.0.1, so an IPv6-only name could never reach it.
	if host != "127.0.0.1" && host != "localhost" {
		return Target{}, false
	}
	target := Target{Host: host, Port: 80}
	if value := parsed.Port(); value != "" {
		if _, err := fmt.Sscanf(value, "%d", &target.Port); err != nil || target.Port <= 0 {
			return Target{}, false
		}
	}
	return target, true
}

// healthy reports whether a chat server already answers on the target.
func healthy(ctx context.Context, target Target) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.BaseURL()+"/healthz", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}
