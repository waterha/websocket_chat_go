package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type options struct {
	mode, address, fixture, output, metrics, migrations string
	members                                             int
	idle, duration, drain                               time.Duration
	rates                                               string
}

type credentials struct {
	UserID string `json:"user_id"`
	Token  string `json:"token"`
}

type fixture struct {
	URL, Origin, Group, AdminToken string
	Accounts                       []credentials
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0600)
}

func parseRates(value string) ([]int, error) {
	rates := []int{}
	for _, part := range strings.Split(value, ",") {
		rate, err := strconv.Atoi(part)
		if err != nil || rate < 1 || rate > 1000 {
			return nil, fmt.Errorf("rates must contain integers from 1 to 1000")
		}
		rates = append(rates, rate)
	}
	return rates, nil
}

func run() error {
	var settings options
	flag.StringVar(&settings.mode, "mode", "client", "server or client")
	flag.StringVar(&settings.address, "address", "127.0.0.1:18080", "loopback test server address")
	flag.StringVar(&settings.fixture, "fixture", "", "temporary credential file; never commit this file")
	flag.StringVar(&settings.output, "output", "", "client JSON result file")
	flag.StringVar(&settings.metrics, "metrics", "", "server JSONL runtime metrics file")
	flag.StringVar(&settings.migrations, "migrations", "../database/migrations", "migration directory")
	flag.IntVar(&settings.members, "members", 1000, "group members and real WebSocket connections")
	flag.DurationVar(&settings.idle, "idle", 30*time.Second, "connected idle baseline")
	flag.DurationVar(&settings.duration, "duration", 20*time.Second, "duration of each sending stage")
	flag.DurationVar(&settings.drain, "drain", 60*time.Second, "final recovery deadline")
	flag.StringVar(&settings.rates, "rates", "1,5,10", "group messages per second in successive stages")
	flag.Parse()
	if settings.fixture == "" || settings.members < 2 || settings.members > 1000 || settings.idle < 0 || settings.duration < time.Second || settings.drain < time.Second {
		return fmt.Errorf("require -fixture, 2..1000 members and positive stage/drain durations")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if settings.mode == "server" {
		return serve(ctx, settings)
	}
	if settings.mode != "client" || settings.output == "" {
		return fmt.Errorf("client requires -output; mode must be server or client")
	}
	rates, err := parseRates(settings.rates)
	if err != nil {
		return err
	}
	return load(ctx, settings, rates)
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
