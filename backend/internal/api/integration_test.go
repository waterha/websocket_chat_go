package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
	"websocket_chat_go/backend/internal/svc"
)

func fixture(t *testing.T, start bool) *svc.ServiceContext {
	t.Helper()
	databaseURL, redisAddr := os.Getenv("IM_TEST_DATABASE_URL"), os.Getenv("IM_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("set IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR for integration tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "im_test_" + model.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	postgres, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	postgres.ConnConfig.RuntimeParams["search_path"] = schema
	settings := config.Config{Postgres: postgres, RedisAddr: redisAddr, Origin: "http://im.test", MigrationsDir: filepath.Join("..", "..", "..", "database", "migrations"), Heartbeat: 200 * time.Millisecond, HeartbeatTimeout: time.Second}
	services, err := svc.New(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	services.Hub = realtime.NewHub(1024)
	workers, cancel := context.WithCancel(ctx)
	t.Cleanup(func() { cancel(); services.Close() })
	if start {
		services.Start(workers)
	}
	return services
}

func account(t *testing.T, services *svc.ServiceContext, name string) (model.User, string) {
	t.Helper()
	user, token, err := services.Auth.Register(t.Context(), logic.Credentials{Email: name + "@example.test", Name: name, Password: "secure-password"})
	if err != nil {
		t.Fatal(err)
	}
	return user, token
}

func status(t *testing.T, err error, expected int) {
	t.Helper()
	var failure *model.Error
	if !errors.As(err, &failure) || failure.Status != expected {
		t.Fatalf("wanted status %d, got %v", expected, err)
	}
}

func TestMessagingPersistenceOrderingAndCursors(t *testing.T) {
	services := fixture(t, false)
	ctx, chat := t.Context(), services.Chat
	alice, _ := account(t, services, "alice")
	bob, _ := account(t, services, "bob")
	charlie, _ := account(t, services, "charlie")
	outsider, _ := account(t, services, "outsider")
	direct, err := chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	same, err := chat.Create(ctx, bob.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{alice.ID}})
	if err != nil || same != direct {
		t.Fatalf("direct pair not unique: %s %v", same, err)
	}
	group, err := chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, charlie.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan model.Message, 80)
	failures := make(chan error, 80)
	for index := 0; index < 80; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			sender := alice.ID
			if index%2 == 1 {
				sender = bob.ID
			}
			message, _, err := chat.Send(ctx, sender, model.SendInput{ConversationID: group, ClientMsgID: fmt.Sprintf("parallel-%03d", index), Content: "ordered"})
			if err != nil {
				failures <- err
			} else {
				results <- message
			}
		}(index)
	}
	wait.Wait()
	close(failures)
	close(results)
	for err := range failures {
		t.Error(err)
	}
	sequence := []int{}
	for message := range results {
		sequence = append(sequence, int(message.Seq))
	}
	sort.Ints(sequence)
	if len(sequence) != 80 {
		t.Fatalf("missing messages: %d", len(sequence))
	}
	for index, seq := range sequence {
		if seq != index+1 {
			t.Fatalf("sequence gap at %d: %d", index, seq)
		}
	}
	input := model.SendInput{ConversationID: group, ClientMsgID: "same-retry-id", Content: "idempotent"}
	duplicates := make(chan model.Message, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			item, _, err := chat.Send(ctx, alice.ID, input)
			if err != nil {
				t.Error(err)
			} else {
				duplicates <- item
			}
		}()
	}
	wait.Wait()
	close(duplicates)
	var stable string
	for item := range duplicates {
		if stable == "" {
			stable = item.ID
		}
		if item.ID != stable || item.Seq != 81 {
			t.Fatal("duplicate send inserted twice")
		}
	}
	input.Content = "different payload"
	_, _, err = chat.Send(ctx, alice.ID, input)
	status(t, err, 409)
	input.Content = "idempotent"
	input.ConversationID = direct
	_, _, err = chat.Send(ctx, alice.ID, input)
	status(t, err, 409)
	for index := 0; index < 125; index++ {
		_, _, err := chat.Send(ctx, alice.ID, model.SendInput{ConversationID: group, ClientMsgID: fmt.Sprintf("offline-%03d", index), Content: "offline"})
		if err != nil {
			t.Fatal(err)
		}
	}
	var after int64
	pages := 0
	for {
		page, err := chat.Sync(ctx, bob.ID, group, after, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Messages {
			if item.Seq != after+1 {
				t.Fatal("roaming gap")
			}
			after = item.Seq
		}
		if page.NextSeq != after || page.LastSeq != 206 {
			t.Fatal("bad sync cursor", page)
		}
		pages++
		if !page.HasMore {
			break
		}
	}
	if after != 206 || pages != 3 {
		t.Fatalf("offline backlog truncated: %d / %d", after, pages)
	}
	history, err := chat.History(ctx, bob.ID, group, 50, 10)
	if err != nil || len(history) != 10 || history[0].Seq != 40 || history[9].Seq != 49 {
		t.Fatalf("bad history: %v %v", history, err)
	}
	_, _, err = chat.Send(ctx, outsider.ID, model.SendInput{ConversationID: group, ClientMsgID: "forbidden-id", Content: "forbidden"})
	status(t, err, 404)
	_, err = chat.Sync(ctx, outsider.ID, group, 0, 100)
	status(t, err, 404)
	_, err = chat.History(ctx, outsider.ID, group, 50, 10)
	status(t, err, 404)
	_, err = chat.MarkRead(ctx, outsider.ID, group, 1)
	status(t, err, 404)
	_, err = chat.MarkReceived(ctx, outsider.ID, group, "phone", 1)
	status(t, err, 404)
	_, err = chat.Sync(ctx, bob.ID, group, 207, 100)
	status(t, err, 400)
	receipt, err := chat.MarkRead(ctx, bob.ID, group, 50)
	if err != nil || receipt.ReadAt == nil {
		t.Fatalf("read failed: %v", err)
	}
	older, err := chat.MarkRead(ctx, bob.ID, group, 10)
	if err != nil || older.Seq != 50 || !older.ReadAt.Equal(*receipt.ReadAt) {
		t.Fatal("read cursor moved backwards")
	}
	_, err = chat.MarkRead(ctx, bob.ID, group, 207)
	status(t, err, 400)
	for _, cursor := range []struct {
		device string
		seq    int64
	}{{"phone", 60}, {"desktop", 15}, {"phone", 10}} {
		if _, err := chat.MarkReceived(ctx, bob.ID, group, cursor.device, cursor.seq); err != nil {
			t.Fatal(err)
		}
	}
	for device, expected := range map[string]int64{"phone": 60, "desktop": 15, "new": 0} {
		cursors, err := chat.Cursors(ctx, bob.ID, device)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, cursor := range cursors {
			if cursor.ConversationID == group {
				found = true
				if cursor.ReceivedSeq != expected {
					t.Fatal("device cursor mixed")
				}
			}
		}
		if !found {
			t.Fatal("missing cursor")
		}
	}
	list, err := chat.Conversations(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range list {
		if conversation.ID == group && conversation.LastReadSeq != 50 {
			t.Fatal("received cursor marked messages read")
		}
	}

	aliceSocket, _ := services.Hub.Register(ctx, alice.ID, "phone", "session")
	bobSocket, _ := services.Hub.Register(ctx, bob.ID, "phone", "session")
	charlieSocket, _ := services.Hub.Register(ctx, charlie.ID, "phone", "session")
	outsiderSocket, _ := services.Hub.Register(ctx, outsider.ID, "phone", "session")
	dispatcher := &realtime.Dispatcher{Store: services.Store, Hub: services.Hub}
	for index := 0; index < 3; index++ {
		if err := dispatcher.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, client := range []*realtime.Client{aliceSocket, bobSocket, charlieSocket} {
		last, count, receipts := int64(0), 0, 0
		for len(client.Outbound) > 0 {
			event := <-client.Outbound
			if event.Event == "message.created" {
				payload, _ := json.Marshal(event.Data)
				var message model.Message
				_ = json.Unmarshal(payload, &message)
				if message.Seq != last+1 {
					t.Fatal("outbox changed message order")
				}
				last = message.Seq
				count++
			}
			if event.Event == "conversation.read" {
				receipts++
			}
		}
		if count != 206 || receipts != 1 {
			t.Fatalf("fanout mismatch: %d messages, %d receipts", count, receipts)
		}
	}
	if len(outsiderSocket.Outbound) != 0 {
		t.Fatal("outsider received group events")
	}
	late, err := services.Store.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Rollback(ctx)
	query := `INSERT INTO message_outbox(conversation_id,recipients,payload) VALUES($1,$2,'{"event":"probe"}')`
	if _, err = late.Exec(ctx, query, direct, []string{alice.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(ctx, query, group, []string{alice.ID}); err != nil {
		t.Fatal(err)
	}
	if err = dispatcher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err = late.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = dispatcher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if len(aliceSocket.Outbound) != 2 {
		t.Fatal("late committed lower outbox ID skipped")
	}
	if err = services.Store.Migrate(ctx, services.Config.MigrationsDir); err != nil {
		t.Fatal("migration not repeatable", err)
	}
}

func TestMigrationAdoptsLegacyDatabase(t *testing.T) {
	services := fixture(t, false)
	ctx := t.Context()
	_, err := services.Store.DB.Exec(ctx, `DROP TABLE message_outbox,device_cursors,schema_migrations; ALTER TABLE conversation_members DROP COLUMN read_at; ALTER TABLE conversation_members DROP CONSTRAINT read_cursor_nonnegative; DROP INDEX sessions_expiry_idx`)
	if err != nil {
		t.Fatal(err)
	}
	account(t, services, "legacy")
	if err := services.Store.Migrate(ctx, services.Config.MigrationsDir); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy data lost", err)
	}
	if err := services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatal("bad migration versions", err)
	}
}

type wireEvent struct {
	Event     string          `json:"event"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Error     *model.Error    `json:"error"`
}

type peer struct {
	connection *websocket.Conn
	events     chan wireEvent
	done       chan struct{}
}

func dial(t *testing.T, server *httptest.Server, token, device string) *peer {
	t.Helper()
	headers := http.Header{"Origin": {"http://im.test"}, "Cookie": {"session=" + token}}
	connection, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?device_id="+device, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	client := &peer{connection: connection, events: make(chan wireEvent, 1024), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); _ = connection.CloseNow(); <-client.done })
	go func() {
		defer close(client.done)
		for {
			_, payload, err := connection.Read(ctx)
			if err != nil {
				return
			}
			var event wireEvent
			if json.Unmarshal(payload, &event) != nil {
				return
			}
			select {
			case client.events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	await(t, client, "connection.ready", "")
	return client
}

func await(t *testing.T, client *peer, event, requestID string) wireEvent {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case incoming := <-client.events:
			if incoming.Event == event && (requestID == "" || incoming.RequestID == requestID) {
				return incoming
			}
		case <-client.done:
			t.Fatal("connection closed before " + event)
		case <-timer.C:
			t.Fatal("timeout waiting for " + event)
		}
	}
}

func commandTo(t *testing.T, client *peer, event, requestID string, data any) {
	t.Helper()
	payload, err := json.Marshal(model.Event{Event: event, RequestID: requestID, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err = client.connection.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
}

func httpCall(t *testing.T, server *httptest.Server, token, method, path string, body any, expected int) json.RawMessage {
	t.Helper()
	payload, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("%s: wanted %d got %d: %s", path, expected, response.StatusCode, content)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(content, &envelope) != nil {
		t.Fatal("invalid response")
	}
	return envelope.Data
}

func noMessages(t *testing.T, client *peer) {
	t.Helper()
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event := <-client.events:
			if event.Event == "message.created" {
				t.Fatal("unauthorized fanout")
			}
		case <-timer.C:
			return
		}
	}
}

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
