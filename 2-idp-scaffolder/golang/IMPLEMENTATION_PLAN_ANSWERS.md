# Answer Key — Scaffolder Refactor

Companion to `IMPLEMENTATION_PLAN_CLAUDE.md`. **Try each phase yourself first.** Come here when you're
stuck for more than ~20 minutes, or afterwards to compare approaches.

The code below is complete, not pseudocode — but it was written by hand and has **not** been through a
compiler, so expect the odd missing import or typo. Treat it as a reference implementation to read and
adapt, not to paste. Where there's a Go idiom worth knowing, it's called out in a
> **Why** block — those are the parts worth reading even if your own code already works.

---

## Phase 0 — The corrupted files

### `1-platform-catalog/catalog.yaml` line 6

```yaml
capabilities_source_base: "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules"
```

### `building-blocks/capabilities/{postgres,s3,iam}.tf.tmpl`

All three become byte-identical, because `.Name` now comes from the view struct:

```terraform
locals {
  resource_name_prefix = "[[ .TeamName ]]-[[ .AppName ]]"
  tags = {
    Team      = "[[ .TeamName ]]"
    Service   = "[[ .AppName ]]"
    ManagedBy = "terraform"
  }
}

module "[[ .Name ]]" {
  source    = "[[ .SourceBase ]]/[[ .Module ]]?ref=[[ .Version ]]"
  team_name = "[[ .TeamName ]]"
  app_name  = "[[ .AppName ]]"
}
```

Keep them as three separate files even though they're identical today — the moment `postgres` needs a
parameter group or `s3` needs a lifecycle rule, they diverge. Identical-for-now is fine; one file with
`if eq .Name "postgres"` branches inside it is not.

### `blueprints/system/gitops/applicationset.yaml.tmpl` — the name line only

```yaml
      name: '[[ .TeamName ]]-{{path[4]}}-{{path.basename}}'
```

---

## Phase 0.5 (optional but recommended) — normalize the destinations keys

Right now some keys are real catalog paths (`blueprints/team/gitops`) and some are abbreviations
(`runtimes`, `capabilities`). Make every key the literal source path and the table becomes a pure
source → destination map, so your Go can use one string for both:

```yaml
destinations:
  blueprints/team/gitops:            "{team}-gitops/platform/team/"
  blueprints/team/infra:             "{team}-infra/platform/"
  blueprints/system/gitops:          "{team}-gitops/platform/systems/{system}/"
  building-blocks/runtimes:          "{team}-services/{system}/{app}/"
  building-blocks/service-meta:      "{team}-services/{system}/{app}/"
  building-blocks/capabilities:      "{team}-infra/{system}/{app}/{env}/"
  building-blocks/delivery/release:  "{team}-gitops/systems/{system}/{app}/{env}/"
```

Two keys pointing at the same destination is fine — the `plan.add` collision guard below catches actual
file conflicts. All code that follows assumes these key names.

Also `git mv 1-platform-catalog/building-blocks/runtimes/catalog-info.yaml.tmpl
1-platform-catalog/building-blocks/service-meta/catalog-info.yaml.tmpl` (plan §2.5).

---

## `internal/catalog/catalog.go`

