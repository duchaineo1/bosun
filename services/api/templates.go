package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Template struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Image        *string         `json:"image"`
	Playbook     string          `json:"playbook"`
	ExtraVars    json.RawMessage `json:"extra_vars"`
	GitURL       *string         `json:"git_url"`
	GitRef       string          `json:"git_ref"`
	CredentialID *string         `json:"credential_id"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (app *App) listTemplates(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(r.Context(),
		`SELECT id, name, description, image, playbook, extra_vars::text, git_url, git_ref, credential_id, created_at, updated_at
		 FROM job_templates ORDER BY created_at DESC`)
	if err != nil {
		apiErr(w, "db error", 500)
		return
	}
	defer rows.Close()

	out := []Template{}
	for rows.Next() {
		var t Template
		var ev string
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Image, &t.Playbook, &ev, &t.GitURL, &t.GitRef, &t.CredentialID, &t.CreatedAt, &t.UpdatedAt); err == nil {
			t.ExtraVars = json.RawMessage(ev)
			out = append(out, t)
		}
	}
	writeJSON(w, out)
}

func (app *App) createTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Image        *string         `json:"image"`
		Playbook     string          `json:"playbook"`
		ExtraVars    json.RawMessage `json:"extra_vars"`
		GitURL       *string         `json:"git_url"`
		GitRef       string          `json:"git_ref"`
		CredentialID *string         `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		apiErr(w, "name is required", 400)
		return
	}
	if body.Playbook == "" {
		body.Playbook = "/playbooks/sample.yml"
	}
	if len(body.ExtraVars) == 0 {
		body.ExtraVars = json.RawMessage("{}")
	}
	if body.GitRef == "" {
		body.GitRef = "main"
	}

	var t Template
	var ev string
	err := app.db.QueryRow(r.Context(),
		`INSERT INTO job_templates (name, description, image, playbook, extra_vars, git_url, git_ref, credential_id)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)
		 RETURNING id, name, description, image, playbook, extra_vars::text, git_url, git_ref, credential_id, created_at, updated_at`,
		body.Name, body.Description, body.Image, body.Playbook, string(body.ExtraVars), body.GitURL, body.GitRef, body.CredentialID,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Image, &t.Playbook, &ev, &t.GitURL, &t.GitRef, &t.CredentialID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		apiErr(w, "db error", 500)
		return
	}
	t.ExtraVars = json.RawMessage(ev)
	w.WriteHeader(201)
	writeJSON(w, t)
}

func (app *App) getTemplate(w http.ResponseWriter, r *http.Request) {
	var t Template
	var ev string
	err := app.db.QueryRow(r.Context(),
		`SELECT id, name, description, image, playbook, extra_vars::text, git_url, git_ref, credential_id, created_at, updated_at
		 FROM job_templates WHERE id=$1`, chi.URLParam(r, "id"),
	).Scan(&t.ID, &t.Name, &t.Description, &t.Image, &t.Playbook, &ev, &t.GitURL, &t.GitRef, &t.CredentialID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		apiErr(w, "not found", 404)
		return
	}
	t.ExtraVars = json.RawMessage(ev)
	writeJSON(w, t)
}

func (app *App) updateTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Image        *string         `json:"image"`
		Playbook     string          `json:"playbook"`
		ExtraVars    json.RawMessage `json:"extra_vars"`
		GitURL       *string         `json:"git_url"`
		GitRef       string          `json:"git_ref"`
		CredentialID *string         `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiErr(w, "bad request", 400)
		return
	}
	if len(body.ExtraVars) == 0 {
		body.ExtraVars = json.RawMessage("{}")
	}
	if body.GitRef == "" {
		body.GitRef = "main"
	}

	var t Template
	var ev string
	err := app.db.QueryRow(r.Context(),
		`UPDATE job_templates
		 SET name=$2, description=$3, image=$4, playbook=$5, extra_vars=$6::jsonb, git_url=$7, git_ref=$8, credential_id=$9, updated_at=NOW()
		 WHERE id=$1
		 RETURNING id, name, description, image, playbook, extra_vars::text, git_url, git_ref, credential_id, created_at, updated_at`,
		chi.URLParam(r, "id"), body.Name, body.Description, body.Image, body.Playbook, string(body.ExtraVars), body.GitURL, body.GitRef, body.CredentialID,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Image, &t.Playbook, &ev, &t.GitURL, &t.GitRef, &t.CredentialID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		apiErr(w, "not found", 404)
		return
	}
	t.ExtraVars = json.RawMessage(ev)
	writeJSON(w, t)
}

func (app *App) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	if _, err := app.db.Exec(r.Context(),
		"DELETE FROM job_templates WHERE id=$1", chi.URLParam(r, "id"),
	); err != nil {
		apiErr(w, "db error", 500)
		return
	}
	w.WriteHeader(204)
}
