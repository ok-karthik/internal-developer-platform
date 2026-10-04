# Plan: Go-only scaffolder (retire the Python engine, flatten `golang/`)

**Executor:** `implementer` subagent (Sonnet). **Reviewer:** `reviewer` subagent (Opus).
**Planned by:** Opus, 2026-10-04.

- Work in a **separate git worktree**, never in the main checkout. The main checkout has the
  owner's uncommitted Phase 21 work staged (`PLAN.md`, `3-tenant-repos/.../app-a/catalog-info.yaml`)
  and must not be touched:
  ```bash
  git -C /Users/karthik.orugonda/github/internal-developer-platform worktree add \
      ../idp-go-only -b refactor/go-only-scaffolder main
  cd /Users/karthik.orugonda/github/internal-developer-platform/../idp-go-only
  ```
  All paths below are relative to that worktree's root.
- **One commit per Part** (Parts 1–5), conventional-commit messages as given in each Part.
- **Do NOT** push, open a PR, merge, or delete the worktree. The owner does that after review.
- **Do NOT** edit `PLAN.md`. Its old phases mention `python/` and `golang/` paths — they are
  history and stay as written.

---

## Scope — the only files that may change

| Path | Change |
|---|---|
| `2-idp-scaffolder/python/` (whole tree) | deleted |
| `2-idp-scaffolder/golang/*` (all tracked files) | moved up one level into `2-idp-scaffolder/` with `git mv` |
| `2-idp-scaffolder/internal/templater/render_test.go` | one relative path constant |
| `2-idp-scaffolder/CODE_WALKTHROUGH.md` | paths + one relative link |
| `2-idp-scaffolder/README.md` | **new** — short README for the scaffolder |
| `.github/workflows/ci.yaml` | Go paths; remove two Python jobs; add a Go smoke-run job |
| `Makefile` | remove `install-scaffolder` and `run-api`; repoint two `cd` lines |
| `.gitignore` | two lines repointed |
| `README.md` | scaffolder sections rewritten for one engine |
| `.agents/AGENTS.md` | Python sections removed, Go paths repointed |
| `docs/backstage/README.md`, `docs/backstage/software-template.yaml` | path text |
| `1-platform-catalog/README.md` | one sentence (line 47) |
| `docs/adr/0015-go-only-scaffolder.md` | **new** ADR |

Nothing else. In particular: no Go source changes other than `render_test.go`, no template
changes under `1-platform-catalog/`, no changes under `3-tenant-repos/` or `4-platform-engineering/`.

---

## What was measured — do not re-derive (2026-10-04, on `main` @ `0688f58`)

| Fact | Value |
|---|---|
| Python engine size | 789 lines of `.py` (excluding `.venv`) |
| Go engine size | 1,594 lines of `.go` |
| Go baseline | `go build ./... && go vet ./... && gofmt -l .` clean; `go test -count=1 ./...` → `internal/catalog` ok, `internal/templater` ok |
| Go module path | `module scaffolder` in `go.mod` — **not** path-based, so moving the directory needs no import changes |
| Only relative-path code in Go | `internal/templater/render_test.go:20` `const catalogDir = "../../../../1-platform-catalog"` (4 levels up → becomes 3) |
| CLI catalog discovery | `cmd/cli/root.go` uses `--catalog-root` or fetches from GitHub; no relative path to the repo — unaffected |
| Tracked non-`.go` files under `golang/` | `.golangci.yml`, `CODE_WALKTHROUGH.md`, `LICENSE`, `go.mod`, `go.sum`, 4 files under `internal/templater/testdata/` |
| Untracked build output under `golang/` | `idp-cli`, `scaffolder` (both Go build archives), `bin/scaffolder` — delete, do not move |
| `golang/TODO.md` | does **not** exist (docs/backstage/README.md:57 and AGENTS.md refer to a "Go TODO" anyway) |
| Who calls Python today | `Makefile:269` (`install-scaffolder`), `Makefile:272` (`run-api`), CI jobs `test-python-engine` and `verify-engine-parity`. Nothing else. Backstage template uses Go. |
| Branch protection on `main` | none (`gh api .../protection` → 404, rulesets `[]`), so removing CI jobs breaks no required check |
| Highest ADR number | `0014-observability-boundary.md` → new one is **0015** |
| `renovate.json` | no reference to the scaffolder |
| README scaffold example | stale: uses `onboard-team --team-name payments` / `checkout-api`; the real verbs are `onboard-tenant --tenant-name` / `add-service --tenant-name --app-name` (see `ci.yaml:69-70`) |

