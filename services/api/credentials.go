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
		Name           string `json:"name"`
		Type           string `json:"type"`
		Token          string `json:"token"`           // pat
		AppID          string `json:"app_id"`          // github_app
		InstallationID string `json:"installation_id"` // github_app
		PrivateKey     string `json:"private_key"`     // github_app, ssh_key
		Passphrase     string `json:"passphrase"`      // ssh_key (optional)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		apiErr(w, "name is required", 400)
		return
	}
	if body.Type == "" {
		body.Type = "pat"
	}

	var rawData []byte
	switch body.Type {
	case "pat":
		if body.Token == "" {
			apiErr(w, "token is required for pat credentials", 400)
			return
		}
		encrypted, err := encryptToken(body.Token)
		if err != nil {
			apiErr(w, "encryption error: "+err.Error(), 500)
			return
		}
		rawData, _ = json.Marshal(map[string]string{"token": encrypted})
	case "github_app":
		if body.AppID == "" || body.InstallationID == "" || body.PrivateKey == "" {
			apiErr(w, "app_id, installation_id, and private_key are required for github_app credentials", 400)
			return
		}
		encryptedKey, err := encryptToken(body.PrivateKey)
		if err != nil {
			apiErr(w, "encryption error: "+err.Error(), 500)
			return
		}
		rawData, _ = json.Marshal(map[string]string{
			"app_id":          body.AppID,
			"installation_id": body.InstallationID,
			"private_key":     encryptedKey,
		})
	case "ssh_key":
		if body.PrivateKey == "" {
			apiErr(w, "private_key is required for ssh_key credentials", 400)
			return
		}
		encryptedKey, err := encryptToken(body.PrivateKey)
		if err != nil {
			apiErr(w, "encryption error: "+err.Error(), 500)
			return
		}
		data := map[string]string{"private_key": encryptedKey}
		if body.Passphrase != "" {
			encryptedPass, err := encryptToken(body.Passphrase)
			if err != nil {
				apiErr(w, "encryption error: "+err.Error(), 500)
				return
			}
			data["passphrase"] = encryptedPass
		}
		rawData, _ = json.Marshal(data)
	default:
		apiErr(w, "unsupported credential type: "+body.Type, 400)
		return
	}

	var c Credential
	if err := app.db.QueryRow(r.Context(),
		`INSERT INTO credentials (name, type, data) VALUES ($1,$2,$3::jsonb)
		 RETURNING id, name, type, created_at`,
		body.Name, body.Type, string(rawData),
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
