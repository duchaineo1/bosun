package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	gvrCredential   = schema.GroupVersionResource{Group: "bosun.io", Version: "v1alpha1", Resource: "bosuncredentials"}
	gvrTemplate     = schema.GroupVersionResource{Group: "bosun.io", Version: "v1alpha1", Resource: "bosuntemplates"}
	gvrOrganization = schema.GroupVersionResource{Group: "bosun.io", Version: "v1alpha1", Resource: "bosunorganizations"}
	gvrUser         = schema.GroupVersionResource{Group: "bosun.io", Version: "v1alpha1", Resource: "bosunusers"}
)

// reconcileOperator reconciles all Bosun CRD instances into the DB.
// Errors are logged by the caller but never block job scheduling.
// Each sub-reconciler is idempotent; running every 5s is safe.
func reconcileOperator(ctx context.Context, db *pgxpool.Pool, dyn dynamic.Interface, k8s *kubernetes.Clientset, ns string) error {
	// credentials must run before templates (FK: job_templates.credential_id → credentials.id)
	if err := reconcileCRDCredentials(ctx, db, dyn, k8s, ns); err != nil {
		return fmt.Errorf("credentials: %w", err)
	}
	if err := reconcileCRDTemplates(ctx, db, dyn, ns); err != nil {
		return fmt.Errorf("templates: %w", err)
	}
	if err := reconcileCRDOrganizations(ctx, db, dyn, ns); err != nil {
		return fmt.Errorf("organizations: %w", err)
	}
	if err := reconcileCRDUsers(ctx, db, dyn, ns); err != nil {
		return fmt.Errorf("users: %w", err)
	}
	return nil
}

// ── Credentials ─────────────────────────────────────────────────────────────

func reconcileCRDCredentials(ctx context.Context, db *pgxpool.Pool, dyn dynamic.Interface, k8s *kubernetes.Clientset, ns string) error {
	list, err := dyn.Resource(gvrCredential).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCRDMissing(err) {
			return nil
		}
		return err
	}

	declared := make([]string, 0, len(list.Items))

	for _, item := range list.Items {
		name := item.GetName()
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			log.Printf("operator: BosunCredential %s has no spec — skipping", name)
			continue
		}
		credType, _ := spec["type"].(string)
		secretRef, _ := spec["secretRef"].(string)
		if credType == "" || secretRef == "" {
			log.Printf("operator: BosunCredential %s missing type or secretRef — skipping", name)
			continue
		}

		secret, err := k8s.CoreV1().Secrets(ns).Get(ctx, secretRef, metav1.GetOptions{})
		if err != nil {
			log.Printf("operator: BosunCredential %s: cannot read Secret %s: %v — skipping", name, secretRef, err)
			continue
		}

		data, err := buildCredentialData(credType, secret.Data)
		if err != nil {
			log.Printf("operator: BosunCredential %s: %v — skipping", name, err)
			continue
		}

		dataJSON, _ := json.Marshal(data)
		if _, err := db.Exec(ctx,
			`INSERT INTO credentials (name, type, data, source)
			 VALUES ($1, $2, $3::jsonb, 'crd')
			 ON CONFLICT (name) DO UPDATE
			   SET type=EXCLUDED.type, data=EXCLUDED.data, source='crd'`,
			name, credType, string(dataJSON),
		); err != nil {
			log.Printf("operator: BosunCredential %s: db upsert: %v", name, err)
			continue
		}
		declared = append(declared, name)
	}

	// Delete CRD-managed credentials that no longer have a CRD instance
	if _, err := db.Exec(ctx,
		`DELETE FROM credentials WHERE source='crd' AND name != ALL($1::text[])`,
		declared,
	); err != nil {
		return fmt.Errorf("delete orphan credentials: %w", err)
	}

	return nil
}

// buildCredentialData reads secret bytes by type and returns an encrypted data map.
func buildCredentialData(credType string, secretData map[string][]byte) (map[string]string, error) {
	enc := func(key string) (string, error) {
		v, ok := secretData[key]
		if !ok || len(v) == 0 {
			return "", fmt.Errorf("Secret missing key %q", key)
		}
		return encryptToken(strings.TrimSpace(string(v)))
	}

	switch credType {
	case "pat":
		token, err := enc("token")
		if err != nil {
			return nil, err
		}
		return map[string]string{"token": token}, nil

	case "github_app":
		appID, err := enc("app_id")
		if err != nil {
			return nil, err
		}
		instID, err := enc("installation_id")
		if err != nil {
			return nil, err
		}
		pk, err := enc("private_key")
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"app_id":          appID,
			"installation_id": instID,
			"private_key":     pk,
		}, nil

	case "ssh_key":
		pk, err := enc("private_key")
		if err != nil {
			return nil, err
		}
		data := map[string]string{"private_key": pk}
		if pp, ok := secretData["passphrase"]; ok && len(pp) > 0 {
			encPP, err := encryptToken(strings.TrimSpace(string(pp)))
			if err != nil {
				return nil, fmt.Errorf("encrypt passphrase: %w", err)
			}
			data["passphrase"] = encPP
		}
		return data, nil

	default:
		return nil, fmt.Errorf("unknown credential type %q", credType)
	}
}

