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

type Auth struct{ Store *model.Store }

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

func (auth *Auth) Register(ctx context.Context, input Credentials) (model.User, string, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || len(input.Name) < 1 || len([]rune(input.Name)) > 64 || len(input.Password) < 8 || len(input.Password) > 72 {
		return model.User{}, "", model.Fail(400, "invalid_registration")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, "", err
	}
	account := model.User{ID: model.NewID(), Email: input.Email, Name: input.Name}
	tx, err := auth.Store.DB.Begin(ctx)
	if err != nil {
		return model.User{}, "", err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "INSERT INTO users(id,email,name,password_hash) VALUES($1,$2,$3,$4)", account.ID, account.Email, account.Name, string(passwordHash))
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			err = model.Fail(409, "email_exists")
		}
		return model.User{}, "", err
	}
	token := model.NewID() + model.NewID()
	if _, err = tx.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", model.TokenHash(token), account.ID, time.Now().Add(7*24*time.Hour)); err != nil {
		return model.User{}, "", err
	}
	return account, token, tx.Commit(ctx)
}

func (auth *Auth) Login(ctx context.Context, input Credentials) (model.User, string, error) {
	var account model.User
	var passwordHash string
	err := auth.Store.DB.QueryRow(ctx, "SELECT id,email,name,password_hash FROM users WHERE email=$1", strings.ToLower(strings.TrimSpace(input.Email))).Scan(&account.ID, &account.Email, &account.Name, &passwordHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return account, "", err
	}
	if err != nil || len(input.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		return account, "", model.Fail(401, "invalid_credentials")
	}
	token := model.NewID() + model.NewID()
	_, err = auth.Store.DB.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", model.TokenHash(token), account.ID, time.Now().Add(7*24*time.Hour))
	return account, token, err
}

func (auth *Auth) Authenticate(ctx context.Context, sessionHash string) (model.User, error) {
	var account model.User
	err := auth.Store.DB.QueryRow(ctx, "SELECT u.id,u.email,u.name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()", sessionHash).Scan(&account.ID, &account.Email, &account.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, model.Fail(401, "unauthorized")
	}
	return account, err
}

func (auth *Auth) Logout(ctx context.Context, sessionHash string) error {
	_, err := auth.Store.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", sessionHash)
	return err
}

func (auth *Auth) Users(ctx context.Context, userID string) ([]model.User, error) {
	rows, err := auth.Store.DB.Query(ctx, "SELECT id,email,name FROM users WHERE id<>$1 ORDER BY name LIMIT 100", userID)
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
