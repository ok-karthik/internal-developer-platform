# Plan: Backstage → GitHub Actions → Go CLI → PR (Phase 23)

**Executor:** `implementer` subagent (Sonnet). **Reviewer:** `reviewer` subagent (Opus).
**Planned by:** Opus, 2026-10-04. Background reading for the owner: `docs/backstage/LEARNING.md`.

- Worktree: `git worktree add ../idp-dispatch -b feat/backstage-dispatch-workflow main`.
- **One commit per Part** (4 commits). Do **not** push, open a PR, merge, or change GitHub settings.

## What this builds, in one picture

```
Developer fills the Backstage form (tenant, app, golden path, capabilities, system)
        │  step: github:actions:dispatch   (built-in Backstage action, no TypeScript)
        ▼
GitHub Actions: .github/workflows/scaffold-service.yaml   (workflow_dispatch)
        │  1. validate inputs          3. go run . add-service --output-root ../3-tenant-repos
        │  2. tenant exists, app new   4. commit on branch scaffold/<tenant>-<app>, open a PR
        ▼
Pull request with the new service under 3-tenant-repos/<tenant>/...
        │  human review + merge
        ▼
tenant-repos-ci-cd.yaml (already exists) builds the image and renders manifests → ArgoCD syncs
```

What the dispatch workflow **writes**: exactly what `make demo-add-service` writes today —
`workloads-repo/services/<app>/` (source, `catalog-info.yaml`), `workloads-repo/infra/services/<app>/<env>/`
(Terraform claims), `gitops-repo/services/<app>/<env>/` (`values.yaml`, ACK claims) — but on a
new branch, delivered as a PR instead of a local change.

---

## Scope — the only files that may change

| Path | Change |
|---|---|
| `.github/workflows/scaffold-service.yaml` | **new** |
| `docs/backstage/software-template.yaml` | rewritten |
| `docs/backstage/README.md` | rewritten "how Backstage connects" sections |
| `docs/adr/0016-backstage-dispatches-to-github-actions.md` | **new** |

---

## What was measured — do not re-derive (2026-10-04, `main` @ `622367a`)

| Fact | Value |
|---|---|
| Golden paths in `catalog.yaml` | `go-service-postgres` (go, [postgres]), `python-worker-s3` (python, [s3]) |
| Capabilities | `postgres`, `s3`, `iam` |
| `add-service` flags | `--tenant-name/-t`, `--app-name/-a`, `--golden-path`, `--runtime`, `--capabilities` (comma list), `--system/-s`, `--env` (default `dev`); root: `--catalog-root`, `--output-root`, `--force`, `--dry-run` |
| **CLI does not validate names** | no regex anywhere in `2-idp-scaffolder` (non-test). `--app-name ../../x` would write outside the tenant. The workflow MUST validate before calling the CLI |
| CLI skips existing files | prints `[SKIP]`; so "app already exists" must be checked by the workflow, not inferred |
| Repo Actions settings | `default_workflow_permissions: read`; **"Allow GitHub Actions to create and approve pull requests" is OFF** (`can_approve_pull_request_reviews: false`) → the PR step fails until the owner turns it on (owner step, below) |
| GitHub limitation | a PR opened with `GITHUB_TOKEN` does **not** trigger `pull_request` workflows, so `ci.yaml` will not run on it automatically |
| Existing workflows | `ci.yaml` (tests, smoke, Kyverno on PR), `tenant-repos-ci-cd.yaml` (on push to `main`: build image, render manifests, push) — unchanged by this plan |
| `workflow_dispatch` | only dispatchable once the file exists on the default branch — so the real end-to-end run is an owner step after merge |
| Backstage side (`../backstage/dev-portal`, separate repo) | backend already loads `@backstage/plugin-scaffolder-backend-module-github` (provides `github:actions:dispatch`); `app-config.yaml` integration uses `${GITHUB_TOKEN}`; catalog location already points at this repo's `docs/backstage/software-template.yaml` by absolute path |
| Current `software-template.yaml` | uncommitted Gemini edit in the owner's main checkout replaced the action with `debug:log`; it also uses the old `--team-name` flag, `icon: git`, and a `/dev` link. This plan replaces the file from `main` |
| `actionlint` | not installed; run it with `go run github.com/rhysd/actionlint/cmd/actionlint@latest` |

## Decisions already made — do not reopen

