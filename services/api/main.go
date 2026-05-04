package main

import (
	"context"
	"log"
	"net/http"
	"os"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	db *pgxpool.Pool
}

func main() {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	app := &App{db: db}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors)

	// First-run setup (no auth required, blocked once a user exists)
	r.Post("/api/v1/setup", app.setup)
	r.Post("/api/v1/auth/login", app.login)

	r.Group(func(r chi.Router) {
		r.Use(app.requireAuth)

		r.Get("/api/v1/templates", app.listTemplates)
		r.Post("/api/v1/templates", app.createTemplate)
		r.Get("/api/v1/templates/{id}", app.getTemplate)
		r.Put("/api/v1/templates/{id}", app.updateTemplate)
		r.Delete("/api/v1/templates/{id}", app.deleteTemplate)
		r.Post("/api/v1/templates/{id}/launch", app.launchTemplate)

		r.Get("/api/v1/jobs", app.listJobs)
		r.Get("/api/v1/jobs/{id}", app.getJob)
		r.Get("/api/v1/jobs/{id}/logs", app.streamLogs)
	})

	addr := ":" + envOr("PORT", "8080")
	log.Printf("api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var: %s", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
