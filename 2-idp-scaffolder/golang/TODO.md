# Implementation Plan — Scaffolder, remaining work

Phases are ordered by value-per-risk. Each states **what**, **why**, **where**
(file:line), **what to research**, and **how to verify**. Working code for every phase
is in the [Answer Key](#answer-key) at the end — attempt the phase first, then compare.

---

## Already done

| Change | Where | Result |
|---|---|---|
| Capability merge/dedup via `slices.Sort` + `slices.Compact` | `cmd/cli/add_service.go` | 20 lines → 16, 6 ifs → 4 |
| `--capabilities` as `StringSliceVar` | `add_service.go` init | comma lists *and* repeated flags |
| `MarkFlagsOneRequired("golden-path","runtime")` | `add_service.go` init | validated at parse time, shows in `--help` |
| Table-driven rendering (`blueprint` struct) | `internal/templater/render.go` | 3 duplicated blocks → 1 table + loop |
| Shared `renderDestinations` helper | `render.go` | one place where "resolve dest, then walk" lives |
| Exported-boundary guard on `RenderService` | `render.go` | empty `Runtime` can no longer render *every* runtime |
| `Resolve` extracted as a pure function | `render.go` | no globals, no filesystem; unit-testable |
| `TestResolve` table-driven test | `internal/templater/resolve_test.go` | 4 cases passing, 3 marked TODO |

**Bugs found during that pass** — every one built clean and passed `go vet` and
`gofmt`, and was caught only by running the binary:

1. Dedup rewrite dropped the merge → `--capabilities` silently ignored
2. `RenderTenantFoundation` lost its `Destinations[...]` lookup → every team written into a shared `blueprints/` directory
3. `renderDestinations` error discarded → exit 0 on failure, orphaned `postgres.tf` with no owning service
4. `_, err :=` discarded the resolved config
5. `RenderService(cfg)` instead of `RenderService(resolved)` → golden paths broken

That list is the justification for Phases 1 and 2. Nothing in the current toolchain
would have caught any of them.

---

## ~~Phase 1 — `golangci-lint`~~ ✅ DONE

`.golangci.yml` (schema v2) sits at the module root. Defaults plus `errorlint`,
`misspell`, `nilerr`, `unconvert`, `wastedassign`; `errcheck` runs with
`check-blank: true` so `x, _ := f()` is reported too.

It found two real defects on the first run, both invisible to build, vet and gofmt:

1. `render.go` — `defer outFile.Close()` discarded its error. Closing a file you just
   wrote to is precisely where a buffered write can still fail (full disk, network
   filesystem), so this could report success having lost bytes. Replaced the
   Create/WriteTo/deferred-Close trio with a single `os.WriteFile`: one call, one
   error, and one line for Phase 3's `Writer` seam to replace later.
2. `root.go` — `home, _ := os.UserHomeDir()`. On failure `home` is `""`, which makes
   the cache path a *relative* `.scaffolder-cache/<ref>`, written into whatever
   directory the user ran from and never found again.

One exclusion is configured, with its reasoning in the file: cobra's `MarkFlagRequired`
returns an error only when the named flag does not exist, which is a programmer error
caught the first time the command runs.

**Run:** `golangci-lint run ./...` — currently `0 issues`.

> Note the config schema: golangci-lint v2 moved `exclusions` under `linters:` and
> requires a top-level `version: "2"`. Check with `golangci-lint config verify`.

---

## ~~Phase 2 — Finish the test suite~~ ✅ DONE

### ~~2a. Complete `resolve_test.go`~~ ✅ DONE

Three TODOs are already in the table; each is a five-line struct literal.

- duplicates collapse (`{"postgres","iam","postgres"}` → `{"iam","postgres"}`) — the case bug 1 broke
- no runtime and no golden path → error (`cmd/api` will not go through cobra)
- capability ordering is deterministic

Then implement `TestResolveDoesNotMutateInput`, currently `t.Skip`ped. Build a `Config`
whose `Capabilities` has spare capacity (`make([]string, 1, 4)`), call `Resolve`, assert
the caller's slice is unchanged. Delete the `slices.Clone` line in `Resolve` and watch
it fail — that failure is the lesson.

**Research:** Go slice internals (pointer/len/cap); why struct assignment copies a slice
*header* but shares the backing array.

### ~~2b. Golden-file test for the rendered tree~~ ✅ DONE

Automates the `git worktree` + `diff -r` loop used to verify the refactor by hand.

**Shape:** build a `Renderer` with `CatalogFS: os.DirFS(<catalog>)` and
`OutputDir: t.TempDir()`, render, walk the result into `map[relPath]contents`, compare
against `testdata/`.

**Idioms to learn:**
- `t.TempDir()` — auto-cleaned per test, no teardown
- `testdata/` — a directory name the Go toolchain ignores by convention
- an `-update` flag (`var update = flag.Bool("update", false, "...")`) so
  `go test ./... -update` regenerates goldens after an intentional change

Cases worth pinning: golden-path-only service (catches bug 5), full `onboard-team` tree
(catches bug 2), a service with two capabilities.

**Research:** "golang golden file testing"; `testing.T.TempDir`; `fs.WalkDir` vs
`filepath.WalkDir` — the code uses both, know why.

### ~~2c. Error-path test~~ ✅ DONE

Bad runtime → non-nil error **and zero files written**. Pins the "no half-scaffolded
service" property that bug 3 violated. Assert both halves; the file count is the half
that actually catches it.

**Verify:** `go test ./... -v`

---

## ~~Phase 3 — Overwrite protection + `--dry-run` (the main event)~~ ✅ DONE

Both touch `processSingleTemplate`, so do them together, overwrite first.

### The problem

`render.go:102` uses `os.Create`, which **truncates**. Re-running `add-service` silently
destroys a team's edited `main.go`. That is data loss, and the most likely source of an
angry message in week one of rollout.

Separately, `--dry-run` is declared at `root.go:19,103` and **never read** — a user
passing it gets a full, silent write.

### The design

One seam fixes both:

```go
type Writer interface {
	WriteFile(path string, content []byte) error
	MkdirAll(path string) error
}
```

- `OSWriter` — real filesystem
- `DryRunWriter` — prints `[WOULD WRITE] <path>`, touches nothing
- `MapWriter` — in-memory; later makes 2b tests filesystem-free

`Renderer` gains a `Writer` field. Three call sites change: `os.Create` + `buf.WriteTo`
in `processSingleTemplate` (`render.go:102-110`), `os.MkdirAll` in `walkAndRender`
(`render.go:67`), and `os.MkdirAll` for the capabilities directory in `RenderService`.

**Skip-if-exists policy sits *above* the writer**, in `processSingleTemplate`, so both
modes report it identically: `os.Stat` the target and, unless `--force`, log `[SKIP]`
and return nil.

### The trap

Deciding *where* the skip check lives is the exercise. Inside the writers, dry-run
cannot report skips. Inside the loop, it duplicates. Above the writer — in the one
function that already knows the target path — is correct.

### Beyond skip-if-exists

Skip-if-exists buys about a year. The durable answer is a **Copier-style answers file**:
record the resolved `Config` into `.scaffolder-answers.yaml` inside the generated tree
so a later `scaffolder update` can re-render and three-way merge against the team's
edits. Read how Copier and Backstage Software Templates handle this before designing it.

**Research:** `os.OpenFile` with `O_EXCL` (atomic create-if-not-exists) vs
stat-then-write, and the TOCTOU race between them; "accept interfaces, return structs";
Copier's answers-file and update model.

**Verify:**

```bash
CAT=../../1-platform-catalog

# dry run writes nothing
go run . add-service --dry-run --catalog-root $CAT --output-root /tmp/dry \
    -t payments -a checkout --golden-path go-service-postgres
find /tmp/dry -type f | wc -l          # want 0

# second run must not clobber a hand edit
go run . add-service --catalog-root $CAT --output-root /tmp/real \
    -t payments -a checkout --golden-path go-service-postgres
echo "// hand edit" >> /tmp/real/3-tenant-workloads/payments/apps/checkout/main.go
go run . add-service --catalog-root $CAT --output-root /tmp/real \
    -t payments -a checkout --golden-path go-service-postgres   # expect [SKIP]
grep -c "hand edit" /tmp/real/3-tenant-workloads/payments/apps/checkout/main.go  # want 1
```

---

## Phase 4 — Pin the catalog version

**Where:** `root.go:58`, `root.go:112`.

**Problem:** the ref is hardcoded to `"feature/go-cli"`. Every engineer's output tracks
a live branch — one bad catalog commit breaks everyone at once with no rollback. Worse,
the cache directory is keyed on the ref *name*, so a moving branch yields a stale cache
that never refreshes.

**Do:**
- add `--catalog-ref`, defaulting to a package-level `var catalogRef = "main"`
- stamp at build time: `go build -ldflags "-X 'scaffolder/cmd/cli.catalogRef=v1.2.0'"`
- add `--catalog-refresh`, or key the cache by resolved commit SHA rather than ref name
- print the resolved ref every run, so output is traceable to a catalog version

**Research:** `go build -ldflags -X` (package-level `string` vars only; needs the full
import path); `go-getter` ref/subdirectory syntax; `runtime/debug.ReadBuildInfo` as an
alternative build stamp.

**Also fix here:** `PersistentPreRunE` runs *before* cobra's flag-group validation, so
`add-service` with no flags downloads the whole catalog from GitHub before reporting the
flag error. Make catalog loading lazy, or validate first.

**Verify:** a `--version`-style output naming the catalog ref; `--catalog-ref <tag>`
produces that tag's templates.

---

## Phase 5 — Smaller items

### 5a. ~~`outputRoot` discovery~~ — DONE (and the original instruction was wrong)

`outputRoot` now defaults to the current working directory, and the demo flow moved to
`make demo-add-service` / `make demo-onboard-team`, which pass
`--output-root "$(git rev-parse --show-toplevel)"`.

This item used to say *"replace with a walk up the tree looking for `.git`, erroring
cleanly if absent."* That was the wrong fix, for two reasons worth keeping written down:

- **`.git` is the wrong marker.** It is a *file*, not a directory, in a `git worktree` or
  a submodule; it is absent entirely from a release tarball or a Docker layer; and it
  answers "am I in a git repo?" when the question was "where is the platform tree?".
- **The search itself is the wrong shape.** Auto-discovery only makes sense in this
  reference repo. A client runs the binary inside *their own* checkout, where neither
  `.git`-relative nor `1-platform-catalog`-relative discovery gives the right answer. The
  cwd default is the same contract as `terraform`, `npm`, and `kubectl apply -f .`, and
  needs no explanation.

The general lesson: a heuristic that guesses the caller's layout belongs in a dev wrapper,
not compiled into a binary other people run. `git rev-parse --show-toplevel` already does
the walk, correctly, and is one line of Make.

### 5a-bis. ~~Output layout is monorepo-shaped~~ — NOT A DEFECT. Do not "fix" this.

This item previously claimed the `{team}/apps/` prefix was wrong and proposed a
`--layout monorepo|repo-per-kind` flag. Both were mistaken, and the mistake is worth
recording so nobody re-opens it.

`3-tenant-workloads/` is the **authoring** format. The production format is three repos
per team, and the transformation already exists — it is `git subtree split`, whose
prefixes this layout was designed around. Verified, not assumed:

```console
$ git subtree split --prefix=3-tenant-workloads/team-a/apps
1f8a94e38604a7ff63e3e2d909fab5ecca8279af

$ git ls-tree -r --name-only 1f8a94e3
CODEOWNERS
app-a/catalog-info.yaml
app-a/go.mod
app-a/main.go
```

The split strips the prefix. `team-a/apps/app-a/main.go` becomes `app-a/main.go`, and
`CODEOWNERS` lands at the split repo's root — the only place GitHub honours it, which is
the reason `onboard-team` writes it there. So `{team}/` and `apps/` exist in the view that
needs them and disappear from the view where they would be redundant, with no code
involved. A `--layout` flag would add a second code path to keep in sync and a mode for a
user to get wrong, to reach a result git already produces.

**What is genuinely not implemented:** a client running `add-service` *inside* an
already-split repo such as `<org>/team-a-apps`. One invocation emits three repo kinds, so
no single working directory is correct for all three. That is not a path-joining bug and
no `filepath.Join` fixes it.

**Decision: not planned.** That is a different product from this one. The authoring
surface here is the platform monorepo, and CI/subtree fans out — a coherent model that
plenty of platform teams run. If it is ever wanted, the answer is the Backstage
`publish:github` one — the CLI opens a PR per repo and the developer never `cd`s anywhere
— not a layout flag. Reopen only with that framing.

### 5b. ~~`GoldenPath.Delivery` is dead~~ — DONE

Deleted rather than wired up. There is exactly one delivery mechanism, so
`delivery: standard-helm` was config that lied. Add a `delivery:` registry only when a
second mechanism actually exists — that is when it earns its keep.

The open question about the chart is also settled: it is a shared platform-owned chart,
**never** part of the scaffolder's output contract. It moved to
`1-platform-catalog/charts/service/`, which keeps one rule true — everything under
`building-blocks/` has a `destinations` key and lands in a tenant repo; nothing else
does. Publishing it to an OCI registry and referencing it by version is the natural
next step, but storing it in-repo is fine until a second chart exists.

### 5c. Globals — `root.go:16-25`

`cfg` and `renderer` are package-level and shared by *both* commands' flag bindings.
Fine for one-shot CLI, impossible for `cmd/api/` serving concurrent requests. Build a
per-invocation `Options` struct in `RunE` and keep cobra as a thin adapter over the same
engine the HTTP handler calls. Much of this fell out of the `Resolve` extraction already.

→ [Phase 7b](#7b-contextcontext-through-every-io-path) forces the issue: once `RunE`
reads `cmd.Context()`, per-invocation state has somewhere to live.

### 5d. Logging — `render.go:112`

`fmt.Println(srcPath, "-->", targetPath)` mixes logging into rendering. Inject an
`io.Writer` or `*slog.Logger` on `Renderer` so tests can capture it and `cmd/api` can
route to structured logs. Do it when Phase 3's `Writer` lands — same surgery.

→ Expanded into [Phase 7d](#7d-logslog-instead-of-fmtprintln), which inventories every
print in the codebase and adds the stdout/stderr split.

---

## Verification (whole suite)

```bash
CAT=../../1-platform-catalog

go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./...
go test ./... -v

# happy paths
go run . onboard-team --catalog-root $CAT --output-root /tmp/o -t payments
go run . add-service  --catalog-root $CAT --output-root /tmp/o -t payments -a checkout \
    --golden-path go-service-postgres --capabilities postgres,postgres,iam

# expect, among others:
#   payments/{apps,infra,gitops}/CODEOWNERS
#   payments/gitops/platform/applicationsets/payments.yaml
#   payments/apps/checkout/{go.mod,main.go,catalog-info.yaml}
#   payments/infra/apps/checkout/dev/{postgres.tf,iam.tf}
#   payments/gitops/apps/checkout/dev/values.yaml

# error paths — all must exit 1 and write nothing
go run . add-service ... --golden-path nope
go run . add-service ... --runtime doesnotexist
go run . add-service ... --capabilities bogus     # "unknown capability: bogus"
go run . add-service ...                          # flag-group error
```

**Regression check against any commit** (the manual form of 2b):

```bash
git worktree add /tmp/wt <ref>
(cd /tmp/wt/2-idp-scaffolder/golang && go run . add-service ... --output-root /tmp/base)
go run . add-service ... --output-root /tmp/new
diff -r /tmp/base /tmp/new
git worktree remove --force /tmp/wt
```

Check **exit codes and the file tree**, never just grep stdout — bug 5 hid behind a
success message printed immediately before the failure.

---

## Out of scope (deliberately)

- `cmd/api/` implementation — Phases 3 and 5c are its prerequisites
- Backstage plugin / UI
- Multi-env promotion beyond the `{env}` substitution already in `destinations`
- Real `git subtree split` into per-team repos (the destination paths are already the split prefixes)

---

# Answer Key

Read after attempting the phase.

## Phase 3 — `Writer` seam

```go
// internal/templater/writer.go
package templater

import (
	"fmt"
	"os"
	"path/filepath"
)

// Writer is the only path through which the renderer touches the filesystem.
// One seam serves three needs: real writes, --dry-run, and in-memory tests.
type Writer interface {
	WriteFile(path string, content []byte) error
	MkdirAll(path string) error
}

type OSWriter struct{}

func (OSWriter) MkdirAll(path string) error { return os.MkdirAll(path, 0755) }

func (OSWriter) WriteFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	// O_EXCL is deliberately not used: the skip-if-exists decision lives one level
	// up, so that dry-run can report skips too. See processSingleTemplate.
	return os.WriteFile(path, content, 0644)
}

// DryRunWriter reports what would happen and touches nothing.
type DryRunWriter struct{}

func (DryRunWriter) MkdirAll(path string) error { return nil }

func (DryRunWriter) WriteFile(path string, content []byte) error {
	fmt.Printf("[WOULD WRITE] %s (%d bytes)\n", path, len(content))
	return nil
}
```

`Renderer` gains `Writer Writer` and `Force bool`:

```go
func (r *Renderer) processSingleTemplate(srcPath, targetPath string, data any) error {
	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	// Skip-if-exists sits above the Writer so dry-run reports skips identically.
	if !r.Force {
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("[SKIP] %s (exists; use --force to overwrite)\n", targetPath)
			return nil
		}
	}

	rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", srcPath, err)
	}

	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").Parse(string(rawBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", srcPath, err)
	}

	// Render to a buffer first so a template error never leaves a 0-byte file.
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render template %s: %w", srcPath, err)
	}

	if err := r.Writer.WriteFile(targetPath, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	fmt.Println(srcPath, "-->", targetPath)
	return nil
}
```

Wire it in `root.go`'s `PersistentPreRunE`:

```go
var w templater.Writer = templater.OSWriter{}
if dryRun {
	w = templater.DryRunWriter{}
}
renderer = &templater.Renderer{
	CatalogFS: os.DirFS(catalogRoot),
	Spec:      spec,
	OutputDir: filepath.Join(outputRoot, "3-tenant-workloads"),
	Writer:    w,
	Force:     force,
}
```

**Why `os.WriteFile` over `os.Create`:** `os.Create` is
`OpenFile(O_RDWR|O_CREATE|O_TRUNC)` — the `O_TRUNC` is the data loss. `os.WriteFile` is
not inherently safer, but routing every write through one function puts the truncation
decision in exactly one place you can reason about.

## Phase 4 — version pinning

```go
// cmd/cli/root.go

// catalogRef is overridden at build time:
//   go build -ldflags "-X 'scaffolder/cmd/cli.catalogRef=v1.2.0'"
// Defaults to main so `go run .` from a clean checkout still works.
var catalogRef = "main"

func init() {
	rootCmd.PersistentFlags().StringVar(&catalogRef, "catalog-ref",
		catalogRef, "catalog git ref (tag, branch, or SHA)")
}

func fetchRemoteCatalog(ref string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(home, ".scaffolder-cache", ref)

	// A moving ref (a branch) must not be served from a stale cache.
	if catalogRefresh {
		if err := os.RemoveAll(cacheDir); err != nil {
			return "", err
		}
	}

	url := "git::https://github.com/ok-karthik/internal-developer-platform.git" +
		"//1-platform-catalog?ref=" + ref
	if err := getter.Get(cacheDir, url); err != nil {
		return "", fmt.Errorf("fetching catalog %s: %w", ref, err)
	}
	fmt.Printf("📦 catalog %s\n", ref)
	return cacheDir, nil
}
```

## Phase 5a — git-root discovery

```go
// findRepoRoot walks up from dir looking for .git. filepath.Dir returns its own
// input at the filesystem root, which is the loop's termination condition.
func findRepoRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git found above %s; pass --output-root", dir)
		}
		dir = parent
	}
}
```

## Phase 2b — golden-file test skeleton

```go
package templater

import (
	"flag"
	"os"
	"path/filepath"
	"scaffolder/internal/catalog"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata golden files")

const catalogDir = "../../../../1-platform-catalog"

func TestRenderServiceGolden(t *testing.T) {
	spec, err := catalog.LoadCatalog(filepath.Join(catalogDir, "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: out,
		Writer:    OSWriter{},
		Force:     true, // a fresh TempDir has nothing to skip
	}

	cfg, err := Resolve(spec, "go-service-postgres", Config{
		TeamName: "payments", AppName: "checkout", Env: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderService(cfg); err != nil {
		t.Fatal(err)
	}

	compareGolden(t, out, filepath.Join("testdata", "render_service"))
}
```

`compareGolden` is the part worth writing yourself: walk both trees, diff the sorted
path lists *first* (a missing or extra file is the more common failure), then diff
contents per file. With `-update`, write the tree to `testdata` instead of comparing.
Report the first mismatch with both values — a golden test that says only "trees differ"
wastes the time it was meant to save.

---

## Phase 6 — Refresh `CODE_WALKTHROUGH.md`

The walkthrough is a teaching document that quotes source verbatim, so it rots the
moment a signature changes. Two commits — `bc03765` (declared runtimes) and `214864c`
(chart move) — left it stale. Fix it in one pass rather than drip-feeding, and re-read
§4 end to end afterwards: a walkthrough that is 90% right is worse than none, because a
reader trusts it instead of the code.

**Stale spots, in file order:**

1. **`~264` — the caller.** Shows `catalog.LoadCatalog(filepath.Join(catalogPath, "catalog.yaml"))`.
   It is now `LoadCatalog(catalogFS)`. Worth explaining *why*: `root.go` derives one
   `os.DirFS(catalogRoot)` and hands the same `fs.FS` to both the loader and the
   renderer, so validation checks declarations against the very tree that will be
   walked. That is a design point, not a mechanical rename.

2. **`~294` — the §4 code block.** Still shows `LoadCatalog(filePath string)`,
   `os.ReadFile(filePath)`, `catalog.validate()`, and the error wrap
   `"invalid catalog %s"`. All four changed: `fs.FS`, `fs.ReadFile(catalogFS, "catalog.yaml")`,
   `validate(catalogFS)`, `"invalid catalog.yaml: %w"`. The line
   **"In: a file path"** immediately below needs to become "In: the catalog filesystem".

3. **`~330` — the YAML→struct mapping block.** Two errors: it still lists
   `delivery: standard-helm → GoldenPaths[0].Delivery` (the field was deleted as dead
   config), and it omits `runtimes:` entirely. Pins also read `v1.0.2`; the catalog is
   on `v1.1.0`.

4. **`~340` — "why is golden-paths a list but capabilities a map?"** This is the best
   paragraph in §4 and it now has a *third* data point: `runtimes` is a map for exactly
   the capabilities reason (looked up by name, `c.Runtimes[gp.Runtime]`), never
   iterated for display. Extend it rather than leaving the reader to infer.

5. **`~842` — the call-flow diagram.** Still shows `LoadCatalog(.../catalog.yaml)`.

6. **Missing entirely: the `fs.FS` testability payoff.** The walkthrough already claims
   `Renderer.CatalogFS` is "the testability seam". `LoadCatalog` now sits on that same
   seam, and `internal/catalog/catalog_test.go` is the proof — a table-driven test
   building a whole catalog out of `fstest.MapFS`, no disk, no fixtures directory. Show
   the `fstest.MapFS` literal; it is the most convincing argument in the document for
   why the signature takes an interface rather than a path.

7. **Missing: the runtime checks.** Three of them, and the split matters. `validate()`
   rejects a golden path naming an undeclared runtime, and rejects a declared runtime
   with no directory; `Resolve` (`render.go`) applies the same check to an explicit
   `--runtime`, because a flag override never passes through golden-path validation.
   Note the deliberate *absence* too: an undeclared directory is invisible, not an
   error. See AGENTS.md decision 13 — it is the kind of thing a reader will otherwise
   file as an oversight.

8. **Sweep for `building-blocks/` inventories.** The chart moved to
   `1-platform-catalog/charts/service/`. A grep for the old path comes back clean, but
   any prose listing what lives under `building-blocks/` should now mention that
   `charts/` is a sibling and is rendered by CI, never scaffolded.

**Guard against the next drift:** most of these are quoted code that no test covers.
Once Phase 2b lands, the golden-file `testdata` becomes the honest illustration for §4
and the walkthrough can reference it instead of pasting a second copy of the truth.

---

## Phase 7 — Make it idiomatic Go

Four separate habits, listed together because they share one motivation: `cmd/api/`.
Everything below is invisible in a one-shot CLI and load-bearing the moment the same
engine is called from an HTTP handler. Do them in the order given — 7a changes
signatures that 7b and 7d then thread through, so the reverse order means touching
every function twice.

Overlaps by design: 7d is the full version of [5c](#5c-globals--rootgo16-25) and
[5d](#5d-logging--rendergo112), and 7c is [Phase 2](#phase-2--finish-the-test-suite-half-a-day)
looked at as a convention rather than as coverage. Land those first where they collide.

### 7a. Wrapped errors — `%w`, `errors.Is`, `errors.As`

**Where we already do it right:** `render.go:60,88,93,99,109,217,228` and
`root.go:135,142` all wrap with `%w` and name the offending path. That is the standard
the rest should meet.

**The gaps, in file order:**

| Where | What | Why it matters |
|---|---|---|
| `catalog.go:61` | `return nil, err` from `fs.ReadFile` | error says `open catalog.yaml: no such file` and never says *which catalog root* |
| `catalog.go:67` | `return nil, err` from `yaml.Unmarshal` | a YAML syntax error arrives unattributed |
| `render.go:68` | `os.MkdirAll(targetPath, 0755)` | bare `mkdir ...: permission denied`, no template context |
| `render.go:120,125` | `renderPath` returns raw parse/execute errors | the caller at `render.go:60` wraps it, so this one is arguably fine — decide deliberately, don't leave it accidental |
| `render.go:190` | `os.MkdirAll(infraTargetDir, 0755)` | same as above |

**The larger point — nothing here is inspectable.** Every failure in this codebase is a
`fmt.Errorf` string. `errors.Is` and `errors.As` have nothing to match on, so a caller
cannot tell "user asked for a capability that doesn't exist" (400) from "disk is full"
(500). That distinction is exactly what an HTTP handler needs and what a CLI wants for
exit codes.

Introduce two things in `internal/catalog` / `internal/templater`:

```go
// Sentinels for the yes/no cases.
var (
	ErrUnknownCapability = errors.New("unknown capability")
	ErrUnknownRuntime    = errors.New("unknown runtime")
	ErrUnknownGoldenPath = errors.New("golden path not found")
	ErrRuntimeRequired   = errors.New("a runtime is required")
)

// A type where the caller wants the offending value back, not just the class.
type ValidationError struct {
	Field string // "capability", "runtime", "golden-path"
	Value string
	Err   error  // one of the sentinels above
}

func (e *ValidationError) Error() string { ... }
func (e *ValidationError) Unwrap() error { return e.Err }
```

Then `Resolve` (`render.go:246,256,264`) and `RenderService` (`render.go:174,198`)
return those instead of naked `fmt.Errorf`, keeping the *message* identical — the
existing text is good, only the type is missing. Call sites that today do nothing
gain the ability to branch:

```go
var ve *templater.ValidationError
switch {
case errors.As(err, &ve):
	// CLI: exit 2 and print the offered values. API: 400 with ve.Field/ve.Value.
default:
	// exit 1 / 500
}
```

**The trap:** wrapping is not free of consequence. `%w` makes the wrapped error part of
your **public API** — once a caller writes `errors.Is(err, ErrUnknownCapability)`, you
cannot stop returning it without breaking them. Use `%v` where you deliberately want the
detail in the message but not in the chain. Knowing which you meant is the whole
exercise; `errorlint` (already enabled in `.golangci.yml`) only catches the mechanical
half.

**Research:** the Go 1.13 error-values proposal; `errors.Join` (Go 1.20) for the
capability loop, which today aborts on the first bad name — collecting all of them is
strictly better UX for a scaffolder invoked with `--capabilities a,b,c`; why
`errors.As` takes a `**ValidationError`.

**Verify:** a test that asserts `errors.Is(err, ErrUnknownCapability)` on a bad
`--capabilities`, not `strings.Contains(err.Error(), "unknown capability")`. The second
form is why error strings become frozen APIs by accident.

### 7b. `context.Context` through every I/O path

**Current state: zero occurrences of `context` in the module.** Verified —
`grep -rn context --include='*.go' .` returns nothing.

Three real I/O paths, in order of how much cancellation actually buys:

1. **`fetchRemoteCatalog` (`root.go:127-146`)** — the one that matters. `getter.Get` is
   a network clone with no timeout and no cancellation: Ctrl-C during the spinner leaves
   a half-populated `~/.scaffolder-cache/<ref>` that every later run happily reads. Swap
   the package-level helper for the client form, which has a `Ctx` field:

   ```go
   client := &getter.Client{
       Ctx:  ctx,
       Src:  url,
       Dst:  cacheDir,
       Mode: getter.ClientModeAny,
   }
   if err := client.Get(); err != nil { ... }
   ```

   A partially-written cache directory is a bug on its own — clone to a temp dir and
   rename into place, or delete on failure.

2. **Cobra wiring** — `Execute()` (`root.go:109`) calls `rootCmd.Execute()`, which
   supplies `context.Background()`. Use `signal.NotifyContext(context.Background(),
   os.Interrupt, syscall.SIGTERM)` and `rootCmd.ExecuteContext(ctx)`; every `RunE` and
   `PersistentPreRunE` then reads `cmd.Context()`. This is the whole reason cobra has
   `ExecuteContext` — it is not extra plumbing, it is the plumbing that already exists
   being left unused.

3. **The render walk** — `walkAndRender`, `processSingleTemplate`, `RenderService`,
   `RenderTenantFoundation`, `renderDestinations` take `ctx context.Context` as the
   first parameter, and `handleFile` (`render.go:47`) starts with:

   ```go
   if err := ctx.Err(); err != nil {
       return err
   }
   ```

   `fs.WalkDir` has no cancellation of its own; returning an error from the callback is
   how you stop it. Local disk writes are fast, so this is mostly about the API case,
   where a client disconnecting should not leave the server rendering a tree nobody will
   read.

**Rules to follow, since this is the part people get wrong:** `ctx` is always the first
parameter and always named `ctx`; never store it in a struct — that means **not** a
`Renderer.Ctx` field, even though it looks tidier than threading it; `context.TODO()` is
for a migration in progress, not a resting state.

**Deliberately out of scope:** `Resolve` (`render.go:236`) is pure — no I/O, no
blocking. Giving it a `ctx` would be cargo cult. The doc comment already says it "reads
nothing outside its parameters"; keep that true.

**Research:** `context.Context` docs (the "do not store in a struct" rule and its
reasoning); `signal.NotifyContext`; cobra `ExecuteContext`; `errors.Is(err,
context.Canceled)` vs `context.DeadlineExceeded` — the CLI should exit quietly on the
first and loudly on the second.

**Verify:**

```bash
# Ctrl-C during the fetch must leave no cache directory behind
rm -rf ~/.scaffolder-cache
go run . add-service -t payments -a checkout --golden-path go-service-postgres &
sleep 1; kill -INT %1
ls ~/.scaffolder-cache            # want: absent, or complete — never partial
```

Plus a unit test that cancels a context before calling `RenderService` and asserts
`errors.Is(err, context.Canceled)` **and zero files written** — the same two-part
assertion as [2c](#2c-error-path-test).

### 7c. Table-driven tests as the default shape

`resolve_test.go` and `catalog_test.go` are already table-driven and are the model:
a `[]struct` with a `name` field, `t.Run(tc.name, ...)`, and — the part most people
skip — cases that assert *failure* as carefully as success.

What is missing is not style but coverage; that is [Phase 2](#phase-2--finish-the-test-suite-half-a-day),
and there is no point restating it here. What belongs in *this* phase is making the
convention explicit so the next test written follows it:

- one table per behaviour, not one table per function
- `name` reads as a sentence describing the case, because it is what `-run` matches
  and what a failure prints
- for error cases assert the sentinel from 7a with `errors.Is`, never the message text
- no shared mutable state between cases; anything expensive goes in the table, not in
  package-level `var`s
- `t.Parallel()` in both the outer function and inside `t.Run` once cases are
  independent — and they are only independent once `t.TempDir()` and the `Writer` seam
  from Phase 3 have removed the shared filesystem

Write this down in `.agents/AGENTS.md` alongside the other decisions rather than leaving
it as folklore in two files that happen to do it right.

**Research:** `t.Parallel()` and the loop-variable capture that used to break it (fixed
by the Go 1.22 per-iteration semantics — know that the old `tc := tc` line is now
unnecessary, and why you will still see it everywhere); `t.Cleanup` vs `defer`;
`testing.T.Setenv` and why it forbids `t.Parallel()`.

### 7d. `log/slog` instead of `fmt.Println`

**Every print in the codebase, in file order:**

| Where | What it prints | Should be |
|---|---|---|
| `render.go:112` | `srcPath --> targetPath` | `logger.Debug` — per-file progress is noise at default level |
| `render.go:213` | `Adding infrastructure capability: %s` | `logger.Info` with `slog.String("capability", name)` |
| `root.go:83` | `✅ Templates fetched!` | `logger.Info` with the resolved ref (Phase 4) as an attribute |
| `root.go:85` | `📁 Using local catalog from: %s` | `logger.Info` with `slog.String("catalog_root", ...)` |

Two structural problems behind those four lines.

**First, it all goes to stdout.** Progress chatter and machine-readable output share one
stream, so `scaffolder ... > files.txt` captures the emoji too. Human-facing logs belong
on **stderr**; stdout is reserved for output a caller might parse. The spinner
(`root.go:66`) has the same issue and needs disabling when stderr is not a TTY.

**Second, `render.go` prints at all.** A library that writes to a global stream cannot
be tested for its output and cannot be embedded in a server. Give `Renderer` a
`Logger *slog.Logger` field, defaulting to `slog.Default()` if nil so existing
construction sites keep compiling. This is the same surgery as Phase 3's `Writer` — do
both in one pass.

**The wiring**, in `root.go`:

```go
var handler slog.Handler
switch logFormat {
case "json": // what cmd/api will want
	handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
default:     // human default
	handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
}
logger := slog.New(handler)
```

Add `--log-level` (default `info`) and `--log-format` (default `text`) as persistent
flags. Then `--dry-run`'s `[WOULD WRITE]` and Phase 3's `[SKIP]` become log records with
attributes rather than `fmt.Printf` — which is what makes them assertable in a test
via a handler writing to a `bytes.Buffer`.

**The trap:** `slog` is structured logging, so `logger.Info("rendered " + path)` throws
away the entire point. The message is a constant; everything variable is an attribute.
That is what lets `cmd/api` filter by team or capability later.

**Also worth doing here:** `logger.With(slog.String("team", cfg.TeamName),
slog.String("app", cfg.AppName))` once in `RenderService`, so every record beneath it
carries the identifiers without repeating them at each call.

**Research:** `log/slog` package docs; `slog.HandlerOptions` and dynamic level via
`slog.LevelVar`; `slog.SetDefault` and why a *library* should never call it; the
`context`-aware `logger.InfoContext` variants, which pair with 7b once tracing is on the
table.

**Verify:**

```bash
# stdout stays clean; logs go to stderr
go run . add-service --catalog-root $CAT --output-root /tmp/o \
    -t payments -a checkout --golden-path go-service-postgres 2>/dev/null
# want: no emoji, no --> lines

go run . add-service ... --log-format json 2>&1 >/dev/null | head -1
# want: {"time":"...","level":"INFO","msg":"...","team":"payments",...}

go run . add-service ... --log-level debug 2>&1 | grep -c '"msg":"rendered file"'
```

**Done when:** `grep -rn 'fmt.Print' --include='*.go' internal/` returns nothing.
`cmd/cli/` may keep `fmt.Print` for genuine user-facing output — the distinction between
"output" and "logs" is the thing being learned.
