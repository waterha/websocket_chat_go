package api

import (
	"net/http"
	"websocket_chat_go/backend/internal/logic"
)

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
