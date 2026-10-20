package api

import (
	"net/http"

	"websocket_chat_go/backend/internal/logic"
)

// csrfCookie carries the session's CSRF token in a readable cookie so the client
// can echo it in the X-CSRF-Token header. It is deliberately not HttpOnly,
// because the JavaScript client must read it; the token is only useful together
// with the HttpOnly session cookie.
func (server *Server) csrfCookie(writer http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{Name: "csrf", Value: token, Path: "/", HttpOnly: false, Secure: server.Services.Config.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func (server *Server) cookie(writer http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, Secure: server.Services.Config.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// establishedSession sets both cookies after a successful login or registration.
func (server *Server) establishedSession(writer http.ResponseWriter, session logic.Session) {
	seconds := int(logic.SessionLifetime.Seconds())
	server.cookie(writer, session.Token, seconds)
	server.csrfCookie(writer, session.CSRFToken, seconds)
}

func (server *Server) register(writer http.ResponseWriter, request *http.Request) {
	var input logic.Credentials
	if !decode(writer, request, &input) {
		return
	}
	account, session, err := server.Services.Auth.Register(request.Context(), input)
	if err != nil {
		fail(writer, err)
		return
	}
	server.establishedSession(writer, session)
	respond(writer, 201, account)
}

func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input logic.Credentials
	if !decode(writer, request, &input) {
		return
	}
	account, session, err := server.Services.Auth.Login(request.Context(), input)
	if err != nil {
		fail(writer, err)
		return
	}
	server.establishedSession(writer, session)
	respond(writer, 200, account)
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Auth.Logout(request.Context(), caller.sessionHash); err != nil {
		fail(writer, err)
		return
	}
	server.Services.Hub.DisconnectSession(caller.sessionHash)
	server.cookie(writer, "", -1)
	server.csrfCookie(writer, "", -1)
	respond(writer, 200, nil)
}

// changePassword rotates the password and revokes every other session, which
// also disconnects their WebSocket connections.
func (server *Server) changePassword(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Auth.ChangePassword(request.Context(), caller.account.ID, caller.sessionHash, input.CurrentPassword, input.NewPassword); err != nil {
		fail(writer, err)
		return
	}
	// Other sessions are gone; only the caller's own connection survives.
	server.Services.Hub.DisconnectOthers(caller.account.ID, caller.sessionHash)
	respond(writer, 200, map[string]any{"revoked_other_sessions": true})
}
