package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"websocket_chat_go/backend/internal/model"
)

func TestSyncProcessesOnceAndConfirmsSeparately(t *testing.T) {
	group := "group"
	current := &stage{}
	stats := &statistics{messages: map[string]sentMessage{"message-one": {started: time.Now(), stage: current}}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var data any
		if request.Method == "POST" {
			data = model.DeviceCursor{ReceivedSeq: 1}
		} else if request.URL.Query().Get("after_seq") == "0" {
			data = model.SyncPage{Messages: []model.Message{{ConversationID: group, ClientMsgID: "message-one", Seq: 1}}, NextSeq: 1, LastSeq: 1}
		} else {
			data = model.SyncPage{Messages: []model.Message{}, NextSeq: 1, LastSeq: 1}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	peer := &client{fixture: &fixture{URL: server.URL, Group: group}, http: server.Client(), stats: stats}
	peer.target.Store(1)
	for attempt := 0; attempt < 2; attempt++ {
		if err := peer.synchronize(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if peer.received.Load() != 1 || peer.confirmed.Load() != 1 || current.Deliveries != 1 || len(stats.httpDurations) != 3 {
		t.Fatal("duplicate delivery or missing receive confirmation")
	}
}

func TestSyncRejectsGapsAndInvalidPageCursor(t *testing.T) {
	for _, page := range []model.SyncPage{
		{Messages: []model.Message{{ConversationID: "group", ClientMsgID: "known", Seq: 2}}, NextSeq: 2},
		{Messages: []model.Message{{ConversationID: "group", ClientMsgID: "known", Seq: 1}}, NextSeq: 2},
		{Messages: []model.Message{}, NextSeq: 0, HasMore: true},
	} {
		current := &stage{}
		stats := &statistics{messages: map[string]sentMessage{"known": {started: time.Now(), stage: current}}}
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": page})
		}))
		peer := &client{fixture: &fixture{URL: server.URL, Group: "group"}, http: server.Client(), stats: stats}
		if err := peer.synchronize(context.Background()); err == nil || peer.received.Load() != 0 || peer.confirmed.Load() != 0 || current.Deliveries != 0 {
			t.Fatal("invalid page advanced a delivery cursor")
		}
		server.Close()
	}
}
