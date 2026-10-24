package localsvc

import (
	"testing"
	"time"
)

func TestSettingsBuildsExplicitConfiguration(t *testing.T) {
	options := Options{
		HTTPAddr: "127.0.0.1:8080", Origin: "http://127.0.0.1:4173",
		PostgresHost: "127.0.0.1", PostgresPort: 5434,
		PostgresUser: "websocket_chat_go", PostgresDatabase: "websocket_chat_go",
		PostgresPassword: "secret", RedisAddr: "127.0.0.1:6380",
		MigrationsDir: "migrations", MediaDir: "media",
	}
	settings, err := options.settings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.HTTPAddr != options.HTTPAddr || settings.Origin != options.Origin {
		t.Fatalf("unexpected HTTP settings: %+v", settings)
	}
	if settings.SecureCookie {
		t.Fatal("the local server must not mark cookies secure over HTTP")
	}
	if settings.Postgres.ConnConfig.Host != "127.0.0.1" || settings.Postgres.ConnConfig.Port != 5434 {
		t.Fatalf("unexpected database host: %+v", settings.Postgres.ConnConfig)
	}
	if settings.Postgres.ConnConfig.User != "websocket_chat_go" || settings.Postgres.ConnConfig.Password != "secret" {
		t.Fatalf("unexpected database credentials: %+v", settings.Postgres.ConnConfig)
	}
	if settings.RedisAddr != "127.0.0.1:6380" || settings.MigrationsDir != "migrations" || settings.MediaDir != "media" {
		t.Fatalf("unexpected service settings: %+v", settings)
	}
	if settings.Heartbeat != 20*time.Second || settings.HeartbeatTimeout != 8*time.Second {
		t.Fatalf("unexpected heartbeat settings: %+v", settings)
	}
}

func TestParsePortFallsBackToDefault(t *testing.T) {
	if port := ParsePort(""); port != 8080 {
		t.Fatalf("ParsePort(\"\") = %d, want 8080", port)
	}
	if port := ParsePort("9000"); port != 9000 {
		t.Fatalf("ParsePort(9000) = %d, want 9000", port)
	}
	if port := ParsePort("0"); port != 8080 {
		t.Fatalf("ParsePort(0) = %d, want 8080", port)
	}
	if port := ParsePort("not-a-port"); port != 8080 {
		t.Fatalf("ParsePort(not-a-port) = %d, want 8080", port)
	}
}
