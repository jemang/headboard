package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/acl"
	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// PolicyBody is the ACL document plus everything the editor needs around it.
type PolicyBody struct {
	// HuJSON is the document as stored, comments and all.
	HuJSON string `json:"hujson"`

	// SHA256 identifies this exact text. Send it back with a write and the
	// server refuses if the document changed underneath — Headplane has no
	// such guard, so two admins silently clobber each other.
	SHA256 string `json:"sha256"`

	// Schema is the same document decoded for the form.
	Schema *acl.Schema `json:"schema,omitempty"`

	// Tokens is the picker vocabulary: only aliases that actually resolve.
	Tokens acl.Tokens `json:"tokens"`

	// Editable is false when Headscale runs policy.mode = file, in which
	// case writes are rejected upstream and the UI should say so rather
	// than letting someone compose an edit that cannot be saved.
	Editable bool `json:"editable"`

	// ParseError is set when the stored document does not parse. The raw
	// text is still returned so it can be fixed in the editor.
	ParseError string `json:"parseError,omitempty"`
}

type policyOutput struct {
	Body PolicyBody
}

type policyWriteInput struct {
	Body struct {
		// HuJSON replaces the whole document, for the raw editor.
		HuJSON string `json:"hujson,omitempty"`

		// Ops patches it instead, for the form. Comments outside the
		// patched values survive.
		Ops []acl.Op `json:"ops,omitempty"`

		// SHA256 is the hash the caller last read.
		SHA256 string `json:"sha256"`

		Note string `json:"note,omitempty"`
	}
}

