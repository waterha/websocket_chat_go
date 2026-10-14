package api

import (
	"context"
	"net/http"
	"time"
	"websocket_chat_go/backend/internal/model"
)

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
