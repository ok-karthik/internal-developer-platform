# Plan: the CLI validates every name it turns into a path or a Kubernetes name (Phase 24)

**Executor:** `implementer` subagent (Sonnet). **Reviewer:** `reviewer` subagent (Opus).
**Planned by:** Opus, 2026-10-04. Runs **before** Phase 23 (Phase 23 sends Backstage form input into this CLI).

## How to work (owner's rules for this repo)

- Work **in this checkout**: `/Users/karthik.orugonda/github/internal-developer-platform`, on `main`.
  No worktrees, no files outside the repo folder (temp output goes in `t.TempDir()` or `$(mktemp -d)`).
- **One local commit per Part.** Stage files by name (`git add <path>`), never `git add -A` —
  the owner may have unrelated uncommitted edits (e.g. `docs/backstage/LEARNING.md`); leave them alone.
- Do **not** push, open a PR, or merge.
- The owner is learning: the report must explain in plain words **what changed, why, and where
  the config/code lives** (file paths), not just list commits.

---

## The problem, in plain words

The CLI takes names from the user — tenant, app, system, env, owner — and uses them to build
**folder paths** (`3-tenant-repos/<tenant>/workloads-repo/services/<app>/`) and **Kubernetes names**
(namespace = tenant, Helm release and Service = app, ArgoCD Application = tenant-app-env-cluster).
Today it checks none of them (measured below), so:
- `--app-name ../../x` writes files outside the tenant folder (path traversal);
- `--app-name 'Bad Name: x'` exits 0 and writes a `catalog-info.yaml` that is not valid YAML;
- `--env ../prod` has the same traversal problem in the per-env paths.

Fix: one name rule, checked inside the templater at the two exported entry points that write
files, so every caller (CLI today, Backstage-via-Actions in Phase 23, any future API) is protected.

---

## Scope — the only files that may change

| Path | Change |
|---|---|
| `2-idp-scaffolder/internal/templater/names.go` | **new** — the rule and `validateNames` |
| `2-idp-scaffolder/internal/templater/names_test.go` | **new** — table tests |
| `2-idp-scaffolder/internal/templater/errors.go` | one new sentinel error |
| `2-idp-scaffolder/internal/templater/render.go` | call `validateNames` at the top of `RenderTenantFoundation` and `RenderService` |
| `2-idp-scaffolder/internal/templater/render_test.go` | one test: a bad name writes nothing |
| `.agents/AGENTS.md` | one bullet in "Go Scaffolder Conventions" |
| `2-idp-scaffolder/README.md` | one short "Name rules" section |
| `PLAN.md` | move the follow-up to "Recently done" (Part 3) |

No template changes, no fixture changes, no `cmd/cli/` changes (the CLI already prints a
returned error and exits 1 — see measured facts).

## What was measured — do not re-derive (2026-10-04, `main` @ `6f1e25d`)

| Fact | Value |
|---|---|
| Name checks in the Go code | **none** (no `regexp` in non-test `.go` files) |
| Reviewer reproduction (Phase 21 review) | `add-service --app-name 'Bad Name: x'` → exit 0, invalid YAML at the `name:` line |
| Where names come from | `Config{TenantName, SystemName, AppName, Env, Runtime, Capabilities, Owners}` (`render.go:18-25`) |
| Already validated | `Runtime` (against `catalog.yaml` runtimes, `resolve.go:44`), `Capabilities` (unknown → `ErrUnknownCapability`), golden path |
| Entry points that write files | `(*Renderer).RenderTenantFoundation` (`render.go:179`, used by `onboard-tenant`) and `(*Renderer).RenderService` (`render.go:191`, used by `add-service` after `Resolve`) |
| Existing pattern to copy | `RenderService` already guards an empty `Runtime` at the top with a `*ValidationError` — "exported methods validate their own inputs" (`.agents/AGENTS.md`, Go conventions) |
| Error type | `ValidationError{Field, Value, Err}` with sentinels in `errors.go`; message `field "value": err` |
| CLI behaviour on error | `RunE` returns the error → Cobra prints it, exit 1. `SilenceUsage: true` in `root.go` |
| Where names end up | tenant → namespace (DNS label, ≤63), app → Helm release (≤53) and Service name (DNS-1035: must **start with a letter**), ArgoCD Application `<tenant>-<app>-<env>-<cluster>`, Backstage `metadata.name` (≤63), GitHub team slug (owner) |
| Names used in tests/fixtures today | `tenant-a`, `app-a`, `team-a`, `payments`, `checkout`, `dev`, `sys-a`, `test-tenant`, `test-app` — all pass the rule below |
| Phase 23 workflow regex (planned) | will be changed to this same rule so there is one rule |

