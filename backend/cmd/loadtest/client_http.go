package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"websocket_chat_go/backend/internal/model"
)

func (peer *client) request(ctx context.Context, method, path string, input, result any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, peer.fixture.URL+"/api/v1"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Cookie", "session="+peer.account.Token)
	request.Header.Set("Origin", peer.fixture.Origin)
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := peer.http.Do(request)
	if err == nil {
		defer response.Body.Close()
		var body struct {
			Data  json.RawMessage `json:"data"`
			Error *model.Error    `json:"error"`
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		if err == nil && response.StatusCode >= 400 {
			err = fmt.Errorf("%s %s: status %d (%v)", method, path, response.StatusCode, body.Error)
		}
		if err == nil && result != nil {
			err = json.Unmarshal(body.Data, result)
		}
	}
	peer.stats.mutex.Lock()
	peer.stats.httpDurations = append(peer.stats.httpDurations, time.Since(started))
	peer.stats.mutex.Unlock()
	if err != nil && ctx.Err() == nil {
		peer.stats.failure("http", err)
	}
	return err
}

func administrative(ctx context.Context, transport *http.Client, data *fixture, method, path string, result any) error {
	request, err := http.NewRequestWithContext(ctx, method, data.URL+"/_loadtest/"+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Loadtest-Token", data.AdminToken)
	response, err := transport.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("admin %s: %d %s", path, response.StatusCode, body)
	}
	if result != nil {
		return json.NewDecoder(response.Body).Decode(result)
	}
	return nil
}
