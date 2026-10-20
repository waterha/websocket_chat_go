package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

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
	// A throttling failure carries a suggested wait in a header, not the body.
	if failure.RetryAfter > 0 {
		writer.Header().Set("Retry-After", logic.RetrySeconds(failure.RetryAfter))
	}
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