## Decisions already made — do not reopen

1. **Delete Python, do not archive it.** No `archived-python/` folder. Git history is the archive.
2. **Flatten**: Go code lives directly in `2-idp-scaffolder/` (no `golang/` subfolder).
3. **No Go REST API in this task.** Backstage calls the CLI (custom action `idp:run-cli`, as
   `docs/backstage/README.md` already designs). CI calls the CLI. A REST API (`cmd/api/`) is
   added only when a second consumer needs one. The ADR records this.
4. **Demo fixture names** in any new example text: `tenant-a`, `team-a`, `app-a` — never
   `payments`/`checkout`. (Existing testdata under `internal/templater/testdata/render_service/payments/`
   stays as it is — renaming it is out of scope.)

---

## Part 1 — Delete the Python engine

```bash
git rm -r -q 2-idp-scaffolder/python
rm -rf 2-idp-scaffolder/python      # removes leftover untracked __pycache__/.venv if any
```
Commit: `refactor(scaffolder): delete the Python engine`

## Part 2 — Flatten `golang/` into `2-idp-scaffolder/`

```bash
rm -rf 2-idp-scaffolder/golang/idp-cli 2-idp-scaffolder/golang/scaffolder 2-idp-scaffolder/golang/bin
cd 2-idp-scaffolder
for f in $(git ls-tree --name-only HEAD golang/); do git mv "$f" .; done
# git ls-tree prints "golang/<name>"; git mv golang/<name> . moves it up. Includes dotfiles (.golangci.yml).
rmdir golang          # must succeed — if not empty, STOP and report what is left
cd ..
```

Then fix the one relative path:
- `2-idp-scaffolder/internal/templater/render_test.go:20`
  `"../../../../1-platform-catalog"` → `"../../../1-platform-catalog"`

Fix `2-idp-scaffolder/CODE_WALKTHROUGH.md`:
- line 3: `` (`2-idp-scaffolder/golang/`) `` → `` (`2-idp-scaffolder/`) ``
- line 80: `2-idp-scaffolder/golang/` → `2-idp-scaffolder/`
- line 102: link `../../docs/adr/0010-tenant-repository-topology.md` → `../docs/adr/0010-tenant-repository-topology.md`
- grep the file for any other `golang/` or `python` mention and fix it the same way.

Fix `.gitignore` lines 16 and 18:
- `2-idp-scaffolder/golang/scaffolder` → `2-idp-scaffolder/scaffolder`
- `2-idp-scaffolder/golang/idp-cli` → `2-idp-scaffolder/idp-cli`
- add `2-idp-scaffolder/bin/` directly below them.

Run `cd 2-idp-scaffolder && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./...`
— must match the baseline (two `ok` packages) before committing.

Commit: `refactor(scaffolder): move the Go engine up to 2-idp-scaffolder/`

## Part 3 — CI and Makefile

### `.github/workflows/ci.yaml`
- `test-go-engine`: `cache-dependency-path: 2-idp-scaffolder/go.sum`,
  `working-directory: 2-idp-scaffolder`.
- **Delete** the whole `test-python-engine` job.
- **Replace** the whole `verify-engine-parity` job with this (keeps the end-to-end smoke run
  that the parity job used to give, without Python):

```yaml
  smoke-test-go-cli:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version: '1.26'
          cache-dependency-path: 2-idp-scaffolder/go.sum

      - name: Build Go CLI
        working-directory: 2-idp-scaffolder
        run: go build -o idp-cli .

      - name: Scaffold a tenant and a service
        run: |
          mkdir -p /tmp/go-output
          ./2-idp-scaffolder/idp-cli onboard-tenant --tenant-name test-tenant --owner test-owner --catalog-root ./1-platform-catalog --output-root /tmp/go-output
          ./2-idp-scaffolder/idp-cli add-service --tenant-name test-tenant --app-name test-app --golden-path go-service-postgres --catalog-root ./1-platform-catalog --output-root /tmp/go-output

      - name: Assert output was written
        run: |
          test -f /tmp/go-output/test-tenant/workloads-repo/services/test-app/catalog-info.yaml
          test -d /tmp/go-output/test-tenant/gitops-repo/services/test-app
```

  Before committing, run those two CLI commands locally and confirm both `test` paths exist.
  If a path differs, use the path the CLI really writes and say so in the report — do not
  change Go code to fit.
