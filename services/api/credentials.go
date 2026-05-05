package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Credential struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

func (app *App) listCredentials(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(r.Context(),
		`SELECT id, name, type, created_at FROM credentials ORDER BY created_at DESC`)
	if err != nil {
		apiErr(w, "db error", 500)
		return
	}
	defer rows.Close()

	out := []Credential{}
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.CreatedAt); err == nil {
			out = append(out, c)
		}
	}
	writeJSON(w, out)
}

func (app *App) createCredential(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Token == "" {
		apiErr(w, "name and token are required", 400)
		return
	}
	if body.Type == "" {
		body.Type = "pat"
	}

	encrypted, err := encryptToken(body.Token)
	if err != nil {
		apiErr(w, "encryption error: "+err.Error(), 500)
		return
	}
	data, _ := json.Marshal(map[string]string{"token": encrypted})

	var c Credential
	if err = app.db.QueryRow(r.Context(),
		`INSERT INTO credentials (name, type, data) VALUES ($1,$2,$3::jsonb)
		 RETURNING id, name, type, created_at`,
		body.Name, body.Type, string(data),
	).Scan(&c.ID, &c.Name, &c.Type, &c.CreatedAt); err != nil {
		apiErr(w, "db error", 500)
		return
	}
	w.WriteHeader(201)
	writeJSON(w, c)
}

func (app *App) deleteCredential(w http.ResponseWriter, r *http.Request) {
	if _, err := app.db.Exec(r.Context(),
		"DELETE FROM credentials WHERE id=$1", chi.URLParam(r, "id"),
	); err != nil {
		apiErr(w, "db error", 500)
		return
	}
	w.WriteHeader(204)
}