type policyPreviewOutput struct {
	Body struct {
		HuJSON string   `json:"hujson"`
		Diff   acl.Diff `json:"diff"`
		Valid  bool     `json:"valid"`
		Error  string   `json:"error,omitempty"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "getPolicy",
			Method:      http.MethodGet,
			Path:        "/api/policy",
			Summary:     "The ACL policy, decoded for the form and raw for the editor",
			Tags:        []string{"policy"},
		}, func(ctx context.Context, _ *struct{}) (*policyOutput, error) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			return loadPolicy(ctx, deps)
		})

		// Preview is separate from save on purpose: an ACL mistake locks
		// people out of machines, so the confirm step shows the real diff
		// and the real validation result before anything is written.
		huma.Register(api, huma.Operation{
			OperationID: "previewPolicy",
			Method:      http.MethodPost,
			Path:        "/api/policy/preview",
			Summary:     "Validate a change and show the diff, without saving",
			Tags:        []string{"policy"},
		}, func(ctx context.Context, in *policyWriteInput) (*policyPreviewOutput, error) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			current, next, err := applyChange(ctx, deps, in)
			if err != nil {
				return nil, err
			}

			out := &policyPreviewOutput{}
			out.Body.HuJSON = next
			out.Body.Diff = acl.Unified(current, next)
			out.Body.Valid = true

			if err := deps.Mutator.CheckPolicy(ctx, next); err != nil {
				out.Body.Valid = false
				out.Body.Error = err.Error()
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "savePolicy",
			Method:      http.MethodPut,
			Path:        "/api/policy",
			Summary:     "Save the ACL policy",
			Description: "Requires the sha256 of the document you last read. A mismatch means " +
				"someone else saved in the meantime and the write is refused.",
			Tags: []string{"policy"},
		}, func(ctx context.Context, in *policyWriteInput) (*policyOutput, error) {
			p, err := require(ctx, auth.CapManagePolicy)
			if err != nil {
				return nil, err
			}

			current, next, err := applyChange(ctx, deps, in)
			if err != nil {
				return nil, err
			}

			if current == next {
				return nil, huma.Error409Conflict("that change would not alter the policy")
			}

			// Validate before writing. Headscale would reject it too,
			// but a rejected PUT gives a worse message than an explicit
			// check does.
			if err := deps.Mutator.CheckPolicy(ctx, next); err != nil {
				return nil, huma.Error422UnprocessableEntity("the policy is not valid", err)
			}

			if _, err := deps.Mutator.SetPolicy(ctx, next); err != nil {
				return nil, upstream(err, "could not save the policy")
			}

			// Snapshot after the write succeeds. This is what makes
			// rollback possible: Headscale keeps only the current
			// document.
			if _, err := deps.Store.SavePolicyRevision(ctx, store.PolicyRevision{
				SHA256:       acl.SHA256(next),
				Body:         next,
				AuthorUserID: &p.User.ID,
				Note:         in.Body.Note,
			}); err != nil {
				deps.Log.Error("policy saved but the revision snapshot failed", "err", err)
			}

			diff := acl.Unified(current, next)

			finish(ctx, deps, p, "policy.save", "policy", 0,
				map[string]any{"sha256": acl.SHA256(current)},
				map[string]any{
					"sha256":  acl.SHA256(next),
					"added":   diff.Added,
					"removed": diff.Removed,
					"note":    in.Body.Note,
				})

			snap, err := currentSnapshot(deps)
			if err != nil {
				return nil, err
			}

			// loadPolicy would read the old watcher snapshot here. Decode the
			// just-written text for this response, then refresh shared state for
			// every other policy consumer.
			out, err := policyOutputFor(next, snap.Users)
			if err != nil {
				return nil, err
			}

			deps.Tailnet.Invalidate()

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "listPolicyRevisions",
			Method:      http.MethodGet,
			Path:        "/api/policy/revisions",
			Summary:     "Policy history",
			Tags:        []string{"policy"},
		}, func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Revisions []store.PolicyRevision `json:"revisions"`
			}
		}, error,
		) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			revs, err := deps.Store.ListPolicyRevisions(ctx, 50)
			if err != nil {
				return nil, statusFor(err, "could not list policy revisions")
			}

			out := &struct {
				Body struct {
					Revisions []store.PolicyRevision `json:"revisions"`
				}
			}{}
			out.Body.Revisions = revs

			if out.Body.Revisions == nil {
				out.Body.Revisions = []store.PolicyRevision{}
			}

			return out, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "getPolicyRevision",
			Method:      http.MethodGet,
			Path:        "/api/policy/revisions/{id}",
			Summary:     "One stored policy revision, including its text",
			Tags:        []string{"policy"},
		}, func(ctx context.Context, in *struct {
			ID int64 `path:"id"`
		},
		) (*struct{ Body store.PolicyRevision }, error) {
			if _, err := require(ctx, auth.CapManagePolicy); err != nil {
				return nil, err
			}

			rev, err := deps.Store.PolicyRevision(ctx, in.ID)
			if err != nil {
				return nil, huma.Error404NotFound("no such revision")
			}

			return &struct{ Body store.PolicyRevision }{Body: rev}, nil
		})
	})
}

// loadPolicy reads the current document from the snapshot and decodes it.
func loadPolicy(ctx context.Context, deps Deps) (*policyOutput, error) {
	snap, err := currentSnapshot(deps)
	if err != nil {
		return nil, err
	}

	return policyOutputFor(snap.Policy.HuJSON, snap.Users)
}

// policyOutputFor decodes a specific policy text for the editor. Saves call it
// with their confirmed write so the browser never receives a stale snapshot.
func policyOutputFor(hujson string, users []types.User) (*policyOutput, error) {
	out := &policyOutput{}
	body, err := policyBodyFor(hujson, users)
	if err != nil {
		return nil, err
	}

	out.Body = body

	return out, nil
}

func policyBodyFor(hujson string, users []types.User) (PolicyBody, error) {
	out := PolicyBody{}
	out.HuJSON = hujson
	out.SHA256 = acl.SHA256(hujson)
	out.Editable = true

	usernames := make([]string, 0, len(users))
	for _, u := range users {
		usernames = append(usernames, u.Name)
	}

	doc, err := acl.Parse(hujson)
	if err != nil {
		if errors.Is(err, acl.ErrEmpty) {
			// A fresh Headscale has no policy. The form should open
			// on an empty document rather than an error.
			empty := &acl.Schema{}
			out.Schema = empty
			out.Tokens = empty.TokensFor(usernames)

			return out, nil
		}

		// Return the text anyway: the editor is where this gets fixed.
		out.ParseError = err.Error()

		return out, nil
	}

	schema, err := doc.Schema()
	if err != nil {
		out.ParseError = err.Error()

		return out, nil
	}

	out.Schema = schema
	out.Tokens = schema.TokensFor(usernames)

	return out, nil
}

// applyChange resolves a write request into the current and proposed documents,
// enforcing the concurrency guard.
func applyChange(ctx context.Context, deps Deps, in *policyWriteInput) (current, next string, err error) {
	snap, snapErr := currentSnapshot(deps)
	if snapErr != nil {
		return "", "", snapErr
	}

	current = snap.Policy.HuJSON

	if in.Body.SHA256 == "" {
		return "", "", huma.Error422UnprocessableEntity(
			"sha256 is required: send back the hash you read, so a concurrent edit can be detected")
	}

	// The guard. Headscale's updatedAt has second resolution, which is
	// exactly the window two admins collide in, so this compares content.
	if in.Body.SHA256 != snap.PolicySHA256 {
		return "", "", huma.Error409Conflict(
			"the policy changed since you loaded it; reload and reapply your edit")
	}

	switch {
	case in.Body.HuJSON != "" && len(in.Body.Ops) > 0:
		return "", "", huma.Error422UnprocessableEntity(
			"send either hujson or ops, not both")

	case in.Body.HuJSON != "":
		doc, err := acl.Parse(in.Body.HuJSON)
		if err != nil {
			return "", "", huma.Error422UnprocessableEntity("the document does not parse", err)
		}

		return current, doc.Text(), nil

	case len(in.Body.Ops) > 0:
		doc, err := acl.Parse(current)
		if err != nil {
			return "", "", huma.Error409Conflict(
				"the stored policy does not parse, so it cannot be patched; edit it as text instead", err)
		}

		patched, err := doc.Patch(in.Body.Ops)
		if err != nil {
			return "", "", huma.Error422UnprocessableEntity("the patch does not apply", err)
		}

		return current, patched.Text(), nil

	default:
		return "", "", huma.Error422UnprocessableEntity("send either hujson or ops")
	}
}
