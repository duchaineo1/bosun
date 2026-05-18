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

		// credentials
		r.With(app.requirePermission("credential", "read")).Get("/api/v1/credentials", app.listCredentials)
		r.With(app.requirePermission("credential", "write")).Post("/api/v1/credentials", app.createCredential)
		r.With(app.requirePermission("credential", "delete")).Delete("/api/v1/credentials/{id}", app.deleteCredential)

		// templates
		r.With(app.requirePermission("template", "read")).Get("/api/v1/templates", app.listTemplates)
		r.With(app.requirePermission("template", "write")).Post("/api/v1/templates", app.createTemplate)
		r.With(app.requirePermission("template", "read")).Get("/api/v1/templates/{id}", app.getTemplate)
		r.With(app.requirePermission("template", "write")).Put("/api/v1/templates/{id}", app.updateTemplate)
		r.With(app.requirePermission("template", "delete")).Delete("/api/v1/templates/{id}", app.deleteTemplate)
		r.With(app.requirePermission("template", "execute")).Post("/api/v1/templates/{id}/launch", app.launchTemplate)

		// jobs
		r.With(app.requirePermission("job", "read")).Get("/api/v1/jobs", app.listJobs)
		r.With(app.requirePermission("job", "read")).Get("/api/v1/jobs/{id}", app.getJob)
		r.With(app.requirePermission("job", "read")).Get("/api/v1/jobs/{id}/logs", app.streamLogs)

		// users
		r.Post("/api/v1/users", app.createUser) // admin check inside handler
		r.Get("/api/v1/users/me", app.getMe)
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