// ── Templates ────────────────────────────────────────────────────────────────

func reconcileCRDTemplates(ctx context.Context, db *pgxpool.Pool, dyn dynamic.Interface, ns string) error {
	list, err := dyn.Resource(gvrTemplate).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCRDMissing(err) {
			return nil
		}
		return err
	}

	declared := make([]string, 0, len(list.Items))

	for _, item := range list.Items {
		name := item.GetName()
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			log.Printf("operator: BosunTemplate %s has no spec — skipping", name)
			continue
		}

		playbook, _ := spec["playbook"].(string)
		if playbook == "" {
			log.Printf("operator: BosunTemplate %s missing playbook — skipping", name)
			continue
		}

		gitURL, _ := spec["gitUrl"].(string)
		gitRef, _ := spec["gitRef"].(string)
		if gitRef == "" {
			gitRef = "main"
		}
		image, _ := spec["image"].(string)
		credentialRef, _ := spec["credentialRef"].(string)

		// extraVars: marshal the map to JSON
		extraVarsJSON := "{}"
		if ev, _ := spec["extraVars"].(map[string]interface{}); ev != nil {
			if b, err := json.Marshal(ev); err == nil {
				extraVarsJSON = string(b)
			}
		}

		// Resolve credential FK (NULL if credentialRef is empty or not found)
		var credentialID *string
		if credentialRef != "" {
			var id string
			if err := db.QueryRow(ctx,
				"SELECT id FROM credentials WHERE name=$1", credentialRef,
			).Scan(&id); err == nil {
				credentialID = &id
			} else {
				log.Printf("operator: BosunTemplate %s: credential %q not found in DB — storing without credential", name, credentialRef)
			}
		}

		var imagePtr *string
		if image != "" {
			imagePtr = &image
		}
		var gitURLPtr *string
		if gitURL != "" {
			gitURLPtr = &gitURL
		}

		if _, err := db.Exec(ctx,
			`INSERT INTO job_templates (name, playbook, image, git_url, git_ref, extra_vars, credential_id, source)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, 'crd')
			 ON CONFLICT (name) DO UPDATE
			   SET playbook=EXCLUDED.playbook, image=EXCLUDED.image,
			       git_url=EXCLUDED.git_url, git_ref=EXCLUDED.git_ref,
			       extra_vars=EXCLUDED.extra_vars, credential_id=EXCLUDED.credential_id,
			       source='crd', updated_at=NOW()`,
			name, playbook, imagePtr, gitURLPtr, gitRef, extraVarsJSON, credentialID,
		); err != nil {
			log.Printf("operator: BosunTemplate %s: db upsert: %v", name, err)
			continue
		}
		declared = append(declared, name)
	}

	if _, err := db.Exec(ctx,
		`DELETE FROM job_templates WHERE source='crd' AND name != ALL($1::text[])`,
		declared,
	); err != nil {
		return fmt.Errorf("delete orphan templates: %w", err)
	}

	return nil
}

// ── Organizations ────────────────────────────────────────────────────────────

