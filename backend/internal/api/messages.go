package api

import (
	"math"
	"net/http"
	"strconv"
	"time"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

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

// searchAll searches every conversation the caller belongs to. Pagination uses a
// (created_at, id) cursor because message seq is only contiguous per
// conversation and therefore cannot order results across conversations.
func (server *Server) searchAll(writer http.ResponseWriter, request *http.Request, caller identity) {
	values := request.URL.Query()
	limit := int64(20)
	var err error
	if values.Has("limit") {
		limit, err = strconv.ParseInt(values.Get("limit"), 10, 64)
		if err != nil || limit < 1 || limit > 50 {
			fail(writer, model.Fail(400, "invalid_limit"))
			return
		}
	}
	var before *time.Time
	beforeID := values.Get("before_id")
	if values.Has("before_time") {
		parsed, err := time.Parse(time.RFC3339Nano, values.Get("before_time"))
		if err != nil {
			fail(writer, model.Fail(400, "invalid_cursor"))
			return
		}
		if beforeID == "" {
			fail(writer, model.Fail(400, "invalid_cursor"))
			return
		}
		before = &parsed
	} else if beforeID != "" {
		fail(writer, model.Fail(400, "invalid_cursor"))
		return
	}
	page, err := server.Services.Chat.SearchAll(request.Context(), caller.account.ID, values.Get("q"), before, beforeID, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, page)
}

func (server *Server) searchMessages(writer http.ResponseWriter, request *http.Request, caller identity) {
	values := request.URL.Query()
	before, limit := int64(math.MaxInt64), int64(30)
	var err error
	if values.Has("before_seq") {
		before, err = strconv.ParseInt(values.Get("before_seq"), 10, 64)
		if err != nil || before < 1 {
			fail(writer, model.Fail(400, "invalid_cursor"))
			return
		}
	}
	if values.Has("limit") {
		limit, err = strconv.ParseInt(values.Get("limit"), 10, 64)
		if err != nil || limit < 1 || limit > 100 {
			fail(writer, model.Fail(400, "invalid_limit"))
			return
		}
	}
	page, err := server.Services.Chat.SearchMessages(request.Context(), caller.account.ID, request.PathValue("id"), values.Get("q"), before, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, page)
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

// thread returns one page of a message's window: the root plus its replies,
// oldest first. It is the endpoint behind the conversation window.
func (server *Server) thread(writer http.ResponseWriter, request *http.Request, caller identity) {
	after, err := integer(request, "after_seq", 0)
	if err != nil {
		fail(writer, err)
		return
	}
	limit, err := integer(request, "limit", logic.ThreadReplyLimit)
	if err != nil {
		fail(writer, err)
		return
	}
	page, err := server.Services.Chat.Thread(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("message_id"), after, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, page)
}

func (server *Server) recall(writer http.ResponseWriter, request *http.Request, caller identity) {
	seq, err := strconv.ParseInt(request.PathValue("seq"), 10, 64)
	if err != nil {
		fail(writer, model.Fail(404, "not_found"))
		return
	}
	message, err := server.Services.Chat.Recall(request.Context(), caller.account.ID, request.PathValue("id"), seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, message)
}

func (server *Server) send(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		ClientMsgID  string        `json:"client_msg_id"`
		Content      string        `json:"content"`
		MediaID      string        `json:"media_id"`
		Blocks       []model.Block `json:"blocks"`
		ReplyToID    string        `json:"reply_to_id"`
		ThreadRootID string        `json:"thread_root_id"`
		Mentions     []string      `json:"mentions"`
	}
	if !decode(writer, request, &input) {
		return
	}
	// ThreadRootID is accepted for forward compatibility but the server derives
	// the real root from ReplyToID, so a client cannot file a reply into an
	// unrelated window. Blocks carry the ordered text and images; when present the
	// server derives content from them.
	item, duplicate, err := server.Services.Chat.Send(request.Context(), caller.account.ID, model.SendInput{ConversationID: request.PathValue("id"), ClientMsgID: input.ClientMsgID, Content: input.Content, MediaID: input.MediaID, Blocks: input.Blocks, ReplyToID: input.ReplyToID, ThreadRootID: input.ThreadRootID, Mentions: input.Mentions})
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
