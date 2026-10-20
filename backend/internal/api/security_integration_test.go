package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// login performs a real HTTP login so the CSRF cookie and token are exercised
// exactly as a browser would see them.
func login(t *testing.T, server *httptest.Server, email, password string, expected int) ([]*http.Cookie, []byte) {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/v1/auth/login", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("login: wanted %d got %d: %s", expected, response.StatusCode, content)
	}
	return response.Cookies(), content
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func TestCSRFProtectsStateChangingRequests(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, aliceToken := account(t, services, "alice")

	// A write without the token is refused even with a valid session cookie.
	payload, _ := json.Marshal(map[string]string{"client_msg_id": "csrf-0001", "content": "hi"})
	request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/v1/conversations/unknown/messages", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+aliceToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("a write without a CSRF token should be 403, got %d", response.StatusCode)
	}

	// A wrong token is refused too.
	request, _ = http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/v1/conversations/unknown/messages", bytes.NewReader(payload))
	request.Header.Set("Cookie", "session="+aliceToken)
	request.Header.Set("X-CSRF-Token", "wrong-token")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("a wrong CSRF token should be 403, got %d", response.StatusCode)
	}

	// Safe methods stay exempt, so history and sync remain reachable.
	if data := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200); len(data) == 0 {
		t.Fatal("GET should not require a CSRF token")
	}

	// A session with no stored token forces re-authentication rather than being
	// trusted: this is what protects sessions created before the upgrade.
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE sessions SET csrf_token=NULL WHERE token_hash=$1", model.TokenHash(aliceToken)); err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/unknown/messages", map[string]string{"client_msg_id": "csrf-0002", "content": "hi"}, 401)
}

func TestLoginSetsBothCookiesAndRotatesCSRF(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, _ = account(t, services, "alice")
	// Failures are recorded in Redis under a hashed key, so this test clears the
	// window first to stay independent of other tests sharing the instance.
	clearLoginLimit(t, services, "alice@example.test")

	cookies, body := login(t, server, "alice@example.test", "secure-password", 200)
	session := cookieValue(cookies, "session")
	csrf := cookieValue(cookies, "csrf")
	if len(session) != 64 || csrf == "" {
		t.Fatalf("login did not set both cookies: session=%q csrf=%q", session, csrf)
	}
	var envelope struct {
		Data model.User `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("unexpected login body: %s", body)
	}

	// A second login produces a different token pair.
	second, _ := login(t, server, "alice@example.test", "secure-password", 200)
	if cookieValue(second, "session") == session || cookieValue(second, "csrf") == csrf {
		t.Fatal("login reused the previous session or CSRF token")
	}
}

func TestLoginRateLimitKicksInAfterRepeatedFailures(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, _ = account(t, services, "alice")
	clearLoginLimit(t, services, "alice@example.test")

	// Ten wrong attempts are allowed and each returns 401.
	for attempt := 0; attempt < 10; attempt++ {
		login(t, server, "alice@example.test", "definitely-wrong", 401)
	}
	// The next attempt is throttled, and the response advertises a wait.
	payload, _ := json.Marshal(map[string]string{"email": "alice@example.test", "password": "definitely-wrong"})
	request, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/v1/auth/login", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 429 {
		t.Fatalf("expected throttling after repeated failures, got %d", response.StatusCode)
	}
	if response.Header.Get("Retry-After") == "" {
		t.Fatal("a throttled response should carry Retry-After")
	}
	// Even the correct password is refused while the window is open, which is
	// what makes the limiter meaningful against guessing.
	login(t, server, "alice@example.test", "secure-password", 429)
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, _ = account(t, services, "alice")
	clearLoginLimit(t, services, "alice@example.test")

	first, _ := login(t, server, "alice@example.test", "secure-password", 200)
	second, _ := login(t, server, "alice@example.test", "secure-password", 200)
	keeper, keeperCSRF := cookieValue(first, "session"), cookieValue(first, "csrf")
	victim, victimCSRF := cookieValue(second, "session"), cookieValue(second, "csrf")

	// The wrong current password is rejected.
	httpCallCSRF(t, server, keeper, keeperCSRF, "POST", "/api/v1/auth/password", map[string]string{"current_password": "wrong", "new_password": "brand-new-secret"}, 403)
	// The same password is refused rather than silently accepted.
	httpCallCSRF(t, server, keeper, keeperCSRF, "POST", "/api/v1/auth/password", map[string]string{"current_password": "secure-password", "new_password": "secure-password"}, 400)
	// A too-short new password is refused.
	httpCallCSRF(t, server, keeper, keeperCSRF, "POST", "/api/v1/auth/password", map[string]string{"current_password": "secure-password", "new_password": "short"}, 400)

	httpCallCSRF(t, server, keeper, keeperCSRF, "POST", "/api/v1/auth/password", map[string]string{"current_password": "secure-password", "new_password": "brand-new-secret"}, 200)

	// The other session is gone while the caller's own session survives. The
	// revoked session is checked with its own former CSRF token so the result is
	// a session failure rather than a CSRF failure.
	httpCallCSRF(t, server, victim, victimCSRF, "GET", "/api/v1/me", nil, 401)
	if data := httpCallCSRF(t, server, keeper, keeperCSRF, "GET", "/api/v1/me", nil, 200); len(data) == 0 {
		t.Fatal("the session that changed the password should survive")
	}
	// The old password no longer works and the new one does. The single failed
	// attempt stays well inside the limiter window.
	login(t, server, "alice@example.test", "secure-password", 401)
	clearLoginLimit(t, services, "alice@example.test")
	login(t, server, "alice@example.test", "brand-new-secret", 200)
}

// clearLoginLimit removes any recorded failures so tests do not interfere with
// each other through the Redis-backed window.
func clearLoginLimit(t *testing.T, services *svc.ServiceContext, email string) {
	t.Helper()
	var limiter = services.Auth.Limiter
	limiter.Clear(t.Context(), "login", email)
}

// httpCallCSRF is httpCall with an explicit CSRF token, used where a token is
// deliberately absent or belongs to a specific session.
func httpCallCSRF(t *testing.T, server *httptest.Server, token, csrf, method, path string, body any, expected int) json.RawMessage {
	t.Helper()
	payload, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+token)
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
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