1. Backstage uses the built-in `github:actions:dispatch`. No custom TypeScript action, no REST API (ADR 0015).
2. The workflow opens a **PR**; it never pushes to `main`.
3. Inputs reach shell **only through `env:`**, never as `${{ inputs.x }}` inside `run:` (script-injection rule).
4. Name rule for tenant and app: `^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$` (DNS-label style, ≤ 40 chars). System: same rule, or empty.
5. Golden path is a `choice` input with the two catalog values; capabilities is a free string
   validated against `^(postgres|s3|iam)(,(postgres|s3|iam))*$` or empty.
6. Env is fixed to `dev` (promotion is a separate copy-values step, ADR 0007).

---

## Part 1 — `.github/workflows/scaffold-service.yaml`

```yaml
name: Scaffold a service (Backstage)

# Started by Backstage's github:actions:dispatch step (docs/backstage/software-template.yaml)
# or by hand: gh workflow run scaffold-service.yaml -f tenant_name=tenant-a -f app_name=app-b ...
# Runs the same Go CLI as `make demo-add-service` and delivers the result as a pull request.
on:
  workflow_dispatch:
    inputs:
      tenant_name:  { description: "Existing tenant (3-tenant-repos/<tenant>)", required: true,  type: string }
      app_name:     { description: "New service name",                          required: true,  type: string }
      golden_path:  { description: "Golden path from catalog.yaml",             required: true,  type: choice,
                      options: [go-service-postgres, python-worker-s3] }
      capabilities: { description: "Extra capabilities, comma-separated (optional)", required: false, type: string, default: "" }
      system:       { description: "Backstage system (optional)",               required: false, type: string, default: "" }
      requested_by: { description: "Backstage user who asked (for the PR body)", required: false, type: string, default: "unknown" }

permissions:
  contents: write
  pull-requests: write

concurrency:
  group: scaffold-${{ inputs.tenant_name }}-${{ inputs.app_name }}
  cancel-in-progress: false

jobs:
  scaffold:
    runs-on: ubuntu-latest
    env:
      TENANT: ${{ inputs.tenant_name }}
      APP: ${{ inputs.app_name }}
      GOLDEN_PATH: ${{ inputs.golden_path }}
      CAPABILITIES: ${{ inputs.capabilities }}
      SYSTEM: ${{ inputs.system }}
      REQUESTED_BY: ${{ inputs.requested_by }}
      FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true
    steps:
      - uses: actions/checkout@v7

      - name: Validate inputs
        run: |
          set -euo pipefail
          name_re='^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$'
          [[ "$TENANT" =~ $name_re ]] || { echo "::error::tenant_name '$TENANT' must match $name_re"; exit 1; }
          [[ "$APP"    =~ $name_re ]] || { echo "::error::app_name '$APP' must match $name_re"; exit 1; }
          [[ -z "$SYSTEM" || "$SYSTEM" =~ $name_re ]] || { echo "::error::system '$SYSTEM' must match $name_re"; exit 1; }
          [[ -z "$CAPABILITIES" || "$CAPABILITIES" =~ ^(postgres|s3|iam)(,(postgres|s3|iam))*$ ]] \
            || { echo "::error::capabilities '$CAPABILITIES' must be a comma list of postgres,s3,iam"; exit 1; }
          [[ -d "3-tenant-repos/$TENANT" ]] || { echo "::error::tenant '$TENANT' is not onboarded (no 3-tenant-repos/$TENANT)"; exit 1; }
          [[ ! -e "3-tenant-repos/$TENANT/workloads-repo/services/$APP" ]] \
            || { echo "::error::service '$APP' already exists in tenant '$TENANT'"; exit 1; }

      - uses: actions/setup-go@v7
        with:
          go-version: '1.26'
          cache-dependency-path: 2-idp-scaffolder/go.sum

      - name: Run the scaffolder
        working-directory: 2-idp-scaffolder
        run: |
          set -euo pipefail
          args=(add-service --catalog-root ../1-platform-catalog --output-root ../3-tenant-repos
                --tenant-name "$TENANT" --app-name "$APP" --golden-path "$GOLDEN_PATH")
          [[ -n "$CAPABILITIES" ]] && args+=(--capabilities "$CAPABILITIES")
          [[ -n "$SYSTEM" ]] && args+=(--system "$SYSTEM")
          go run . "${args[@]}"
          test -f "../3-tenant-repos/$TENANT/workloads-repo/services/$APP/catalog-info.yaml"

      - name: Open a pull request
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          set -euo pipefail
          branch="scaffold/$TENANT-$APP"
          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git switch -c "$branch"
          git add "3-tenant-repos/$TENANT"
          git commit -m "feat($TENANT): scaffold $APP ($GOLDEN_PATH)"
          git push -u origin "$branch"
          gh pr create --base main --head "$branch" \
            --title "feat($TENANT): scaffold $APP" \
            --body "Scaffolded by \`scaffold-service.yaml\` for **$REQUESTED_BY** (golden path \`$GOLDEN_PATH\`, capabilities \`${CAPABILITIES:-none}\`, system \`${SYSTEM:-none}\`).

          CI does not start by itself on PRs opened by GitHub Actions. To run \`ci.yaml\`, close and reopen this PR. After merge, \`tenant-repos-ci-cd.yaml\` builds the image and renders manifests."
          echo "### PR opened for $TENANT/$APP" >> "$GITHUB_STEP_SUMMARY"
```

