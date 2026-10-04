# Plan: close the four validation gaps the Phase 24 review found (Phase 24b)

**Executor:** `implementer` subagent (Sonnet). **Reviewer:** `reviewer` subagent (Opus).
**Planned by:** Opus, 2026-10-04.

## How to work (owner's rules for this repo)

- Work **in this checkout** (`/Users/karthik.orugonda/github/internal-developer-platform`) on `main`.
  No worktrees, no files outside the repo folder. Test output goes in `t.TempDir()`; CLI runs use
  `$(mktemp -d)` and delete it afterwards.
- **One local commit per Part** (4 commits), staging files by name. No push, no PR.
- Report in plain words: **what changed, why, where it lives**, the exact new error messages, and what was not verified.

## The four gaps, in plain words

| # | Gap | What goes wrong today |
|---|---|---|
| 1 | Names are checked one by one, but templates **join** them: S3 bucket and IAM role are named `<tenant>-<app>-<env>` | Three 40-char names give a 122-char bucket name. AWS allows 63 (bucket) / 64 (role). The CLI says success; the failure appears later in the AWS controller (ACK) |
| 2 | `RenderService` trusts `Runtime` | Safe from the CLI (`Resolve` checks it first), but a direct caller (future API) can pass `../../gitops` and files land in the wrong folder |
| 3 | Unknown capabilities are found **inside** the writing loop | `--capabilities bogus` exits 1 after 4 files were already written — a half-made service |
| 4 | `--owner` is optional | No owner → CODEOWNERS with no team, RoleBinding with no subjects |

All four are fixed the same way as Phase 24: **check everything first, write nothing on failure**, inside `internal/templater` so every caller is protected.

---

## Scope — the only files that may change

| Path | Change |
|---|---|
| `2-idp-scaffolder/internal/templater/names.go` | combined-length check; fix the wrong "still fits" comment |
| `2-idp-scaffolder/internal/templater/errors.go` | two sentinels: `ErrNameTooLong`, `ErrOwnerRequired` |
| `2-idp-scaffolder/internal/templater/render.go` | runtime + capability checks before rendering in `RenderService` |
| `2-idp-scaffolder/internal/templater/names_test.go` | tests for gaps 1 and 4 |
| `2-idp-scaffolder/internal/templater/render_test.go` | tests for gaps 2 and 3 (writes-nothing style) |
| `2-idp-scaffolder/README.md` | fix the "Name rules" reason; add the combined limit and the owner rule |
| `.agents/AGENTS.md` | extend the Phase 24 "Names are validated" bullet |
| `PLAN.md` | "Recently done" line (Part 4) |

No template, fixture, golden-file or `cmd/cli/` changes.

## What was measured — do not re-derive (2026-10-04, `main` @ `cf64241`)

| Fact | Value |
|---|---|
| Derived AWS names | `s3.yaml.tmpl:20,27` and `iam.yaml.tmpl:25,32`: `name: [[ .TenantName ]]-[[ .AppName ]]-[[ .Env ]]` |
| Reproduced | tenant, app, env = 40 × `a`, `--runtime go --capabilities s3` → exit 0, S3 `name` length **122** |
| AWS limits | S3 bucket name ≤ 63; IAM role name ≤ 64 → use **63** for `len(tenant)+len(app)+len(env)+2` |
| `postgres.tf.tmpl` | passes `team_name`/`app_name`/`env` to the external module; what name it builds is unknown (other repo). The 63 cap is the safe bet there too |
| Other derived names at 40 | Helm release = app (≤53 OK), `<app>-service` (48), `<app>-dev.local` label (44), namespace/`<tenant>-*` (≤54), Backstage owner `team-<tenant>` (45), ArgoCD Application (~133, OK under annotation tracking) — no change needed |
| `names.go` comment + `2-idp-scaffolder/README.md:34-35` | claim 40 keeps `<tenant>-<app>-<env>-<cluster>` "fitting" — wrong, must be corrected |
| `RenderService` today (`render.go:191`) | checks `Runtime == ""`, then `validateServiceNames`, then renders; runtime existence only checked in `Resolve` (`resolve.go:44`) |
| Capability check today | inside `for _, capName := range cfg.Capabilities` at `render.go:230-244`, **after** runtime/meta/release files are written |
| `--owner` | `onboard_tenant.go:40`, `StringSliceVar`, default empty, not required |
| Callers that pass `--owner` already | `Makefile` demo (`DEMO_OWNER`), CI smoke (`--owner test-owner`), both READMEs, Phase 23 plan does not call onboard-tenant |
| Tests calling `RenderTenantFoundation` | `render_test.go:515` (`Owners: []string{"team-a"}`) — already passes an owner |
| Reviewer's lesson (now in `plan-task` skill) | a writes-nothing test must count files **one level above** the output dir (traversal escapes it) |