func reconcileCRDOrganizations(ctx context.Context, db *pgxpool.Pool, dyn dynamic.Interface, ns string) error {
	list, err := dyn.Resource(gvrOrganization).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCRDMissing(err) {
			return nil
		}
		return err
	}

	declaredOrgSlugs := make([]string, 0, len(list.Items))

	for _, item := range list.Items {
		orgSlug := item.GetName()
		spec, _ := item.Object["spec"].(map[string]interface{})

		orgName, _ := spec["name"].(string)
		if orgName == "" {
			orgName = orgSlug
		}

		// Upsert org
		var orgID string
		if err := db.QueryRow(ctx,
			`INSERT INTO organizations (slug, name, updated_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, updated_at=NOW()
			 RETURNING id`,
			orgSlug, orgName,
		).Scan(&orgID); err != nil {
			log.Printf("operator: BosunOrganization %s: db upsert: %v", orgSlug, err)
			continue
		}
		declaredOrgSlugs = append(declaredOrgSlugs, orgSlug)

		// Reconcile teams
		teamsRaw, _ := spec["teams"].([]interface{})
		declaredTeamSlugs := make([]string, 0, len(teamsRaw))

		for _, t := range teamsRaw {
			teamMap, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			teamSlug, _ := teamMap["slug"].(string)
			teamName, _ := teamMap["name"].(string)
			if teamSlug == "" || teamName == "" {
				continue
			}

			var teamID string
			if err := db.QueryRow(ctx,
				`INSERT INTO teams (org_id, slug, name, updated_at)
				 VALUES ($1, $2, $3, NOW())
				 ON CONFLICT (org_id, slug) DO UPDATE SET name=EXCLUDED.name, updated_at=NOW()
				 RETURNING id`,
				orgID, teamSlug, teamName,
			).Scan(&teamID); err != nil {
				log.Printf("operator: BosunOrganization %s team %s: db upsert: %v", orgSlug, teamSlug, err)
				continue
			}
			declaredTeamSlugs = append(declaredTeamSlugs, teamSlug)

			// Replace team members wholesale
			db.Exec(ctx, "DELETE FROM team_members WHERE team_id=$1", teamID)
			membersRaw, _ := teamMap["members"].([]interface{})
			for _, m := range membersRaw {
				username, ok := m.(string)
				if !ok || username == "" {
					continue
				}
				if _, err := db.Exec(ctx,
					`INSERT INTO team_members (team_id, user_id)
					 SELECT $1, id FROM users WHERE username=$2
					 ON CONFLICT DO NOTHING`,
					teamID, username,
				); err != nil {
					log.Printf("operator: team %s member %s: %v", teamSlug, username, err)
				}
			}

			// Replace team permissions wholesale
			db.Exec(ctx, "DELETE FROM team_permissions WHERE team_id=$1", teamID)
			permsRaw, _ := teamMap["permissions"].([]interface{})
			for _, p := range permsRaw {
				permMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				resource, _ := permMap["resource"].(string)
				verbsRaw, _ := permMap["verbs"].([]interface{})
				verbs := make([]string, 0, len(verbsRaw))
				for _, v := range verbsRaw {
					if s, ok := v.(string); ok {
						verbs = append(verbs, s)
					}
				}
				if resource == "" || len(verbs) == 0 {
					continue
				}
				if _, err := db.Exec(ctx,
					`INSERT INTO team_permissions (team_id, resource, verbs) VALUES ($1, $2, $3)`,
					teamID, resource, verbs,
				); err != nil {
					log.Printf("operator: team %s permission %s: %v", teamSlug, resource, err)
				}
			}
		}

		// Delete teams removed from this org's spec
		if _, err := db.Exec(ctx,
			`DELETE FROM teams WHERE org_id=$1 AND slug != ALL($2::text[])`,
			orgID, declaredTeamSlugs,
		); err != nil {
			log.Printf("operator: BosunOrganization %s: delete orphan teams: %v", orgSlug, err)
		}
	}

	// Delete orgs removed from the cluster
	if _, err := db.Exec(ctx,
		`DELETE FROM organizations WHERE slug != ALL($1::text[])`,
		declaredOrgSlugs,
	); err != nil {
		return fmt.Errorf("delete orphan organizations: %w", err)
	}

	return nil
}

// ── Users ────────────────────────────────────────────────────────────────────

func reconcileCRDUsers(ctx context.Context, db *pgxpool.Pool, dyn dynamic.Interface, ns string) error {
	list, err := dyn.Resource(gvrUser).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCRDMissing(err) {
			return nil
		}
		return err
	}

	for _, item := range list.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		username, _ := spec["username"].(string)
		role, _ := spec["role"].(string)
		if username == "" {
			continue
		}
		if role == "" {
			role = "viewer"
		}

		tag, err := db.Exec(ctx,
			"UPDATE users SET role=$2 WHERE username=$1",
			username, role,
		)
		if err != nil {
			log.Printf("operator: BosunUser %s: db update: %v", item.GetName(), err)
			continue
		}
		if tag.RowsAffected() == 0 {
			log.Printf("operator: BosunUser %s: user %q not found in DB — create them via POST /api/v1/users first", item.GetName(), username)
		}
	}

	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// isCRDMissing returns true when the CRD definition hasn't been applied to the
// cluster yet. These errors are expected before make deploy-crds runs.
func isCRDMissing(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "the server could not find the requested resource")
}