Notes for the executor:
- `${{ inputs.* }}` appears only in `env:` and in `concurrency.group`. Nowhere in `run:`. Check with
  `grep -n 'inputs\.' .github/workflows/scaffold-service.yaml` — every hit must be in `env:`/`concurrency:`.
- Run `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/scaffold-service.yaml`
  (includes shellcheck if installed). Fix what it reports without changing behaviour; report each fix.
- **Test the shell locally** (you cannot dispatch it): copy the three `run:` bodies of the first
  two steps into a scratch script, export the env vars, and run from the worktree root with
  `--output-root` pointed at a temp copy. Cases, each with its exit code in the report:
  1. `TENANT=tenant-a APP=app-b GOLDEN_PATH=go-service-postgres` → exit 0, files written
  2. `APP=../../x` → exit 1 with the app_name error, nothing written
  3. `APP=app-a` (exists) → exit 1 "already exists"
  4. `TENANT=nope` → exit 1 "not onboarded"
  5. `CAPABILITIES='postgres;rm -rf /'` → exit 1 capabilities error
  6. `CAPABILITIES=s3 SYSTEM=sys-a` → exit 0, `catalog-info.yaml` has `system: sys-a`
  For the local test only, replace `../3-tenant-repos` with a temp copy of `3-tenant-repos`;
  never write into the real one. Do not test the PR step.

Commit: `feat(ci): add scaffold-service workflow that opens a PR from Backstage input`

## Part 2 — `docs/backstage/software-template.yaml`

Rewrite completely (from the `main` version in the worktree):

```yaml
# Backstage Software Template. Backstage does no scaffolding itself: its only step asks
# GitHub Actions to run .github/workflows/scaffold-service.yaml, which runs the Go CLI and
# opens a PR. See docs/backstage/LEARNING.md and ADR 0016.
apiVersion: scaffolder.backstage.io/v1beta3
kind: Template
metadata:
  name: idp-add-service
  title: Add a new service (golden path)
  description: Scaffolds a service into an existing tenant and opens a pull request.
  tags: [idp, golden-path]
spec:
  owner: platform-team
  type: service
  parameters:
    - title: Service
      required: [tenantName, appName, goldenPath]
      properties:
        tenantName:
          title: Tenant
          type: string
          default: tenant-a
          pattern: '^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$'
          description: Must already be onboarded (3-tenant-repos/<tenant>/).
        appName:
          title: App name
          type: string
          pattern: '^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$'
          description: Lowercase letters, digits and dashes.
        goldenPath:
          title: Golden path
          type: string
          default: go-service-postgres
          enum: [go-service-postgres, python-worker-s3]
          description: Kept in sync with 1-platform-catalog/catalog.yaml by hand.
    - title: Optional
      properties:
        capabilities:
          title: Extra capabilities
          type: array
          uniqueItems: true
          items: { type: string, enum: [postgres, s3, iam] }
          ui:widget: checkboxes
        system:
          title: Backstage system
          type: string
          pattern: '^([a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?)?$'
  steps:
    - id: dispatch
      name: Ask GitHub Actions to scaffold the service
      action: github:actions:dispatch
      input:
        repoUrl: github.com?owner=ok-karthik&repo=internal-developer-platform
        workflowId: scaffold-service.yaml
        branchOrTagName: main
        workflowInputs:
          tenant_name: ${{ parameters.tenantName }}
          app_name: ${{ parameters.appName }}
          golden_path: ${{ parameters.goldenPath }}
          capabilities: ${{ (parameters.capabilities or []) | join(',') }}
          system: ${{ parameters.system or '' }}
          requested_by: ${{ user.entity.metadata.name or 'guest' }}
  output:
    links:
      - title: Workflow runs (watch it here)
        icon: github
        url: https://github.com/ok-karthik/internal-developer-platform/actions/workflows/scaffold-service.yaml
      - title: Pull requests for this service
        icon: github
        url: https://github.com/ok-karthik/internal-developer-platform/pulls?q=is%3Apr+head%3Ascaffold%2F${{ parameters.tenantName }}-${{ parameters.appName }}
```

