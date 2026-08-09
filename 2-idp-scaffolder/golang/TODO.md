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

## Phase 2 — Finish the test suite (half a day)

### 2a. Complete `resolve_test.go`

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

### 2b. Golden-file test for the rendered tree

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

### 2c. Error-path test

Bad runtime → non-nil error **and zero files written**. Pins the "no half-scaffolded
service" property that bug 3 violated. Assert both halves; the file count is the half
that actually catches it.

**Verify:** `go test ./... -v`

---

## Phase 3 — Overwrite protection + `--dry-run` (the main event)

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

### 5a-bis. Output layout is monorepo-shaped — `catalog.yaml` `destinations:`

The `3-tenant-workloads` segment is gone from the binary: `OutputDir` is now exactly
`--output-root`, and the Makefile names the demo path. But Mode B — a client scaffolding
into their own checkout — is still not implemented. Running the binary in a would-be
`<org>/payments-apps` today produces:

```
./payments/apps/checkout-api/main.go            <- want ./checkout-api/main.go
./payments/gitops/apps/checkout-api/dev/...     <- belongs in <org>/payments-gitops
./payments/infra/apps/checkout-api/dev/...      <- belongs in <org>/payments-infra
```

Two distinct problems, and neither is a path-joining bug:

1. **The `{team}/` prefix is redundant** in a repo that already belongs to that team.
2. **One `add-service` writes three repo kinds.** `apps`, `infra`, and `gitops` are
   separate repos in production, so no single working directory is the right destination
   for all three. This is the real blocker, and it is an architecture question, not a
   flag: does `add-service` open three PRs (the Backstage `publish:github` model), get
   run three times with `--kind`, or keep writing a monorepo tree that
   `git subtree split` fans out?

`destinations:` encodes the monorepo answer today. AGENTS.md already concedes the
monorepo → polyrepo mapping is "documented rather than encoded"; this is where that
concession comes due. Options: a `--layout monorepo|repo-per-kind` flag reading a second
destinations table, or accepting that the local-write CLI is a demo affordance and that
the production path is an API that opens PRs.

Worth deciding before advertising Mode B as supported. The cwd default it needs is
already in place, so nothing here is blocked on plumbing.

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

### 5d. Logging — `render.go:112`

`fmt.Println(srcPath, "-->", targetPath)` mixes logging into rendering. Inject an
`io.Writer` or `*slog.Logger` on `Renderer` so tests can capture it and `cmd/api` can
route to structured logs. Do it when Phase 3's `Writer` lands — same surgery.

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
