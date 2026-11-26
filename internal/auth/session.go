package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "session"

type Sessions struct {
	Pool *pgxpool.Pool
}

type userKey struct{}

type User struct {
	ID   string `json:"userId"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func UserID(ctx context.Context) string {
	user, _ := ctx.Value(userKey{}).(User)
	return user.ID
}

func Me(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userKey{}).(User)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(user)
}

func (s Sessions) Login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID   string `json:"userId"`
		Password string `json:"password"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	if err != nil || json.Unmarshal(body, &input) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if input.UserID == "" || strings.ContainsRune(input.UserID, 0) || len(input.Password) > 72 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	var hash []byte
	err = s.Pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", input.UserID).Scan(&hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		sessionError(w, err)
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword(hash, []byte(input.Password)) != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token := rand.Text()
	var expires time.Time
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO sessions (token, user_id, expires_at)
		VALUES ($1, $2, now() + interval '16 hours')
		RETURNING expires_at
	`, token, input.UserID).Scan(&expires)
	if err != nil {
		sessionError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s Sessions) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		if _, err := s.Pool.Exec(r.Context(), "DELETE FROM sessions WHERE token = $1", cookie.Value); err != nil {
			sessionError(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s Sessions) Require(next http.Handler) http.Handler {
	return s.require("", next)
}

func (s Sessions) RequireRole(role string, next http.Handler) http.Handler {
	return s.require(role, next)
}

func (s Sessions) require(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var user User
		err = s.Pool.QueryRow(r.Context(), `
			SELECT u.id, u.name, u.role
			FROM sessions s JOIN users u ON u.id = s.user_id
			WHERE s.token = $1 AND s.expires_at > now()
		`, cookie.Value).Scan(&user.ID, &user.Name, &user.Role)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			w.WriteHeader(http.StatusUnauthorized)
		case err != nil:
			sessionError(w, err)
		case role != "" && user.Role != role:
			w.WriteHeader(http.StatusForbidden)
		default:
			ctx := context.WithValue(r.Context(), userKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

func sessionError(w http.ResponseWriter, err error) {
	slog.Error("세션 처리 실패", "error", err)
	w.WriteHeader(http.StatusInternalServerError)
}
