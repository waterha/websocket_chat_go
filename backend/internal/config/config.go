package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	HTTPAddr         string
	Origin           string
	SecureCookie     bool
	Postgres         *pgxpool.Config
	RedisAddr        string
	RedisPassword    string
	MigrationsDir    string
	MediaDir         string
	Heartbeat        time.Duration
	HeartbeatTimeout time.Duration
}

func Load() (Config, error) {
	for _, filename := range []string{".env", filepath.Join("..", ".env")} {
		if _, err := os.Stat(filename); err == nil {
			if err = loadEnv(filename); err != nil {
				return Config{}, err
			}
			break
		}
	}
	if os.Getenv("PGPASSWORD") == "" {
		if password := os.Getenv("POSTGRES_PASSWORD"); password != "" {
			if err := os.Setenv("PGPASSWORD", password); err != nil {
				return Config{}, err
			}
		}
	}
	postgres, err := pgxpool.ParseConfig("")
	if err != nil {
		return Config{}, err
	}
	postgres.ConnConfig.Host = value("PGHOST", "127.0.0.1")
	port, err := strconv.ParseUint(value("PGPORT", "5433"), 10, 16)
	if err != nil {
		return Config{}, fmt.Errorf("PGPORT: %w", err)
	}
	postgres.ConnConfig.Port = uint16(port)
	postgres.ConnConfig.User = value("PGUSER", "websocket_chat_go")
	postgres.ConnConfig.Database = value("PGDATABASE", "websocket_chat_go")
	if postgres.ConnConfig.Password == "" {
		return Config{}, fmt.Errorf("set POSTGRES_PASSWORD or PGPASSWORD")
	}
	postgres.ConnConfig.ConnectTimeout = 5 * time.Second
	postgres.MaxConns = 16
	directory := value("MIGRATIONS_DIR", filepath.Join("..", "database", "migrations"))
	if _, err := os.Stat(directory); err != nil && os.Getenv("MIGRATIONS_DIR") == "" {
		directory = filepath.Join("database", "migrations")
	}
	mediaDirectory := value("MEDIA_DIR", filepath.Join("..", "media"))
	if _, err := os.Stat(filepath.Dir(mediaDirectory)); err != nil && os.Getenv("MEDIA_DIR") == "" {
		mediaDirectory = "media"
	}
	return Config{HTTPAddr: value("HTTP_ADDR", "127.0.0.1:8080"), Origin: value("FRONTEND_ORIGIN", "http://127.0.0.1:4173"), SecureCookie: os.Getenv("APP_ENV") == "production", Postgres: postgres, RedisAddr: value("REDIS_ADDR", "127.0.0.1:6379"), RedisPassword: os.Getenv("REDIS_PASSWORD"), MigrationsDir: directory, MediaDir: mediaDirectory, Heartbeat: 20 * time.Second, HeartbeatTimeout: 8 * time.Second}, nil
}

func loadEnv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, content, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid environment entry in %s", filename)
		}
		key = strings.TrimSpace(key)
		content = strings.TrimSpace(content)
		if len(content) >= 2 && (content[0] == '"' && content[len(content)-1] == '"' || content[0] == '\'' && content[len(content)-1] == '\'') {
			content = content[1 : len(content)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, content); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func value(key, fallback string) string {
	if content := os.Getenv(key); content != "" {
		return content
	}
	return fallback
}