## Decisions already made — do not reopen

1. **One rule for all user-supplied names:** `^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`
   — lowercase letters, digits and dashes; starts with a letter; does not end with a dash; 1–40 chars.
   Why 40: leaves room for `<tenant>-<app>-<env>-<cluster>` and for Helm's 53-char release limit.
   Why start with a letter: Kubernetes Service names require it.
2. Checked fields: `TenantName` (required, both verbs), `AppName` (required in `RenderService`),
   `Env` (required in `RenderService`; `--env` defaults to `dev`), `SystemName` (only if non-empty),
   every entry of `Owners` (in `RenderTenantFoundation`).
3. Not checked here: `Runtime` and `Capabilities` (already checked against the catalog).
4. Validation lives in `internal/templater`, not in `cmd/cli/` — so a future API or any other
   caller gets it for free.
5. On failure **nothing is written** — validation runs before any rendering starts.

---

## Part 1 — the rule and its tests

**`errors.go`:** add `ErrInvalidName = errors.New("must be 1-40 chars of lowercase letters, digits and dashes, start with a letter, and not end with a dash")`.

**`names.go`** (new):
```go
package templater

import "regexp"

// nameRule is the one rule for every user-supplied name the scaffolder turns into a
// folder path or a Kubernetes/Backstage name. Starts with a letter (Kubernetes
// Service names require it); max 40 so "<tenant>-<app>-<env>-<cluster>" and Helm's
// 53-char release limit still fit.
var nameRule = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`)