- Leave every other job untouched.

### `Makefile`
- Delete the `install-scaffolder:` and `run-api:` targets (lines 268–272 and the blank line after).
- Remove `install-scaffolder` and `run-api` from `.PHONY` and from any `help` echo text if present
  (grep the Makefile for both names; zero hits must remain).
- `demo-onboard-tenant` / `demo-add-service`: `cd 2-idp-scaffolder/golang` → `cd 2-idp-scaffolder`.

Commit: `ci(scaffolder): drop Python jobs, add a Go CLI smoke test, repoint paths`

## Part 4 — Docs

### `README.md`
- Line 15: `A CLI (Go, with a Python twin)` → `A Go CLI`.
- Line 28 (mermaid): `(Go + Python CLI)` → `(Go CLI)`.
- Line 58 TOC entry `[One Catalog, Two Engines](#-one-catalog-two-engines)`: remove.
- Lines 75–79 tree: `both scaffolder engines read` → `the scaffolder reads`; replace the
  `2-idp-scaffolder/` block with a single line:
  `├── 2-idp-scaffolder/          # THE SELF-SERVICE CLI — Go + Cobra (ADR 0015)`
- "Scaffold a service" (lines ~125–149): rewrite the commands to the real verbs and fixture names:
  ```bash
  cd 2-idp-scaffolder

  # Step 1 — once per tenant: namespace, network policy, RBAC, etc.
  go run . onboard-tenant --catalog-root ../1-platform-catalog \
                          --tenant-name tenant-a --owner team-a

  # Step 2 — once per service: source code + deployment config
  go run . add-service --catalog-root ../1-platform-catalog \
                       --tenant-name tenant-a --app-name app-a \
                       --golden-path go-service-postgres
  ```
  Or point at `make demo-onboard-tenant` / `make demo-add-service`. Delete the Python block
  and the sentence introducing it. In the "Heads up" note: `Neither engine is idempotent yet`
  → `The CLI is not idempotent yet`.
- Section `## 🔬 One Catalog, Two Engines` (~lines 367–381): replace the whole section with
  a short `## 🔬 Keeping the Catalog Honest` section (3–5 sentences) saying: the catalog is
  checked at load time (`internal/catalog` validates required keys), the templater has golden-file
  tests (`internal/templater/testdata/`), and CI scaffolds a real tenant + service on every PR
  (`smoke-test-go-cli`). A Python twin used to check this by byte-for-byte diff; it was
  retired in ADR 0015 because keeping two engines in sync cost more than it caught.
- Line ~472–474 Known Limitations: `Neither scaffolder engine is idempotent` → `The scaffolder
  is not idempotent`; `the next items on both engines' TODO.md` → `the next items for the scaffolder`.

### `.agents/AGENTS.md`
- Tree (lines ~28–35): make `2-idp-scaffolder/` show the Go layout directly
  (`cmd/cli/`, `internal/templater/`, `internal/catalog/`), no `golang/`, no `python/` lines.
- Lines ~112–121 ("Resolved:" paragraph): replace the engine file list with
  `internal/catalog/catalog.go` and `internal/templater/render.go`; `Both scaffolder engines` → `The scaffolder`.
- Line 201 bullet "Both engines must receive the same template fields": delete.
- `### 2. Go Scaffolder Conventions (2-idp-scaffolder/golang/)` → `(2-idp-scaffolder/)`.
- `### 3. Python Scaffolder Conventions`: delete the section; renumber the next heading
  (`### 4. Terraform…` → `### 3. Terraform…`).
- Line ~258 "Verified offline only: both engines byte-identical" — leave (it is history of Phase 18).
- Execution commands: `### Go Scaffolder (2-idp-scaffolder/golang/)` → `(2-idp-scaffolder/)`;
  `CAT=../../1-platform-catalog` → `CAT=../1-platform-catalog`; replace `onboard-team ... -t payments`
  / `-a checkout` examples with `onboard-tenant --tenant-name tenant-a --owner team-a` and
  `add-service --tenant-name tenant-a --app-name app-a` (check flag names with `go run . add-service --help`).
