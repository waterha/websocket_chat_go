package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func TestWebsocketHTTPRoutingReconnectAndLogout(t *testing.T) {
	services := fixture(t, true)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	charlie, charlieToken := account(t, services, "charlie")
	outsider, outsiderToken := account(t, services, "outsider")
	_ = outsider
	owner := dial(t, server, aliceToken, "desktop")
	handshake, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/ws?device_id=desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	handshake.Header.Set("Cookie", "session="+aliceToken)
	handshake.Header.Set("Origin", "http://im.test")
	response, err := server.Client().Do(handshake)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("invalid handshake returned %d", response.StatusCode)
	}
	commandTo(t, owner, "ping", "still-connected", nil)
	await(t, owner, "pong", "still-connected")
	otherDevice := dial(t, server, aliceToken, "phone")
	receiver := dial(t, server, bobToken, "desktop")
	groupMember := dial(t, server, charlieToken, "desktop")
	nonmember := dial(t, server, outsiderToken, "desktop")
	direct, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	input := model.SendInput{ConversationID: direct, ClientMsgID: "ws-stable-id", Content: "你好"}
	commandTo(t, owner, "message.send", "send-1", input)
	ack := await(t, owner, "message.ack", "send-1")
	var persisted model.Message
	if json.Unmarshal(ack.Data, &persisted) != nil || persisted.Seq != 1 {
		t.Fatal("bad send ACK")
	}
	page, err := services.Chat.Sync(t.Context(), bob.ID, direct, 0, 100)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != persisted.ID {
		t.Fatal("ACK preceded persistence")
	}
	for _, client := range []*peer{receiver, otherDevice} {
		event := await(t, client, "message.created", "")
		var pushed model.Message
		_ = json.Unmarshal(event.Data, &pushed)
		if pushed.ID != persisted.ID {
			t.Fatal("wrong routed message")
		}
	}
	noMessages(t, groupMember)
	noMessages(t, nonmember)
	commandTo(t, owner, "message.send", "retry", input)
	retry := await(t, owner, "message.ack", "retry")
	var same model.Message
	_ = json.Unmarshal(retry.Data, &same)
	if same.ID != persisted.ID {
		t.Fatal("WS retry duplicated")
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/messages", map[string]any{"client_msg_id": input.ClientMsgID, "content": input.Content}, 200)
	commandTo(t, receiver, "message.received", "received", map[string]any{"conversation_id": direct, "seq": 1})
	await(t, receiver, "message.received.ack", "received")
	commandTo(t, receiver, "conversation.read", "read", map[string]any{"conversation_id": direct, "seq": 1})
	await(t, receiver, "conversation.read.ack", "read")
	read := await(t, owner, "conversation.read", "")
	var receipt model.Receipt
	_ = json.Unmarshal(read.Data, &receipt)
	if receipt.UserID != bob.ID || receipt.Seq != 1 {
		t.Fatal("bad read fanout")
	}
	commandTo(t, nonmember, "message.send", "denied", input)
	if await(t, nonmember, "error", "denied").Error.Code != "not_found" {
		t.Fatal("WS permission bypass")
	}
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, charlie.ID}})
	if err != nil {
		t.Fatal(err)
	}
	input.ConversationID = group
	input.ClientMsgID = "ws-group-id"
	commandTo(t, owner, "message.send", "group", input)
	await(t, owner, "message.ack", "group")
	for _, client := range []*peer{receiver, otherDevice, groupMember} {
		await(t, client, "message.created", "")
	}
	noMessages(t, nonmember)
	replacement := dial(t, server, bobToken, "desktop")
	select {
	case <-receiver.done:
	case <-time.After(2 * time.Second):
		t.Fatal("same device not replaced")
	}
	_ = replacement.connection.CloseNow()
	select {
	case <-replacement.done:
	case <-time.After(time.Second):
		t.Fatal("failed disconnect")
	}
	input.ConversationID = direct
	input.ClientMsgID = "ws-offline-id"
	commandTo(t, owner, "message.send", "offline", input)
	await(t, owner, "message.ack", "offline")
	reconnected := dial(t, server, bobToken, "desktop")
	data := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+direct+"/sync?after_seq=1", nil, 200)
	if json.Unmarshal(data, &page) != nil || len(page.Messages) != 1 || page.Messages[0].Seq != 2 {
		t.Fatal("offline message lost")
	}
	commandTo(t, reconnected, "ping", "heartbeat", nil)
	await(t, reconnected, "pong", "heartbeat")
	httpCall(t, server, bobToken, "POST", "/api/v1/auth/logout", nil, 200)
	select {
	case <-reconnected.done:
	case <-time.After(2 * time.Second):
		t.Fatal("logout left socket active")
	}
	httpCall(t, server, bobToken, "GET", "/api/v1/me", nil, 401)
	httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+direct+"/sync?after_seq=-1", nil, 400)
	_, response, err = websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?device_id=evil", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://evil.test"}, "Cookie": {"session=" + aliceToken}}})
	if err == nil || response.StatusCode != 403 {
		t.Fatal("invalid websocket Origin accepted")
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if _, err := services.Store.DB.Exec(t.Context(), "UPDATE sessions SET expires_at=$1 WHERE token_hash=$2", time.Now().Add(-time.Second), model.TokenHash(charlieToken)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-groupMember.done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat did not expire invalid session")
	}
}

func TestStrictEventValidation(t *testing.T) {
	for _, payload := range []string{`{"event":"ping","request_id":"one","extra":true}`, `{"event":"ping"} {}`, `[]`} {
		var input command
		err := parseJSON([]byte(payload), &input)
		if err == nil {
			t.Fatal("accepted invalid envelope", payload)
		}
	}
	for _, device := range []string{"", "../device", "中文", strings.Repeat("x", 101)} {
		if logic.ValidDeviceID(device) {
			t.Fatal("invalid device", device)
		}
	}
	if _, err := logic.ValidateSend(model.SendInput{ConversationID: "chat", ClientMsgID: "valid-id", Content: "  "}); err == nil {
		t.Fatal("empty message accepted")
	}
}
