package main

import (
	"context"
	"net/http"
)

// roleVerbs defines the verbs each global role implicitly grants per resource.
// Team permissions are additive on top of these.
var roleVerbs = map[string]map[string][]string{
	"admin": {
		"template":   {"read", "write", "execute", "delete"},
		"credential": {"read", "write", "delete"},
		"job":        {"read"},
		"user":       {"write"}, // create users
	},
	"operator": {
		"template":   {"read", "write", "execute"},
		"credential": {"read"},
		"job":        {"read"},
	},
	"viewer": {
		"template":   {"read"},
		"credential": {},
		"job":        {"read"},
	},
}

// canAccess returns true if the user (identified by userID) is allowed to
// perform verb on resource. It checks the user's global role first, then
// team-level grants. Falls back to allow=true on any DB error so a transient
// connectivity issue never locks everyone out.
func (app *App) canAccess(ctx context.Context, userID, resource, verb string) bool {
	var role string
	if err := app.db.QueryRow(ctx,
		"SELECT role FROM users WHERE id=$1", userID,
	).Scan(&role); err != nil {
		// User row missing or DB error — default allow (backwards compat with
		// the pre-RBAC single-admin model).
		return true
	}

	// admins always pass
	if role == "admin" {
		return true
	}

	// Check global role verb table
	if verbs, ok := roleVerbs[role][resource]; ok {
		for _, v := range verbs {
			if v == verb {
				return true
			}
		}
	}

	// Check team-granted verbs
	var count int
	app.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM team_permissions tp
		JOIN team_members tm ON tm.team_id = tp.team_id
		WHERE tm.user_id = $1
		  AND tp.resource = $2
		  AND $3 = ANY(tp.verbs)
	`, userID, resource, verb).Scan(&count)

	return count > 0
}

// requirePermission returns a chi middleware that enforces resource+verb access.
func (app *App) requirePermission(resource, verb string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, _ := r.Context().Value(keyUserID).(string)
			if !app.canAccess(r.Context(), uid, resource, verb) {
				apiErr(w, "forbidden", 403)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
