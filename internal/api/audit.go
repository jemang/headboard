package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// auditCategories groups action families the way the activity page filters
// them. A category can span families: keys covers both kinds of key.
var auditCategories = map[string][]string{
	"devices": {"device.", "registration."},
	"policy":  {"policy."},
	"people":  {"account.", "user."},
	"keys":    {"apikey.", "preauthkey."},
	"ssh":     {"ssh_"},
	"signin":  {"auth.", "sudo."},
}

type auditInput struct {
	Category string `query:"category" doc:"devices, policy, people, keys, ssh or signin; empty for everything"`
	Actor    string `query:"actor" doc:"exact actor label: an email, cli or agent:<name>"`
	Before   int64  `query:"before" doc:"cursor from the previous page's next"`
	Limit    int    `query:"limit" doc:"page size, at most 200" default:"50"`
}

type auditOutput struct {
	Body struct {
		Entries []store.AuditEntry `json:"entries"`
		// Next is the cursor for the following page, 0 when this is the last.
		Next int64 `json:"next"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "listAudit",
			Method:      http.MethodGet,
			Path:        "/api/audit",
			Summary:     "Who changed what, newest first",
			Tags:        []string{"audit"},
		}, func(ctx context.Context, in *auditInput) (*auditOutput, error) {
			p, err := require(ctx, auth.CapViewAudit)
			if err != nil {
				return nil, err
			}

			prefixes, ok := auditCategories[in.Category]
			if in.Category != "" && !ok {
				return nil, huma.Error422UnprocessableEntity("unknown category " + in.Category)
			}

			limit := in.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}

			// One extra row says whether another page exists without a count query.
			entries, err := deps.Store.ListAudit(ctx, store.AuditFilter{
				Prefixes: prefixes, Actor: in.Actor, Before: in.Before, Limit: limit + 1,
			})
			if err != nil {
				return nil, statusFor(err, "could not read the audit log")
			}

			if !p.Can(auth.CapManageUsers) {
				for i := range entries {
					if entries[i].Action == "auth.login_failed" {
						entries[i].ActorLabel = maskEmail(entries[i].ActorLabel)
					}
				}
			}

			out := &auditOutput{}
			if len(entries) > limit {
				entries = entries[:limit]
				out.Body.Next = entries[limit-1].ID
			}
			out.Body.Entries = entries
			if out.Body.Entries == nil {
				out.Body.Entries = []store.AuditEntry{}
			}

			return out, nil
		})
	})
}

// maskEmail keeps the first letter and the domain of an attempted sign-in
// email. Anything that is not an email (a password typed into the wrong box)
// is hidden whole.
func maskEmail(s string) string {
	if s == "" {
		return ""
	}

	at := strings.LastIndex(s, "@")
	if at < 1 {
		return "***"
	}

	return s[:1] + "***" + s[at:]
}
