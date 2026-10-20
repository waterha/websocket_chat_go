package logic

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"websocket_chat_go/backend/internal/model"
)

type Auth struct {
	Store *model.Store
	// Limiter slows down repeated credential failures.
	Limiter *Limiter
}

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

// Session is a logged-in session together with the CSRF token that must be sent
// on state-changing requests. The token is returned once at login and kept in a
// readable cookie so the client can echo it in a header.
type Session struct {
	Token     string
	CSRFToken string
}

// limitLogin enforces a per-account allowance.
func (auth *Auth) limitLogin(ctx context.Context, email string) error {
	if exceeded, ttl := auth.Limiter.Exceeded(ctx, "login", email, DefaultLoginAttempts); exceeded {
		return model.FailRetry(429, "too_many_attempts", ttl)
	}
	return nil
}

const (
	// DefaultLoginAttempts is how many failures are allowed per window.
	DefaultLoginAttempts = 10
	// LoginWindow is the fixed window length for failed logins.
	LoginWindow = 15 * time.Minute
)

func (auth *Auth) Register(ctx context.Context, input Credentials) (model.User, Session, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || len(input.Name) < 1 || len([]rune(input.Name)) > 64 || len(input.Password) < 8 || len(input.Password) > 72 {
		return model.User{}, Session{}, model.Fail(400, "invalid_registration")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, Session{}, err
	}
	account := model.User{ID: model.NewID(), Email: input.Email, Name: input.Name}
	tx, err := auth.Store.DB.Begin(ctx)
	if err != nil {
		return model.User{}, Session{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "INSERT INTO users(id,email,name,password_hash) VALUES($1,$2,$3,$4)", account.ID, account.Email, account.Name, string(passwordHash))
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			err = model.Fail(409, "email_exists")
		}
		return model.User{}, Session{}, err
	}
	session := auth.newSession()
	if _, err = tx.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at,csrf_token) VALUES($1,$2,$3,$4)", model.TokenHash(session.Token), account.ID, time.Now().Add(SessionLifetime), session.CSRFToken); err != nil {
		return model.User{}, Session{}, err
	}
	return account, session, tx.Commit(ctx)
}

// SessionLifetime is how long a login lasts.
const SessionLifetime = 7 * 24 * time.Hour

func (auth *Auth) newSession() Session {
	return Session{Token: model.NewID() + model.NewID(), CSRFToken: model.NewID()}
}

func (auth *Auth) Login(ctx context.Context, input Credentials) (model.User, Session, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if err := auth.limitLogin(ctx, email); err != nil {
		return model.User{}, Session{}, err
	}
	var account model.User
	var passwordHash string
	err := auth.Store.DB.QueryRow(ctx, "SELECT id,email,name,password_hash FROM users WHERE email=$1", email).Scan(&account.ID, &account.Email, &account.Name, &passwordHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return account, Session{}, err
	}
	// A wrong password and an unknown account are recorded identically so the
	// limiter cannot be used to enumerate registered addresses.
	if err != nil || len(input.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		auth.Limiter.Record(ctx, "login", email, LoginWindow)
		return account, Session{}, model.Fail(401, "invalid_credentials")
	}
	session := auth.newSession()
	_, err = auth.Store.DB.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at,csrf_token) VALUES($1,$2,$3,$4)", model.TokenHash(session.Token), account.ID, time.Now().Add(SessionLifetime), session.CSRFToken)
	if err != nil {
		return account, Session{}, err
	}
	auth.Limiter.Clear(ctx, "login", email)
	return account, session, nil
}

func (auth *Auth) Authenticate(ctx context.Context, sessionHash string) (model.User, error) {
	var account model.User
	err := auth.Store.DB.QueryRow(ctx, "SELECT u.id,u.email,u.name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()", sessionHash).Scan(&account.ID, &account.Email, &account.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, model.Fail(401, "unauthorized")
	}
	return account, err
}

// CSRFToken returns the token bound to a live session. A missing token means the
// session predates CSRF protection, so the caller is asked to log in again
// instead of being trusted implicitly.
func (auth *Auth) CSRFToken(ctx context.Context, sessionHash string) (string, error) {
	var token string
	err := auth.Store.DB.QueryRow(ctx, "SELECT COALESCE(csrf_token,'') FROM sessions WHERE token_hash=$1 AND expires_at>now()", sessionHash).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.Fail(401, "unauthorized")
	}
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", model.Fail(401, "reauthentication_required")
	}
	return token, nil
}

// ChangePassword verifies the current password, stores the new one and revokes
// every session except the one making the change, so a stolen session cannot
// survive a password rotation.
func (auth *Auth) ChangePassword(ctx context.Context, userID, sessionHash, currentPassword, newPassword string) error {
	if len(newPassword) < 8 || len(newPassword) > 72 || len(currentPassword) > 72 {
		return model.Fail(400, "invalid_password")
	}
	var passwordHash string
	if err := auth.Store.DB.QueryRow(ctx, "SELECT password_hash FROM users WHERE id=$1", userID).Scan(&passwordHash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)) != nil {
		return model.Fail(403, "invalid_password")
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(newPassword)) == nil {
		return model.Fail(400, "password_unchanged")
	}
	updated, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := auth.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE id=$2", string(updated), userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1 AND token_hash<>$2", userID, sessionHash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (auth *Auth) UpdateProfile(ctx context.Context, userID string, input model.ProfileInput) (model.User, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || len(input.Name) < 1 || len([]rune(input.Name)) > 64 {
		return model.User{}, model.Fail(400, "invalid_profile")
	}
	var account model.User
	err = auth.Store.DB.QueryRow(ctx, "UPDATE users SET email=$1,name=$2 WHERE id=$3 RETURNING id,email,name", input.Email, input.Name, userID).Scan(&account.ID, &account.Email, &account.Name)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return model.User{}, model.Fail(409, "email_exists")
		}
		return model.User{}, err
	}
	return account, nil
}

func (auth *Auth) Logout(ctx context.Context, sessionHash string) error {
	_, err := auth.Store.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", sessionHash)
	return err
}

func (auth *Auth) Users(ctx context.Context, userID string) ([]model.User, error) {
	rows, err := auth.Store.DB.Query(ctx, "SELECT id,email,name FROM users WHERE id<>$1 ORDER BY name,id LIMIT 1000", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []model.User{}
	for rows.Next() {
		var account model.User
		if err := rows.Scan(&account.ID, &account.Email, &account.Name); err != nil {
			return nil, err
		}
		users = append(users, account)
	}
	return users, rows.Err()
}
