package main

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// createUser creates a new Bosun user. Admin-only.
// Role defaults to 'admin' until a BosunUser CRD sets it otherwise.
func (app *App) createUser(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(keyUserID).(string)
	if !app.canAccess(r.Context(), uid, "user", "write") {
		apiErr(w, "forbidden", 403)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		apiErr(w, "username and password are required", 400)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
	if err != nil {
		apiErr(w, "internal error", 500)
		return
	}

	var u User
	if err := app.db.QueryRow(r.Context(),
		`INSERT INTO users (username, password_hash)
		 VALUES ($1, $2)
		 RETURNING id, username, role, created_at`,
		body.Username, string(hash),
	).Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
		apiErr(w, "db error — username may already exist", 409)
		return
	}

	w.WriteHeader(201)
	writeJSON(w, u)
}

// getMe returns the authenticated user's profile including their current role.
func (app *App) getMe(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(keyUserID).(string)
	var u User
	if err := app.db.QueryRow(r.Context(),
		"SELECT id, username, role, created_at FROM users WHERE id=$1", uid,
	).Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
		apiErr(w, "not found", 404)
		return
	}
	writeJSON(w, u)
}