- Delete `### Python Scaffolder` command block.
- Line ~298: `/tmp/wt/2-idp-scaffolder/golang` → `/tmp/wt/2-idp-scaffolder`.
- Delete `### The two-engine acceptance test` subsection.
- Item 11 (~line 345): `Today both engines render` → `Today the scaffolder renders`; drop
  `Neither engine has` → `It has no`; drop the clause about the Python TODO and the
  `plan_service(cfg) -> dict[str, bytes]` Python signature; keep the Go meaning.
- Item 13 (~line 347): `Both engines use truncating writes` → `The scaffolder uses truncating writes`;
  delete the last sentence about Copier.

### `docs/backstage/`
- `software-template.yaml:15`: `2-idp-scaffolder/golang` → `2-idp-scaffolder`.
- `README.md:12`: `two CLI engines` → `a Go CLI`.
- `README.md:24`: `` (`2-idp-scaffolder/golang/`) `` → `` (`2-idp-scaffolder/`) ``.
- `README.md:43`: `` `2-idp-scaffolder/golang`'s `` → `` `2-idp-scaffolder`'s ``.
- `README.md:57`: `2-idp-scaffolder/golang/TODO.md` → `2-idp-scaffolder/TODO.md (does not exist yet)`.

### `1-platform-catalog/README.md:47`
- `both scaffolder engines look` → `the scaffolder looks`.

### New `2-idp-scaffolder/README.md` (≤ 30 lines)
What it is (Go + Cobra CLI, two verbs), who runs it (developers, Backstage via `idp:run-cli`, CI),
the two `go run` commands from the README above, the test command, and a link to
`CODE_WALKTHROUGH.md` and ADR 0015. Plain words, no marketing.

Commit: `docs(scaffolder): describe the single Go engine`

## Part 5 — ADR 0015

`docs/adr/0015-go-only-scaffolder.md`, same header format as ADR 0014
(`# ADR 0015: …`, `**Date:** 2026-10-04`, `**Status:** Accepted`, then Context / Decision /
Consequences). Content:

- **Context:** two engines (Go 1,594 lines, Python 789 lines) read one catalog; every template
  field change needed both; a CI parity job diffed their output. Backstage, the Makefile demo
  and CI already used Go. Python's only extra was a FastAPI server that nothing called.
- **Decision:** Go is the only engine, at `2-idp-scaffolder/`. Python deleted (recover from git
  history — last commit with it is `0688f58`). Consumers call the CLI: developers directly,
  Backstage through a custom scaffolder action that runs the binary, CI by running it.
  No REST API for now.
- **When to add a REST API (`cmd/api/`, `net/http`):** a second consumer that cannot run a
  binary (e.g. Backstage moved to a hosted backend without the CLI in its image, or a non-Backstage
  portal). `templater.Resolve` is already pure so a handler can reuse it.
- **Consequences:** lose the cross-language contract check; replaced by load-time validation,
  golden-file tests and the CI smoke run. One place to change per template field. Supersedes the
  two-engine reasoning in README's former "One Catalog, Two Engines" section.

Commit: `docs(adr): 0015 — Go is the only scaffolder engine`

---

## Rules this task can break (from `.agents/AGENTS.md` and the owner)

1. **Check exit codes and the file tree, never just stdout** — a success message can print before the failure that matters.
2. **`go vet` does not flag discarded errors** — but you are not changing Go code beyond one constant, so any diff in `.go` files other than `render_test.go:20` is a defect.
3. **Fixture names are `tenant-a` / `team-a` / `app-a`** in any new example — not `payments` / `checkout`.
4. **Never write into `3-tenant-repos/`** when testing — always pass `--output-root /tmp/...`.
5. **Executors never push, merge or publish.**
6. **Do not touch the main checkout** — it has the owner's staged, uncommitted work.

## Verify sequence (run in the worktree, after Part 5)

