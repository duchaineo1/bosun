package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Job struct {
	ID         string          `json:"id"`
	TemplateID *string         `json:"template_id"`
	Status     string          `json:"status"`
	ImageUsed  string          `json:"image_used"`
	Playbook   string          `json:"playbook"`
	ExtraVars  json.RawMessage `json:"extra_vars"`
	K8sJobName *string         `json:"k8s_job_name,omitempty"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (app *App) launchTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tpl struct {
		Image     *string
		Playbook  string
		ExtraVars string
	}
	if err := app.db.QueryRow(r.Context(),
		"SELECT image, playbook, extra_vars::text FROM job_templates WHERE id=$1", id,
	).Scan(&tpl.Image, &tpl.Playbook, &tpl.ExtraVars); err != nil {
		apiErr(w, "template not found", 404)
		return
	}

	// Optional per-launch extra_vars override
	var body struct {
		ExtraVars json.RawMessage `json:"extra_vars"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	extraVars := tpl.ExtraVars
	if len(body.ExtraVars) > 0 {
		extraVars = string(body.ExtraVars)
	}

	// image precedence: template.image > DEFAULT_RUNNER_IMAGE env var
	imageUsed := envOr("DEFAULT_RUNNER_IMAGE", "ghcr.io/duchaineo1/runner:latest")
	if tpl.Image != nil && *tpl.Image != "" {
		imageUsed = *tpl.Image
	}

	var j Job
	var ev string
	err := app.db.QueryRow(r.Context(),
		`INSERT INTO jobs (template_id, status, image_used, playbook, extra_vars)
		 VALUES ($1,'pending',$2,$3,$4::jsonb)
		 RETURNING id, template_id, status, image_used, playbook, extra_vars::text,
		           k8s_job_name, started_at, finished_at, created_at`,
		id, imageUsed, tpl.Playbook, extraVars,
	).Scan(&j.ID, &j.TemplateID, &j.Status, &j.ImageUsed, &j.Playbook, &ev,
		&j.K8sJobName, &j.StartedAt, &j.FinishedAt, &j.CreatedAt)
	if err != nil {
		apiErr(w, "db error", 500)
		return
	}
	j.ExtraVars = json.RawMessage(ev)
	w.WriteHeader(201)
	writeJSON(w, j)
}

func (app *App) listJobs(w http.ResponseWriter, r *http.Request) {
	templateID := r.URL.Query().Get("template_id")

	query := `SELECT id, template_id, status, image_used, playbook, extra_vars::text,
	                 k8s_job_name, started_at, finished_at, created_at
	          FROM jobs ORDER BY created_at DESC LIMIT 100`
	args := []any{}

	if templateID != "" {
		query = `SELECT id, template_id, status, image_used, playbook, extra_vars::text,
		                k8s_job_name, started_at, finished_at, created_at
		         FROM jobs WHERE template_id=$1 ORDER BY created_at DESC LIMIT 100`
		args = append(args, templateID)
	}

	rows, err := app.db.Query(r.Context(), query, args...)
	if err != nil {
		apiErr(w, "db error", 500)
		return
	}
	defer rows.Close()

	out := []Job{}
	for rows.Next() {
		var j Job
		var ev string
		if err := rows.Scan(&j.ID, &j.TemplateID, &j.Status, &j.ImageUsed, &j.Playbook, &ev,
			&j.K8sJobName, &j.StartedAt, &j.FinishedAt, &j.CreatedAt); err == nil {
			j.ExtraVars = json.RawMessage(ev)
			out = append(out, j)
		}
	}
	writeJSON(w, out)
}

func (app *App) getJob(w http.ResponseWriter, r *http.Request) {
	var j Job
	var ev string
	err := app.db.QueryRow(r.Context(),
		`SELECT id, template_id, status, image_used, playbook, extra_vars::text,
		        k8s_job_name, started_at, finished_at, created_at
		 FROM jobs WHERE id=$1`, chi.URLParam(r, "id"),
	).Scan(&j.ID, &j.TemplateID, &j.Status, &j.ImageUsed, &j.Playbook, &ev,
		&j.K8sJobName, &j.StartedAt, &j.FinishedAt, &j.CreatedAt)
	if err != nil {
		apiErr(w, "not found", 404)
		return
	}
	j.ExtraVars = json.RawMessage(ev)
	writeJSON(w, j)
}

// streamLogs sends job log lines as Server-Sent Events.
// Keeps the connection open while the job is running, closes when done.
func (app *App) streamLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var status string
	if err := app.db.QueryRow(r.Context(),
		"SELECT status FROM jobs WHERE id=$1", id,
	).Scan(&status); err != nil {
		apiErr(w, "job not found", 404)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		apiErr(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	flusher.Flush()

	lastLine := 0

	for {
		// Send any new log lines
		rows, err := app.db.Query(r.Context(),
			"SELECT line_num, content FROM job_logs WHERE job_id=$1 AND line_num>$2 ORDER BY line_num",
			id, lastLine)
		if err == nil {
			for rows.Next() {
				var n int
				var content string
				if rows.Scan(&n, &content) == nil {
					fmt.Fprintf(w, "data: %s\n\n", content)
					lastLine = n
				}
			}
			rows.Close()
			flusher.Flush()
		}

		// Check terminal status
		var cur string
		app.db.QueryRow(r.Context(), "SELECT status FROM jobs WHERE id=$1", id).Scan(&cur)
		if cur == "success" || cur == "failed" {
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", cur)
			flusher.Flush()
			return
		}

		// Wait or exit if client disconnected
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}