```go
package catalog

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Capability binds a friendly name to a version-pinned Terraform module.
type Capability struct {
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
}

// GoldenPath is a paved-road service definition: a runtime plus a set of capabilities.
type GoldenPath struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Runtime      string   `yaml:"runtime"`
	Capabilities []string `yaml:"capabilities"`
	Delivery     string   `yaml:"delivery"`
}

// Catalog is the platform's offering (golden paths, capabilities) plus its
// output contract (destinations). It is the single source of truth the CLI reads.
type Catalog struct {
	CapabilitiesSourceBase string                `yaml:"capabilities_source_base"`
	GoldenPaths            []GoldenPath          `yaml:"golden-paths"`
	Capabilities           map[string]Capability `yaml:"capabilities"`
	Destinations           map[string]string     `yaml:"destinations"`
}

// requiredDestinations are the keys the renderer will look up. Missing any of them
// is a catalog authoring error, so we fail at load rather than mid-render.
var requiredDestinations = []string{
	"blueprints/team/gitops",
	"blueprints/team/infra",
	"blueprints/system/gitops",
	"building-blocks/runtimes",
	"building-blocks/service-meta",
	"building-blocks/capabilities",
	"building-blocks/delivery/release",
}

// Load reads catalog.yaml and validates it before returning.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse catalog %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid catalog %s: %w", path, err)
	}
	return &c, nil
}

// validate catches authoring mistakes at the boundary, where the error message
// can still name the offending key.
func (c *Catalog) validate() error {
	if c.CapabilitiesSourceBase == "" {
		return fmt.Errorf("capabilities_source_base is empty")
	}

	// Guards against exactly the bug that shipped in 6875cf9: a shell-escaped
	// YAML scalar that parses as \"git::...\" including the literal backslashes.
	if strings.ContainsAny(c.CapabilitiesSourceBase, `\"`) {
		return fmt.Errorf("capabilities_source_base contains a stray quote or backslash: %q",
			c.CapabilitiesSourceBase)
	}

	for _, key := range requiredDestinations {
		if c.Destinations[key] == "" {
			return fmt.Errorf("destinations is missing required key %q", key)
		}
	}

	// Every capability a golden path claims must actually exist.
	for _, gp := range c.GoldenPaths {
		if gp.Name == "" {
			return fmt.Errorf("a golden path is missing its name")
		}
		if gp.Runtime == "" {
			return fmt.Errorf("golden path %q has no runtime", gp.Name)
		}
		for _, name := range gp.Capabilities {
			if _, ok := c.Capabilities[name]; !ok {
				return fmt.Errorf("golden path %q references unknown capability %q", gp.Name, name)
			}
		}
	}
	return nil
}

// FindGoldenPath looks up a golden path by name.
func (c *Catalog) FindGoldenPath(name string) (*GoldenPath, bool) {
	// Index into the slice rather than taking the address of a range variable —
	// clearer about what the pointer refers to, and safe on every Go version.
	for i := range c.GoldenPaths {
		if c.GoldenPaths[i].Name == name {
			return &c.GoldenPaths[i], true
		}
	}
	return nil, false
}

// GoldenPathNames returns every declared name, for "did you mean" error messages.
func (c *Catalog) GoldenPathNames() []string {
	out := make([]string, 0, len(c.GoldenPaths))
	for _, gp := range c.GoldenPaths {
		out = append(out, gp.Name)
	}
	return out
}

// CapabilityNames returns every declared capability, sorted, for error messages.
func (c *Catalog) CapabilityNames() []string {
	out := make([]string, 0, len(c.Capabilities))
	for name := range c.Capabilities {
		out = append(out, name)
	}
	sort.Strings(out) // map order is random; sort so error text is stable
	return out
}
```

> **Why validate at load?** Every error above would otherwise surface as a confusing failure deep inside
> template rendering — or worse, as silently wrong output that gets committed. Validating config once at
> the boundary, with an error naming the offending key, is the difference between a tool people trust and
> one they debug.

---

## `internal/templater/render.go`

### Types

```go
package templater

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"scaffolder/internal/catalog"
)

// Config is the data every template receives.
type Config struct {
	TeamName     string
	SystemName   string
	AppName      string
	Env          string
	Runtime      string
	Capabilities []string // ordered + deduped names, NOT a map — see CapabilityView
}

// CapabilityView is the data a *single* capability template receives.
// Embedding Config means [[ .TeamName ]] still resolves, while .Name/.Module/.Version
// are scoped to just this capability — so postgres.tf.tmpl never has to look itself
// up with `index .Capabilities "postgres"`.
type CapabilityView struct {
	Config
	Name       string
	Module     string
	Version    string
	SourceBase string
}

