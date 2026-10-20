package api

import (
	"context"
	"net/http"
	"strings"
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
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if request.Method == "OPTIONS" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.URL.Path != "/ws" {
			// Uploads stream up to 100 MiB of video, so they need far more time
			// than an ordinary JSON request; every other route keeps the short
			// bound that protects the rest of the API.
			timeout := 15 * time.Second
			if request.Method == "POST" && strings.HasSuffix(request.URL.Path, "/media") {
				timeout = 10 * time.Minute
			}
			ctx, cancel := context.WithTimeout(request.Context(), timeout)
			defer cancel()
			request = request.WithContext(ctx)
		}
		next.ServeHTTP(writer, request)
	})
}

// safeMethods never change state, so they are exempt from CSRF checks.
func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// csrfExempt lists state-changing routes that run before a session exists, so no
// per-session token can be presented. These are protected by SameSite cookies
// and the login limiter instead.
var csrfExempt = map[string]bool{
	"/api/v1/auth/register": true,
	"/api/v1/auth/login":    true,
}

// csrf protects state-changing requests with a double-submit token: the token is
// stored per session and echoed by the client in X-CSRF-Token. SameSite=Lax
// already blocks most cross-site form posts, but a token check also covers
// browsers or flows where SameSite is not enforced.
func (server *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if safeMethod(request.Method) || request.URL.Path == "/ws" || csrfExempt[request.URL.Path] {
			next.ServeHTTP(writer, request)
			return
		}
		cookie, err := request.Cookie("session")
		if err != nil || len(cookie.Value) != 64 {
			fail(writer, model.Fail(401, "unauthorized"))
			return
		}
		expected, err := server.Services.Auth.CSRFToken(request.Context(), model.TokenHash(cookie.Value))
		if err != nil {
			fail(writer, err)
			return
		}
		if request.Header.Get("X-CSRF-Token") != expected {
			fail(writer, model.Fail(403, "invalid_csrf"))
			return
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
