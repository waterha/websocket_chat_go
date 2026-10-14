package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

type Server struct{ Services *svc.ServiceContext }
type identity struct {
	account     model.User
	sessionHash string
}
type authHandler func(http.ResponseWriter, *http.Request, identity)

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("POST /api/v1/auth/register", server.register)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.auth(server.logout))
	mux.HandleFunc("GET /api/v1/me", server.auth(func(writer http.ResponseWriter, request *http.Request, caller identity) {
		respond(writer, 200, caller.account)
	}))
	mux.HandleFunc("GET /api/v1/users", server.auth(server.users))
	mux.HandleFunc("GET /api/v1/conversations", server.auth(server.conversations))
	mux.HandleFunc("POST /api/v1/conversations", server.auth(server.create))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", server.auth(server.history))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/{seq}/delivery", server.auth(server.deliveryStatus))
	mux.HandleFunc("GET /api/v1/conversations/{id}/sync", server.auth(server.sync))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", server.auth(server.send))
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", server.auth(server.read))
	mux.HandleFunc("POST /api/v1/conversations/{id}/received", server.auth(server.received))
	mux.HandleFunc("GET /api/v1/devices/{device_id}/cursors", server.auth(server.cursors))
	mux.HandleFunc("GET /ws", server.auth(server.websocket))
	return server.cors(mux)
}

func respond(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data})
}

func publicError(err error) *model.Error {
	var failure *model.Error
	if errors.As(err, &failure) {
		return failure
	}
	log.Printf("request failed: %v", err)
	return &model.Error{Status: 500, Code: "internal_error", Message: "internal_error"}
}

func fail(writer http.ResponseWriter, err error) {
	failure := publicError(err)
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(failure.Status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": failure})
}

func decode(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(writer, model.Fail(400, "invalid_json"))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		fail(writer, model.Fail(400, "invalid_json"))
		return false
	}
	return true
}

func (server *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		writer.Header().Add("Vary", "Origin")
		if origin != "" && origin != server.Services.Config.Origin {
			fail(writer, model.Fail(403, "invalid_origin"))
			return
		}
		if origin != "" {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if request.Method == "OPTIONS" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.URL.Path != "/ws" {
			ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
			defer cancel()
			request = request.WithContext(ctx)
		}
		next.ServeHTTP(writer, request)
	})
}

func (server *Server) auth(next authHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("session")
		if err != nil || len(cookie.Value) != 64 {
			fail(writer, model.Fail(401, "unauthorized"))
			return
		}
		sessionHash := model.TokenHash(cookie.Value)
		account, err := server.Services.Auth.Authenticate(request.Context(), sessionHash)
		if err != nil {
			fail(writer, err)
			return
		}
		next(writer, request, identity{account: account, sessionHash: sessionHash})
	}
}

func (server *Server) cookie(writer http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, Secure: server.Services.Config.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func (server *Server) register(writer http.ResponseWriter, request *http.Request) {
	var input logic.Credentials
	if !decode(writer, request, &input) {
		return
	}
	account, token, err := server.Services.Auth.Register(request.Context(), input)
	if err != nil {
		fail(writer, err)
		return
	}
	server.cookie(writer, token, 7*24*3600)
	respond(writer, 201, account)
}

func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input logic.Credentials
	if !decode(writer, request, &input) {
		return
	}
	account, token, err := server.Services.Auth.Login(request.Context(), input)
	if err != nil {
		fail(writer, err)
		return
	}
	server.cookie(writer, token, 7*24*3600)
	respond(writer, 200, account)
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Auth.Logout(request.Context(), caller.sessionHash); err != nil {
		fail(writer, err)
		return
	}
	server.Services.Hub.DisconnectSession(caller.sessionHash)
	server.cookie(writer, "", -1)
	respond(writer, 200, nil)
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if server.Services.Store.DB.Ping(ctx) != nil || server.Services.Redis.Ping(ctx).Err() != nil {
		fail(writer, model.Fail(503, "dependency_unavailable"))
		return
	}
	respond(writer, 200, map[string]string{"status": "ok"})
}

func (server *Server) users(writer http.ResponseWriter, request *http.Request, caller identity) {
	users, err := server.Services.Auth.Users(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, users)
}

func (server *Server) conversations(writer http.ResponseWriter, request *http.Request, caller identity) {
	items, err := server.Services.Chat.Conversations(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}

func (server *Server) create(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.CreateInput
	if !decode(writer, request, &input) {
		return
	}
	id, err := server.Services.Chat.Create(request.Context(), caller.account.ID, input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, map[string]string{"id": id})
}

func integer(request *http.Request, key string, fallback int64) (int64, error) {
	value := request.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, model.Fail(400, "invalid_cursor")
	}
	return number, nil
}

func (server *Server) history(writer http.ResponseWriter, request *http.Request, caller identity) {
	before, err := integer(request, "before_seq", math.MaxInt64)
	if err != nil {
		fail(writer, err)
		return
	}
	limit, err := integer(request, "limit", 30)
	if err != nil {
		fail(writer, err)
		return
	}
	if limit < 1 || limit > 100 {
		fail(writer, model.Fail(400, "invalid_limit"))
		return
	}
	items, err := server.Services.Chat.History(request.Context(), caller.account.ID, request.PathValue("id"), before, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}

func (server *Server) sync(writer http.ResponseWriter, request *http.Request, caller identity) {
	after, err := integer(request, "after_seq", 0)
	if err != nil {
		fail(writer, err)
		return
	}
	limit, err := integer(request, "limit", 100)
	if err != nil {
		fail(writer, err)
		return
	}
	if limit < 1 || limit > 100 {
		fail(writer, model.Fail(400, "invalid_limit"))
		return
	}
	page, err := server.Services.Chat.Sync(request.Context(), caller.account.ID, request.PathValue("id"), after, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, page)
}

func (server *Server) send(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		ClientMsgID string `json:"client_msg_id"`
		Content     string `json:"content"`
	}
	if !decode(writer, request, &input) {
		return
	}
	item, duplicate, err := server.Services.Chat.Send(request.Context(), caller.account.ID, model.SendInput{ConversationID: request.PathValue("id"), ClientMsgID: input.ClientMsgID, Content: input.Content})
	if err != nil {
		fail(writer, err)
		return
	}
	status := 201
	if duplicate {
		status = 200
	}
	respond(writer, status, item)
}

func (server *Server) read(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		Seq int64 `json:"seq"`
	}
	if !decode(writer, request, &input) {
		return
	}
	receipt, err := server.Services.Chat.MarkRead(request.Context(), caller.account.ID, request.PathValue("id"), input.Seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, receipt)
}

func (server *Server) received(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		Seq      int64  `json:"seq"`
		DeviceID string `json:"device_id"`
	}
	if !decode(writer, request, &input) {
		return
	}
	cursor, err := server.Services.Chat.MarkReceived(request.Context(), caller.account.ID, request.PathValue("id"), input.DeviceID, input.Seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, cursor)
}

func (server *Server) deliveryStatus(writer http.ResponseWriter, request *http.Request, caller identity) {
	seq, err := strconv.ParseInt(request.PathValue("seq"), 10, 64)
	if err != nil {
		fail(writer, model.Fail(404, "not_found"))
		return
	}
	status, err := server.Services.Chat.DeliveryStatus(request.Context(), caller.account.ID, request.PathValue("id"), seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, status)
}

func (server *Server) cursors(writer http.ResponseWriter, request *http.Request, caller identity) {
	items, err := server.Services.Chat.Cursors(request.Context(), caller.account.ID, request.PathValue("device_id"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}