// Renderer turns a catalog + a Config into a Plan. It never touches the output tree.
type Renderer struct {
	CatalogFS fs.FS // rooted AT 1-platform-catalog
	Spec      *catalog.Catalog
}

// Plan is every file a verb would produce. Keys are output-root-relative and
// slash-separated. Nothing has touched disk yet.
type Plan struct {
	Files map[string][]byte
}

func newPlan() *Plan { return &Plan{Files: make(map[string][]byte)} }

// add records one file, refusing silent overwrites. If two template sources ever
// resolve to the same destination, that's a catalog bug and you want to hear about it.
func (p *Plan) add(dest string, content []byte) error {
	if _, exists := p.Files[dest]; exists {
		return fmt.Errorf("two templates both target %s", dest)
	}
	p.Files[dest] = content
	return nil
}

// Paths returns destinations in sorted order — the basis of all deterministic output.
func (p *Plan) Paths() []string {
	out := make([]string, 0, len(p.Files))
	for k := range p.Files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

> **Why sorted `Paths()`?** Go randomizes map iteration order deliberately. Anything that ranges over
> `p.Files` directly — writing, logging, diffing — produces different output every run, and your golden
> tests will flap intermittently. Sort once here, then use `Paths()` everywhere else.

### Write and diff

```go
// WriteTo writes every planned file under root, creating parent directories as needed.
func (p *Plan) WriteTo(root string) error {
	for _, rel := range p.Paths() { // sorted → deterministic log/error ordering
		// Plan keys are slash-separated (fs convention); convert for the host OS.
		full := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", rel, err)
		}
		if err := os.WriteFile(full, p.Files[rel], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// DiffAgainst reports what WriteTo would change, without changing it.
// This is what --dry-run prints.
func (p *Plan) DiffAgainst(root string) (string, error) {
	var b strings.Builder
	for _, rel := range p.Paths() {
		full := filepath.Join(root, filepath.FromSlash(rel))
		existing, err := os.ReadFile(full)

		switch {
		case errors.Is(err, fs.ErrNotExist):
			fmt.Fprintf(&b, "  create  %s (%d bytes)\n", rel, len(p.Files[rel]))
		case err != nil:
			return "", fmt.Errorf("read %s: %w", full, err)
		case bytes.Equal(existing, p.Files[rel]):
			fmt.Fprintf(&b, "  same    %s\n", rel)
		default:
			fmt.Fprintf(&b, "  MODIFY  %s (%d -> %d bytes)\n", rel, len(existing), len(p.Files[rel]))
		}
	}
	return b.String(), nil
}
```

> **`errors.Is(err, fs.ErrNotExist)`, not `os.IsNotExist(err)`.** The older helper doesn't unwrap wrapped
> errors, so it silently returns false for anything wrapped with `%w`. `errors.Is` walks the whole chain.
> Since Go 1.13 it's the correct form; `os.IsNotExist` survives only for compatibility.

### Destination resolution

```go
// destination resolves a destinations-table key into an output-root-relative path.
// This is the ONLY place output paths are computed — no filepath.Join("..","..") anywhere.
func (r *Renderer) destination(key string, cfg Config) (string, error) {
	tpl, ok := r.Spec.Destinations[key]
	if !ok {
		return "", fmt.Errorf("catalog has no destination for %q", key)
	}

	dest := strings.NewReplacer(
		"{team}", cfg.TeamName,
		"{system}", cfg.SystemName,
		"{app}", cfg.AppName,
		"{env}", cfg.Env,
	).Replace(tpl)

	// A leftover brace means the catalog has a typo like {teamm} — fail loudly
	// rather than writing a directory literally named "{teamm}-gitops".
	if i := strings.IndexByte(dest, '{'); i >= 0 {
		return "", fmt.Errorf("destination %q has an unresolved placeholder: %s", key, dest[i:])
	}
	return path.Clean(dest), nil
}

// SystemMarker is the file whose existence proves `create-system` has been run.
// Derived from the destinations table so the CLI never hardcodes the gitops layout.
func (r *Renderer) SystemMarker(cfg Config) (string, error) {
	dest, err := r.destination("blueprints/system/gitops", cfg)
	if err != nil {
		return "", err
	}
	return path.Join(dest, "applicationset.yaml"), nil
}
```

### Template rendering

```go
// renderTemplate reads one template out of the catalog FS and executes it against data.
func (r *Renderer) renderTemplate(srcPath string, data any) ([]byte, error) {
	raw, err := fs.ReadFile(r.CatalogFS, srcPath)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", srcPath, err)
	}

	// .Delims MUST come before .Parse — chaining it after silently does nothing
	// and you get literal "[[ .AppName ]]" in your output.
	tmpl, err := template.New(path.Base(srcPath)).Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", srcPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", srcPath, err)
	}
	return buf.Bytes(), nil
}

// renderString evaluates [[ ]] inside a path fragment (e.g. a directory named
// "[[ .AppName ]]") or any other short string.
func (r *Renderer) renderString(s string, data any) (string, error) {
	tmpl, err := template.New("inline").Delims("[[", "]]").Parse(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

> **Why read bytes instead of `ParseFS`?** `ParseFiles`/`ParseFS` register each template under its
> *basename*, so `tmpl.Execute` silently executes the wrong (or an empty) template unless the name you
> passed to `template.New` happens to match. Reading the bytes and calling `.Parse` yields exactly one
> template with no hidden naming contract. Your current `render.go` relies on the fragile form.

### The walk

```go
// walkAndPlan renders every file under srcDir (a path inside CatalogFS)
// into destDir (output-root-relative), recording results in plan.
func (r *Renderer) walkAndPlan(srcDir, destDir string, data any, plan *Plan) error {
	return fs.WalkDir(r.CatalogFS, srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// A Plan is a set of FILES. Empty template dirs produce nothing, and
			// WriteTo mkdir -p's each file's parents. (Behavior change from the
			// old walkAndRender, which created empty dirs as a side effect.)
			return nil
		}

		// No path.Rel exists in the stdlib, so trim the prefix by hand.
		// Assumes srcDir != "." — true for every call site below.
		rel := strings.TrimPrefix(strings.TrimPrefix(p, srcDir), "/")

		// Evaluate [[ ]] inside directory and file NAMES, then drop the .tmpl suffix.
		renderedRel, err := r.renderString(rel, data)
		if err != nil {
			return fmt.Errorf("render path %s: %w", rel, err)
		}
		renderedRel = strings.TrimSuffix(renderedRel, ".tmpl")

		content, err := r.renderTemplate(p, data)
		if err != nil {
			return err
		}
		return plan.add(path.Join(destDir, renderedRel), content)
	})
}
```

> **The `path` vs `filepath` rule — the trap most likely to cost you an evening.** `io/fs` paths are
> always slash-separated, unrooted, and contain no `..`. On macOS/Linux `filepath` is *also*
> slash-separated, so mixing them compiles, runs, and produces subtly wrong Plan keys that only surface in
> your golden tests. Rule: **`path` for everything inside the FS, `filepath` only in
> `WriteTo`/`DiffAgainst`.**

### The three verbs

```go
// PlanTeam scaffolds the tenancy boundary. Once per team.
func (r *Renderer) PlanTeam(cfg Config) (*Plan, error) {
	if cfg.TeamName == "" {
		return nil, errors.New("team name is required")
	}

	plan := newPlan()
	// After the Phase 0.5 key normalization, the destinations key IS the source path.
	for _, src := range []string{"blueprints/team/gitops", "blueprints/team/infra"} {
		dest, err := r.destination(src, cfg)
		if err != nil {
			return nil, err
		}
		if err := r.walkAndPlan(src, dest, cfg, plan); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// PlanSystem scaffolds the per-system ArgoCD ApplicationSet. Once per system.
func (r *Renderer) PlanSystem(cfg Config) (*Plan, error) {
	if cfg.TeamName == "" || cfg.SystemName == "" {
		return nil, errors.New("team name and system name are required")
	}

	const src = "blueprints/system/gitops"
	dest, err := r.destination(src, cfg)
	if err != nil {
		return nil, err
	}

	plan := newPlan()
	return plan, r.walkAndPlan(src, dest, cfg, plan)
}

// PlanService composes a golden path: runtime + service metadata + delivery values
// + one .tf file per capability. Repeatable, N per system.
func (r *Renderer) PlanService(cfg Config) (*Plan, error) {
	if cfg.TeamName == "" || cfg.SystemName == "" || cfg.AppName == "" {
		return nil, errors.New("team, system and app names are required")
	}
	if cfg.Env == "" {
		cfg.Env = "dev" // AGENTS decision #3/#4: scaffold to dev only
	}

	plan := newPlan()

	// 1. Runtime source code.
	runtimeSrc := path.Join("building-blocks/runtimes", cfg.Runtime)
	if _, err := fs.Stat(r.CatalogFS, runtimeSrc); err != nil {
		return nil, fmt.Errorf("unknown runtime %q: no templates at %s", cfg.Runtime, runtimeSrc)
	}
	dest, err := r.destination("building-blocks/runtimes", cfg)
	if err != nil {
		return nil, err
	}
	if err := r.walkAndPlan(runtimeSrc, dest, cfg, plan); err != nil {
		return nil, err
	}

	// 2. Service metadata (catalog-info.yaml) and 3. delivery values — same shape.
	for _, src := range []string{"building-blocks/service-meta", "building-blocks/delivery/release"} {
		dest, err := r.destination(src, cfg)
		if err != nil {
			return nil, err
		}
		if err := r.walkAndPlan(src, dest, cfg, plan); err != nil {
			return nil, err
		}
	}

	// 4. Capabilities — one file each, rendered with a view scoped to that capability.
	capDest, err := r.destination("building-blocks/capabilities", cfg)
	if err != nil {
		return nil, err
	}
	for _, name := range cfg.Capabilities { // slice, so order is stable
		spec, ok := r.Spec.Capabilities[name]
		if !ok {
			return nil, fmt.Errorf("unknown capability %q (available: %s)",
				name, strings.Join(r.Spec.CapabilityNames(), ", "))
		}

		view := CapabilityView{
			Config:     cfg,
			Name:       name,
			Module:     spec.Module,
			Version:    spec.Version,
			SourceBase: r.Spec.CapabilitiesSourceBase,
		}

		src := path.Join("building-blocks/capabilities", name+".tf.tmpl")
		content, err := r.renderTemplate(src, view)
		if err != nil {
			return nil, err
		}
		if err := plan.add(path.Join(capDest, name+".tf"), content); err != nil {
			return nil, err
		}
	}

	return plan, nil
}
```

---

## `cmd/cli/root.go`

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"scaffolder/internal/catalog"
	"scaffolder/internal/templater"

	"github.com/spf13/cobra"
)

var (
	catalogRoot string
	outputRoot  string
	dryRun      bool

	cfg      templater.Config
	renderer *templater.Renderer
)

// findRepoRoot walks up from start looking for the catalog marker file.
// This is what lets the binary run from ANY directory instead of only
// from 2-idp-scaffolder/golang/.
func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "1-platform-catalog", "catalog.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached the filesystem root without finding it
			return "", fmt.Errorf(
				"not inside the platform repo: no 1-platform-catalog/catalog.yaml in %s or any parent",
				start)
		}
		dir = parent
	}
}

var rootCmd = &cobra.Command{
	Use:   "scaffolder",
	Short: "Self-service scaffolding for the platform's golden paths",

	// Without this, Cobra dumps the full help text after every RUNTIME error,
	// burying a one-line "unknown capability" under 30 lines of flag docs.
	// Usage still prints for genuine usage errors (bad flags).
	SilenceUsage: true,

	// Runs before every subcommand: resolve roots once, load + validate the
	// catalog once, build the shared Renderer.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if catalogRoot == "" || outputRoot == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repo, err := findRepoRoot(wd)
			if err != nil {
				return err
			}
			if catalogRoot == "" {
				catalogRoot = filepath.Join(repo, "1-platform-catalog")
			}
			if outputRoot == "" {
				outputRoot = filepath.Join(repo, "3-tenant-workloads")
			}
		}

		spec, err := catalog.Load(filepath.Join(catalogRoot, "catalog.yaml"))
		if err != nil {
			return err
		}

		// os.DirFS roots the FS at the catalog, so every path inside the
		// renderer is a clean relative path like "building-blocks/runtimes/go".
		renderer = &templater.Renderer{CatalogFS: os.DirFS(catalogRoot), Spec: spec}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1) // Cobra already printed the error to stderr
	}
}

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&catalogRoot, "catalog-root", "", "path to 1-platform-catalog (default: auto-discovered)")
	f.StringVar(&outputRoot, "output-root", "", "path to 3-tenant-workloads (default: auto-discovered)")
	f.BoolVar(&dryRun, "dry-run", false, "print what would be written without writing it")
}

// apply is the single place any command turns a Plan into an effect.
// --dry-run is not a parallel code path: it is this same path, minus WriteTo.
func apply(plan *templater.Plan) error {
	if dryRun {
		diff, err := plan.DiffAgainst(outputRoot)
		if err != nil {
			return err
		}
		fmt.Printf("dry run — %d file(s) planned under %s:\n%s", len(plan.Files), outputRoot, diff)
		return nil
	}

	if err := plan.WriteTo(outputRoot); err != nil {
		return err
	}
	for _, p := range plan.Paths() {
		fmt.Println("  wrote", p)
	}
	return nil
}
```

---

## `cmd/cli/add_service.go`

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	goldenPathFlag   string
	capabilitiesFlag string
)

var addServiceCmd = &cobra.Command{
	Use:   "add-service",
	Short: "Add a microservice to a system (runtime + delivery + capabilities)",
	RunE: func(cmd *cobra.Command, args []string) error {

		// 1. SEED from the golden path, if one was named.
		if goldenPathFlag != "" {
			gp, ok := renderer.Spec.FindGoldenPath(goldenPathFlag)
			if !ok {
				return fmt.Errorf("unknown golden path %q (available: %s)",
					goldenPathFlag, strings.Join(renderer.Spec.GoldenPathNames(), ", "))
			}

			// The seed must YIELD to an explicit flag, not clobber it —
			// otherwise "--runtime python --golden-path go-service-postgres"
			// silently gives you Go. (This was a real bug in the old code.)
			if cfg.Runtime == "" {
				cfg.Runtime = gp.Runtime
			}
			cfg.Capabilities = append(cfg.Capabilities, gp.Capabilities...)
		}

		// 2. OVERRIDE / EXTEND with explicit --capabilities.
		for _, c := range strings.Split(capabilitiesFlag, ",") {
			if c = strings.TrimSpace(c); c != "" {
				cfg.Capabilities = append(cfg.Capabilities, c)
			}
		}
		cfg.Capabilities = dedupe(cfg.Capabilities)

		// 3. Validate AFTER seeding — checking before it meant --golden-path
		// alone always failed with "runtime required".
		if cfg.Runtime == "" {
			return fmt.Errorf("need --runtime or --golden-path (available paths: %s)",
				strings.Join(renderer.Spec.GoldenPathNames(), ", "))
		}
		if err := requireSystem(); err != nil {
			return err
		}

		plan, err := renderer.PlanService(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Planning %s [runtime=%s capabilities=%v env=%s]\n",
			cfg.AppName, cfg.Runtime, cfg.Capabilities, cfg.Env)
		return apply(plan)
	},
}

// dedupe removes duplicates while preserving first-seen order, so
// "--golden-path go-service-postgres --capabilities postgres,s3" renders
// postgres once, and output ordering stays stable for golden tests.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// requireSystem enforces the create-system → add-service ordering.
// The two verbs stay separate because they sit on opposite sides of a permission
// boundary (ApplicationSets are argocd-namespace, platform-approved); the UX cost
// is paid back here, with an error that names the exact next command.
func requireSystem() error {
	marker, err := renderer.SystemMarker(cfg)
	if err != nil {
		return err
	}
	full := filepath.Join(outputRoot, filepath.FromSlash(marker))
	if _, err := os.Stat(full); err == nil {
		return nil
	}
	return fmt.Errorf(`system %q does not exist for team %q.

  Expected: %s

  Create it first (requires platform-team approval):
      scaffolder create-system --team-name %s --system-name %s`,
		cfg.SystemName, cfg.TeamName, full, cfg.TeamName, cfg.SystemName)
}

func init() {
	rootCmd.AddCommand(addServiceCmd)

	f := addServiceCmd.Flags()
	f.StringVarP(&cfg.TeamName, "team-name", "t", "", "owning team")
	f.StringVarP(&cfg.SystemName, "system-name", "s", "", "logical system")
	f.StringVarP(&cfg.AppName, "app-name", "a", "", "application name")
	f.StringVar(&cfg.Env, "env", "dev", "target environment")
	f.StringVar(&goldenPathFlag, "golden-path", "", "seed config from a named golden path")
	f.StringVar(&cfg.Runtime, "runtime", "", "override the runtime (e.g. go, python)")
	f.StringVar(&capabilitiesFlag, "capabilities", "", "extra capabilities, comma-separated")

	addServiceCmd.MarkFlagRequired("team-name")
	addServiceCmd.MarkFlagRequired("system-name")
	addServiceCmd.MarkFlagRequired("app-name")
}
```

## `cmd/cli/create_system.go` and `onboard_team.go`

Same shape, shorter — build `cfg` from flags, plan, apply:

```go
var createSystemCmd = &cobra.Command{
	Use:   "create-system",
	Short: "Create a logical system and its ArgoCD ApplicationSet",
	RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := renderer.PlanSystem(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Planning system %s for team %s\n", cfg.SystemName, cfg.TeamName)
		return apply(plan)
	},
}

func init() {
	rootCmd.AddCommand(createSystemCmd)
	f := createSystemCmd.Flags()
	f.StringVarP(&cfg.TeamName, "team-name", "t", "", "owning team")
	f.StringVarP(&cfg.SystemName, "system-name", "s", "", "logical system")
	createSystemCmd.MarkFlagRequired("team-name")
	createSystemCmd.MarkFlagRequired("system-name")
}
```

`onboard_team.go` is identical with `PlanTeam` and only `--team-name`.

---

## `internal/templater/render_test.go`

```bash
go get github.com/google/go-cmp
```

```go
package templater

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"scaffolder/internal/catalog"
)

