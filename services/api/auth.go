package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const keyUserID ctxKey = "uid"

func (app *App) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiErr(w, "bad request", 400)
		return
	}

	var id, hash string
	if err := app.db.QueryRow(r.Context(),
		"SELECT id, password_hash FROM users WHERE username=$1", body.Username,
	).Scan(&id, &hash); err != nil {
		apiErr(w, "invalid credentials", 401)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		apiErr(w, "invalid credentials", 401)
		return
	}

	tok, err := signToken(id, body.Username)
	if err != nil {
		apiErr(w, "token error", 500)
		return
	}
	writeJSON(w, map[string]string{"token": tok})
}

// setup creates the first admin user; blocked once any user exists.
func (app *App) setup(w http.ResponseWriter, r *http.Request) {
	var count int
	app.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		apiErr(w, "already configured", 409)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		apiErr(w, "username and password required", 400)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
	if err != nil {
		apiErr(w, "internal error", 500)
		return
	}

	var id string
	if err := app.db.QueryRow(r.Context(),
		"INSERT INTO users (username, password_hash) VALUES ($1,$2) RETURNING id",
		body.Username, string(hash),
	).Scan(&id); err != nil {
		apiErr(w, "db error", 500)
		return
	}

	tok, _ := signToken(id, body.Username)
	w.WriteHeader(201)
	writeJSON(w, map[string]string{"token": tok})
}

func (app *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokStr := ""
		// SSE clients can't set headers, so also accept ?token= query param
		if t := r.URL.Query().Get("token"); t != "" {
			tokStr = t
		} else {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				apiErr(w, "unauthorized", 401)
				return
			}
			tokStr = strings.TrimPrefix(h, "Bearer ")
		}

		claims := jwt.MapClaims{}
		if _, err := jwt.ParseWithClaims(
			tokStr, claims,
			func(*jwt.Token) (any, error) { return []byte(jwtSecret()), nil },
		); err != nil {
			apiErr(w, "unauthorized", 401)
			return
		}
		uid, _ := claims["sub"].(string)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyUserID, uid)))
	})
}

func signToken(uid, username string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  uid,
		"user": username,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	}).SignedString([]byte(jwtSecret()))
}

func jwtSecret() string { return envOr("JWT_SECRET", "dev-secret-change-me") }

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