## Decisions already made — do not reopen

1. Combined cap: `len(TenantName) + len(AppName) + len(Env) + 2 <= 63`, checked in `validateServiceNames`
   after the per-field checks. Error field: `tenant-name+app-name+env`, value: the joined name, so the user sees what was too long.
2. Per-field rule (`nameRule`, 1–40) stays as is.
3. Runtime check at the top of `RenderService`: `cfg.Runtime` must be a key of `r.Spec.Runtimes`,
   else `&ValidationError{Field: "runtime", Value: cfg.Runtime, Err: ErrUnknownRuntime}` — the same error `Resolve` returns.
4. Capability check: before rendering in `RenderService`, loop all `cfg.Capabilities` against
   `r.Spec.Capabilities`; return the existing `ErrUnknownCapability` error. Leave the in-loop check in place (harmless, keeps the loop safe on its own).
5. Owner: `validateTenantNames` returns `&ValidationError{Field: "owner", Err: ErrOwnerRequired}` when `len(cfg.Owners) == 0`.
   `ErrOwnerRequired = errors.New("at least one --owner is required (it becomes CODEOWNERS and the RoleBinding subject)")`.

---

## Part 1 — combined length (gap 1)

- `errors.go`: `ErrNameTooLong = errors.New("tenant-app-env is longer than 63 characters, the AWS limit for the S3 bucket and IAM role names built from it")`.
- `names.go`: add `const maxDerivedName = 63` with a comment naming `s3.yaml.tmpl`/`iam.yaml.tmpl` and the AWS limits;
  at the end of `validateServiceNames`, compute `joined := cfg.TenantName + "-" + cfg.AppName + "-" + cfg.Env`
  and return `&ValidationError{Field: "tenant-name+app-name+env", Value: joined, Err: ErrNameTooLong}` if `len(joined) > maxDerivedName`.
- Rewrite the `nameRule` comment: starts with a letter (Kubernetes Service names); max 40 per name keeps
  Helm's 53-char release limit and DNS labels safe; **the joined `<tenant>-<app>-<env>` has its own 63 cap** (see `maxDerivedName`).
- `names_test.go`: `tenant-a`/`app-a`/`dev` passes; 20+20+21 (joined 63) passes; 20+20+22 (joined 64) fails with `ErrNameTooLong`; 40+40+40 fails.

Commit: `fix(templater): cap the joined tenant-app-env name at 63 for AWS resource names`

## Part 2 — runtime and capabilities checked before writing (gaps 2 and 3)

- `render.go`, `RenderService`, right after `validateServiceNames`: the runtime check (decision 3), then the
  capability pre-check (decision 4). Comment: everything is checked before the first file is written.
- `render_test.go`: extend `TestRenderRejectsBadNamesAndWritesNothing` (or add a sibling test using the same
  count-above-the-output-dir helper) with:
  - `RenderService` called **directly** (no `Resolve`) with `Runtime: "../../gitops"` → `ErrUnknownRuntime`, zero files;
  - `Runtime: "go"`, `Capabilities: []string{"postgres", "bogus"}` → `ErrUnknownCapability`, zero files.