Check: the file parses (`python3 -c "import yaml; yaml.safe_load(open('docs/backstage/software-template.yaml'))"`),
every `workflowInputs` key equals an input name in Part 1, and the `enum`/`pattern` values equal
Part 1's. You cannot run Backstage; do not try.

Commit: `feat(backstage): template dispatches scaffold-service instead of a custom action`

## Part 3 — `docs/backstage/README.md`

Keep the title, the "Does Backstage replace the CLI? No" section and the design-rule section.
Replace the "Three ways to connect them" table and "What is actually here" with:
- a table of the approaches — (a) reimplement in TypeScript: rejected; (b) custom action running the
  CLI inside Backstage (`idp:run-cli`): **replaced** — needs TypeScript, Go in the Backstage image,
  and push credentials in Backstage; (c) REST API: not built (ADR 0015); **(d) `github:actions:dispatch`
  → `scaffold-service.yaml` → PR: used** (ADR 0016);
- the one-picture flow from the top of this plan;
- "What is here": `software-template.yaml`, `app-config.fragment.yaml` (unchanged description),
  `LEARNING.md` (plain-language guide), and the workflow path.
- the "7 of 577 postings" opening and the "Why nothing here is a running app" section: shorten to
  two sentences saying the Backstage app lives in its own repo (`backstage/dev-portal`) and its
  deployment will live under `4-platform-engineering/2-cluster-services/` later.

Commit: `docs(backstage): describe the dispatch-to-Actions flow`

## Part 4 — ADR 0016

`docs/adr/0016-backstage-dispatches-to-github-actions.md`, same header format as ADR 0015.
Context (the three options and their costs, from the README table), Decision (dispatch → workflow → PR),
Consequences: no TypeScript and no credentials in Backstage; Backstage reports "started", not
"succeeded" (backstage/backstage issues #29727, #33104), so the user follows the output links;
PRs opened by `GITHUB_TOKEN` do not trigger `ci.yaml` (reopen, or later a GitHub App token);
the CLI does not validate names, so the workflow does — moving that check into the CLI is a follow-up.
Revisit trigger: a platform API exists (then a thin custom action calls it), or many templates
need live progress in Backstage.

Commit: `docs(adr): 0016 — Backstage dispatches scaffolding to GitHub Actions`

---

## Rules this task can break

1. **Untrusted input never goes straight into a shell.** Inputs → `env:` → quoted `"$VAR"`.
2. **The workflow never pushes to `main`** — branch + PR only.
3. **Never write into the real `3-tenant-repos/` while testing** — temp copy only.
4. **Check exit codes and the file tree**, not the success message.
5. **Executors never push, merge, publish, or change repo settings.**
6. Fixture names `tenant-a` / `team-a` / `app-a` (and `app-b` for a new one).

## Verify sequence

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/scaffold-service.yaml; echo "actionlint exit=$?"
grep -n 'inputs\.' .github/workflows/scaffold-service.yaml          # only env:/concurrency: lines
python3 -c "import yaml;[yaml.safe_load(open(f)) for f in ['.github/workflows/scaffold-service.yaml','docs/backstage/software-template.yaml']];print('yaml-ok')"
# the 6 local shell cases from Part 1, with exit codes
git diff --name-only main...HEAD      # exactly the 4 Scope files
git status --short 3-tenant-repos/    # empty
```

## Done means

- [ ] 4 commits; actionlint clean (or each remaining warning explained); 6 shell cases give the expected exit codes.
- [ ] Report: commits, actionlint output, the 6 cases with exit codes, and what was not verified
      (the real dispatch, PR creation, Backstage).

## Owner steps after merge (not for the executor)

1. GitHub → repo Settings → Actions → General → Workflow permissions → tick
   **"Allow GitHub Actions to create and approve pull requests"**.
2. Try it without Backstage first:
   `gh workflow run scaffold-service.yaml -f tenant_name=tenant-a -f app_name=app-b -f golden_path=go-service-postgres`
   then `gh run watch` and open the PR. Close it without merging if it was only a test.
3. In `dev-portal`: make sure `GITHUB_TOKEN` is a fine-grained PAT with **Actions: read and write**
   on this repo, restart `yarn start`, run the template from the Create page.

## Out of scope

- Name validation inside the Go CLI (follow-up; the workflow guards it for now).
- A GitHub App token so `ci.yaml` runs on bot PRs.
- An `onboard-tenant` template.
- Deploying Backstage into the cluster.