```bash
# 1. Python is gone, golang/ is gone
test ! -e 2-idp-scaffolder/python && test ! -e 2-idp-scaffolder/golang && echo OK

# 2. Go is green at the new location (expect: two "ok" lines, gofmt prints nothing)
cd 2-idp-scaffolder && go build ./... && go vet ./... && gofmt -l . && go test -count=1 ./... ; cd ..

# 3. The CI smoke commands work from the repo root — LOOK at the tree, not the message
go build -C 2-idp-scaffolder -o idp-cli .
rm -rf /tmp/go-output && mkdir -p /tmp/go-output
./2-idp-scaffolder/idp-cli onboard-tenant --tenant-name test-tenant --owner test-owner --catalog-root ./1-platform-catalog --output-root /tmp/go-output; echo "exit=$?"
./2-idp-scaffolder/idp-cli add-service --tenant-name test-tenant --app-name test-app --golden-path go-service-postgres --catalog-root ./1-platform-catalog --output-root /tmp/go-output; echo "exit=$?"
find /tmp/go-output -type f | sort      # paste this list into the report
rm 2-idp-scaffolder/idp-cli

# 4. Same output as before the move (regression check against main)
git worktree add --detach /tmp/wt-base main   # --detach: main is already checked out in the main checkout
(cd /tmp/wt-base/2-idp-scaffolder/golang && go run . onboard-tenant --tenant-name test-tenant --owner test-owner --catalog-root ../../1-platform-catalog --output-root /tmp/base \
  && go run . add-service --tenant-name test-tenant --app-name test-app --golden-path go-service-postgres --catalog-root ../../1-platform-catalog --output-root /tmp/base)
diff -r /tmp/base /tmp/go-output && echo "IDENTICAL"
git worktree remove --force /tmp/wt-base

# 5. Makefile targets still resolve (dry run only — must not write into 3-tenant-repos/)
make -n demo-add-service | head -3
make -n run-api 2>&1 | head -1          # expect "No rule to make target"

# 6. CI YAML parses and has no Python left
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yaml'))" && echo yaml-ok
grep -n -i "python\|uv\b\|setup-uv\|parity" .github/workflows/ci.yaml   # expect no hits

# 7. Stale-reference grep — every remaining hit must be in PLAN.md or listed as intentional in the report
grep -rnI -e "2-idp-scaffolder/golang" -e "2-idp-scaffolder/python" -e "run-api" -e "install-scaffolder" \
  -e "two engines" -e "both engines" -e "Python twin" -e "../golang" \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=.venv . | grep -v "^./PLAN.md"

# 8. Only scoped files changed
git diff --stat main...HEAD | tail -1
git diff --name-only main...HEAD | grep -v -e "^2-idp-scaffolder/" -e "^.github/workflows/ci.yaml$" \
  -e "^Makefile$" -e "^.gitignore$" -e "^README.md$" -e "^.agents/AGENTS.md$" -e "^docs/backstage/" \
  -e "^1-platform-catalog/README.md$" -e "^docs/adr/0015-go-only-scaffolder.md$"   # expect no output
git diff main...HEAD -- '*.go' | grep '^[-+][^-+]'    # expect exactly the render_test.go constant change
```

## Done means

- [ ] 5 commits on `refactor/go-only-scaffolder` in worktree `../idp-go-only`, one per Part.
- [ ] Verify steps 1–8 all give the expected result; step 4 prints `IDENTICAL`.
- [ ] No file outside Scope changed; `PLAN.md` untouched; main checkout untouched.
- [ ] Report back: the commit list (`git log --oneline main..HEAD`), the output of verify steps
      2, 3 (file list), 4, 7 and 8, any deviation from this plan and why, and **what you did not
      verify** (at minimum: the GitHub Actions run itself, and Backstage — neither can run locally).

## Out of scope

- A Go REST API (`cmd/api/`). Decided against for now — see ADR 0015 / Decision 3.
- Renaming testdata `payments/checkout` → `tenant-a/app-a`.
- Creating `2-idp-scaffolder/TODO.md`.
- The owner's Phase 21 (Backstage metadata). Its step 21.2 mentions the Python engine; the owner
  will drop that half after this lands.
- Any edit to `PLAN.md`, `3-tenant-repos/`, or `4-platform-engineering/`.
- Deleting the repo-root `.venv/` (untracked, owner's local env).