// go test ./... -update  regenerates every golden tree.
// A package-level flag in a _test.go file is the standard Go idiom — `go test`
// already parses flags, so this just works.
var update = flag.Bool("update", false, "rewrite golden files")

// Relative to internal/templater/, which is where `go test` sets the CWD.
const catalogDir = "../../../1-platform-catalog"

func newTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	spec, err := catalog.Load(filepath.Join(catalogDir, "catalog.yaml"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return &Renderer{CatalogFS: os.DirFS(catalogDir), Spec: spec}
}

func testConfig(runtime string, caps []string) Config {
	return Config{
		TeamName: "payments", SystemName: "checkout", AppName: "checkout-api",
		Env: "dev", Runtime: runtime, Capabilities: caps,
	}
}

// TestGoldenPaths derives its cases FROM the catalog rather than hardcoding them,
// so adding a golden path to catalog.yaml without generating fixtures fails the
// suite immediately. The catalog can never silently drift from its tests.
func TestGoldenPaths(t *testing.T) {
	r := newTestRenderer(t)
	if len(r.Spec.GoldenPaths) == 0 {
		t.Fatal("catalog declares no golden paths")
	}

	for _, gp := range r.Spec.GoldenPaths {
		t.Run(gp.Name, func(t *testing.T) {
			plan, err := r.PlanService(testConfig(gp.Runtime, gp.Capabilities))
			if err != nil {
				t.Fatalf("PlanService: %v", err)
			}
			compareGolden(t, filepath.Join("testdata", "golden", gp.Name), plan)
		})
	}
}

func TestUnknownCapabilityIsRejected(t *testing.T) {
	r := newTestRenderer(t)
	if _, err := r.PlanService(testConfig("go", []string{"bogus"})); err == nil {
		t.Fatal("expected an error for an unknown capability")
	}
}

// TestDeterministic catches accidental map-iteration order leaking into output.
func TestDeterministic(t *testing.T) {
	r := newTestRenderer(t)
	a, err := r.PlanService(testConfig("go", []string{"postgres", "s3"}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.PlanService(testConfig("go", []string{"postgres", "s3"}))
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(a.Files, b.Files); d != "" {
		t.Errorf("two identical runs produced different output:\n%s", d)
	}
}

func compareGolden(t *testing.T, dir string, plan *Plan) {
	t.Helper()

	if *update {
		// RemoveAll first so deleted templates disappear from the fixtures too.
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		if err := plan.WriteTo(dir); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %d file(s) in %s", len(plan.Files), dir)
		return
	}

	want, err := readTree(dir)
	if err != nil {
		t.Fatalf("read golden %s: %v\n(first run? go test ./... -update)", dir, err)
	}

	// Compare the file SET first. A missing file otherwise shows up as N content
	// diffs and you have to reconstruct what actually happened.
	if d := cmp.Diff(sortedKeys(want), plan.Paths()); d != "" {
		t.Fatalf("file set mismatch (-golden +got):\n%s", d)
	}

	for _, p := range plan.Paths() {
		// Compare as string, not []byte — cmp prints readable text diffs for strings
		// and unreadable byte slices otherwise.
		if d := cmp.Diff(string(want[p]), string(plan.Files[p])); d != "" {
			t.Errorf("%s (-golden +got):\n%s", p, d)
		}
	}
}

// readTree loads a golden directory into the same shape as Plan.Files.
func readTree(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = b // Plan keys are slash-separated
		return nil
	})
	return out, err
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

> **The point of all this.** After `go test ./... -update`, `git diff testdata/` *is* your review of what a
> platform change did to every customer. Bump a module `version:` in `catalog.yaml`, run `-update`, and the
> diff shows you the exact blast radius before you ship it. That's your "git diff = the PR" idea turned
> inward on the platform team's own changes — and it's what lets you refactor the catalog without fear.

---

## Quick reference — the traps, condensed

| Symptom | Cause |
|---|---|
| Literal `[[ .AppName ]]` in output | `.Delims()` called after `.Parse()` instead of before |
| Empty output file, no error | `ParseFiles`/`ParseFS` template-name mismatch — read bytes and `.Parse` instead |
| `file does not exist` on a path that clearly exists | leading `/` or `..` in an `io/fs` path — they must be unrooted |
| Output at `.../3-tenant-workloads/../../3-tenant-workloads/...` | `Plan.Files` keys aren't output-root-relative |
| Golden tests pass, fail, then pass again | ranging over `p.Files` directly instead of `p.Paths()` |
| `--golden-path X` errors "runtime required" | runtime validated before the seed runs |
| `--runtime python --golden-path go-...` yields Go | unconditional `cfg.Runtime = gp.Runtime` |
| Help text dumped after every error | missing `SilenceUsage: true` |
| Directory literally named `{team}-gitops` | unresolved placeholder — the `strings.IndexByte(dest, '{')` guard catches it |