// checkName returns a *ValidationError naming the field when value breaks nameRule.
func checkName(field, value string) error {
	if !nameRule.MatchString(value) {
		return &ValidationError{Field: field, Value: value, Err: ErrInvalidName}
	}
	return nil
}
```
Plus two small helpers, each returning the first error found:
- `validateTenantNames(cfg Config) error` — `tenant-name`, then each `owner`.
- `validateServiceNames(cfg Config) error` — `tenant-name`, `app-name`, `env`, then `system` if non-empty.

Field strings must equal the CLI flag names (`tenant-name`, `app-name`, `env`, `system`, `owner`) so
the error tells the user which flag to fix.

**`names_test.go`** (new), table-driven over `checkName`:
- valid: `a`, `app-a`, `tenant-a`, `team-a`, `dev`, `a1`, a 40-char name.
- invalid, each asserting `errors.Is(err, ErrInvalidName)`:
  `""`, `App-A` (uppercase), `-app` (leading dash), `app-` (trailing dash), `1app` (leading digit),
  `app_a`, `app.a`, `app a`, `Bad Name: x`, `../../x`, `../prod`, `a/b`, a 41-char name.
- one test each for `validateTenantNames` (bad owner → field `owner`) and `validateServiceNames`
  (empty system is allowed; bad system → field `system`; bad env → field `env`).

Run `cd 2-idp-scaffolder && go test ./internal/templater -run 'Name' -v` and read the output.

Commit: `feat(templater): add one name rule for tenant, app, env, system and owner`

## Part 2 — enforce it where files get written

**`render.go`:**
- First lines of `RenderTenantFoundation`: `if err := validateTenantNames(cfg); err != nil { return err }`.
- In `RenderService`, right after the existing empty-`Runtime` guard: `if err := validateServiceNames(cfg); err != nil { return err }`.
- Comment above each call: one line saying names become paths and Kubernetes names, so they are
  checked before anything is written.

**`render_test.go`:** add `TestRenderRejectsBadNamesAndWritesNothing`: table of
`{verb, cfg}` cases (tenant `../../x` for both verbs, app `Bad Name: x`, env `../prod`, owner `Team A`);
each renders into `t.TempDir()`, asserts `errors.Is(err, ErrInvalidName)`, and asserts the temp dir
is **empty** afterwards (walk it; zero files).

Then check the real CLI from `2-idp-scaffolder/` (output to a temp dir, never `3-tenant-repos/`):
```bash
O=$(mktemp -d); CAT=../1-platform-catalog
go run . onboard-tenant --catalog-root $CAT --output-root $O --tenant-name tenant-a --owner team-a; echo "exit=$?"   # 0
go run . add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name '../../x' --golden-path go-service-postgres; echo "exit=$?"     # 1
go run . add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name 'Bad Name: x' --golden-path go-service-postgres; echo "exit=$?" # 1
go run . add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name app-b --env '../prod' --golden-path go-service-postgres; echo "exit=$?" # 1
go run . onboard-tenant --catalog-root $CAT --output-root $O --tenant-name tenant-b --owner 'Team B'; echo "exit=$?"  # 1
go run . add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name app-b --golden-path go-service-postgres; echo "exit=$?"  # 0
find $O -type f | sed "s|$O/||" | sort      # only tenant-a/... files; no "x", no "prod", no tenant-b
```
Paste the printed error lines and exit codes into the report.

Full gate: `go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...` — all green,
and `go test ./internal/templater -run TestRenderService` still passes **without** `-update`
(golden files must not change).

Commit: `fix(templater): reject unsafe names before writing any file`

## Part 3 — docs

- `.agents/AGENTS.md`, section "Go Scaffolder Conventions": add one bullet —
  **Names are validated at the render boundary.** `RenderTenantFoundation` and `RenderService`
  check tenant, app, env, system and owner against `nameRule` in `internal/templater/names.go`
  before writing anything. Any new user-supplied name that reaches a path or a Kubernetes name
  must be added there.
- `2-idp-scaffolder/README.md`: a short "Name rules" section — the rule in words, the regex, why
  (paths + Kubernetes names), and one example error line copied from the Part 2 run.
- `PLAN.md`: delete the follow-up bullet "The Go CLI does not validate tenant/app names…" and add
  at the top of "Recently done": `- **Phase 24** (<date>) — the CLI validates tenant/app/env/system/owner names; unsafe names write nothing.`

Commit: `docs(scaffolder): document the name rule`

---

## Rules this task can break

1. **Never write into `3-tenant-repos/` while testing** — temp dirs only. `git status --short 3-tenant-repos/` must be empty at the end.
2. **Golden files and fixtures must not change.** All current names pass the rule; if a golden test fails, stop and report.
3. **Check the file tree, not the message** — "exit 1" is not enough; the temp dir must show nothing was written.
4. **Exported methods validate their own inputs** (existing repo rule) — do not put the check only in `cmd/cli/`.
5. Stage by file name; leave the owner's unrelated uncommitted edits untouched. No push, no PR.

## Verify sequence

```bash
cd 2-idp-scaffolder && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./... ; cd ..
git diff --name-only HEAD~3..HEAD          # only the Scope files
git status --short                          # only the owner's pre-existing edits, nothing new
git status --short 3-tenant-repos/          # empty
# the six CLI commands from Part 2, with exit codes and the find output
```

## Done means

- [ ] 3 local commits on `main`; all checks green; golden files untouched.
- [ ] The six CLI cases give the expected exit codes, and the bad ones wrote nothing.
- [ ] Report in plain words: what changed, why, where it lives (file paths), the error message a
      user now sees, and what was not verified.

## Out of scope

- Phase 23's workflow (it will use this same rule; its plan is updated separately).
- Renaming the deprecated hidden `--team-name` flag.
- Validating `catalog.yaml` content (already done at load).