- Mutation check (do it, then restore): move the capability pre-check back out → the `bogus` case must fail
  with files written. Report the failure message.

Commit: `fix(templater): check runtime and capabilities before writing any file`

## Part 3 — owner required (gap 4)

- `errors.go`: `ErrOwnerRequired` (decision 5). `names.go`: the empty-owners check at the start of the owner part of `validateTenantNames`.
- `names_test.go`: `Owners: nil` → `ErrOwnerRequired`, field `owner`.
- Check no existing test or CLI call breaks: `grep -rn "onboard-tenant" Makefile .github README.md 2-idp-scaffolder/README.md`
  — every call must already pass `--owner`; list them in the report.

Commit: `fix(templater): require at least one owner when onboarding a tenant`

## Part 4 — docs

- `2-idp-scaffolder/README.md` "Name rules": replace the wrong "still fit" sentence with:
  per-name rule (40) + the joined `<tenant>-<app>-<env>` ≤ 63 because it becomes the S3 bucket and IAM role
  name; `onboard-tenant` needs at least one `--owner`. Paste one real error line for each from the Part 5 run.
- `.agents/AGENTS.md`: extend the "Names are validated at the render boundary" bullet — the joined cap, runtime
  and capabilities checked up front, owner required; "a validation failure writes nothing".
- `PLAN.md` "Recently done", top line: `- **Phase 24b** (<date>) — joined-name cap (63), runtime/capabilities checked before writing, owner required.`

Commit: `docs(scaffolder): document the joined-name cap and up-front checks`

## Verify sequence (after Part 4)

```bash
cd 2-idp-scaffolder && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...
B=$(mktemp -d); O=$(mktemp -d); go build -o $B/idp . ; CAT=../1-platform-catalog
L40=$(printf 'a%.0s' {1..40})
$B/idp onboard-tenant --catalog-root $CAT --output-root $O --tenant-name tenant-a; echo "no owner exit=$?"          # 1
$B/idp onboard-tenant --catalog-root $CAT --output-root $O --tenant-name tenant-a --owner team-a; echo "exit=$?"    # 0
$B/idp add-service --catalog-root $CAT --output-root $O -t $L40 --app-name $L40 --env $L40 --runtime go; echo "long exit=$?"   # 1
$B/idp add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name app-b --golden-path go-service-postgres --capabilities bogus; echo "bogus exit=$?" # 1
$B/idp add-service --catalog-root $CAT --output-root $O -t tenant-a --app-name app-b --golden-path go-service-postgres --capabilities s3; echo "ok exit=$?"     # 0
find $O -path '*app-b*' -type f | wc -l     # files only from the last (good) run
ls $O                                         # only tenant-a
rm -rf $B $O; cd ..
go test -count=1 ./2-idp-scaffolder/... >/dev/null 2>&1 || (cd 2-idp-scaffolder && go test ./internal/templater -run TestRenderService)  # golden unchanged, no -update
git status --short; git status --short 3-tenant-repos/      # both empty
git diff --name-only HEAD~4..HEAD                            # only Scope files
```
Note: run the `bogus` case **before** the good `app-b` run, and check (e.g. with `find` between the two) that
the `bogus` run wrote zero `app-b` files.

## Done means

- [ ] 4 local commits; gate green; golden files and fixtures unchanged.
- [ ] The five CLI cases give the expected exit codes; `bogus` and the long names wrote nothing.
- [ ] Mutation check in Part 2 done and restored.
- [ ] Report in plain words (what/why/where, the new error lines, what was not verified).

## Out of scope

- Changing S3/IAM templates to shorten or hash names.
- What the external `postgres` module builds from `team_name`/`app_name`/`env`.
- ArgoCD `resourceTrackingMethod` pinning and the `targetRevision: '*'` chart version.
- Removing the now-unreachable `env := "dev"` fallback in `resolveDestination`.
