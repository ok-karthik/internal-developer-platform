# Code Walkthrough — The Go IDP Scaffolder, Line by Line

This document is a comprehensive, production-grade walkthrough of the Go IDP Scaffolder (`2-idp-scaffolder/golang/`). It explains the architecture, design decisions, memory mechanics, error handling, and end-to-end execution flow.

Every trace, path, and code snippet below reflects the current codebase and has been verified against the test suite.

```bash
# Example invocation traced throughout:
scaffolder add-service --team-name payments --app-name checkout-api \
                       --golden-path go-service-postgres
```

---

## 📑 Contents

| § | Section |
|---|---|
| 1 | [Core Principles & System Architecture](#1-core-principles--system-architecture) |
| 2 | [Program Entrypoint & Cobra Lifecycle](#2-program-entrypoint--cobra-lifecycle) |
| 3 | [Boot Sequence (`PersistentPreRunE`) & Version Pinning](#3-boot-sequence-persistentprerune--version-pinning) |
| 4 | [Catalog Ingestion & Schema Validation](#4-catalog-ingestion--schema-validation) |
| 5 | [Command Layer & Pure Config Resolution](#5-command-layer--pure-config-resolution) |
| 6 | [Output Contract & Destination Mapping](#6-output-contract--destination-mapping) |
| 7 | [Directory Traversal & Context Cancellation](#7-directory-traversal--context-cancellation) |
| 8 | [Safe Rendering, Overwrite Protection & Writer Abstraction](#8-safe-rendering-overwrite-protection--writer-abstraction) |
| 9 | [Path Templating Engine](#9-path-templating-engine) |
| 10 | [Idiomatic Go Design Patterns & Memory Mechanics](#10-idiomatic-go-design-patterns--memory-mechanics) |
| 11 | [Full End-to-End Execution Trace](#11-full-end-to-end-execution-trace) |
| 12 | [Architectural Defect Resolution & Answer Key](#12-architectural-defect-resolution--answer-key) |
| 13 | [Testing Architecture & Golden File Harness](#13-testing-architecture--golden-file-harness) |
| 14 | [Suggested Reading Order](#14-suggested-reading-order) |

---

## 1. Core Principles & System Architecture

At its core, the Go IDP Scaffolder performs one fundamental task:

> **Traverse a catalog template tree, execute every template in-memory via `text/template`, and write the output to paths dynamically resolved from a declarative destination lookup table.**

```
                           ┌────────────────────────────────────────────────────────┐
                           │                  1-platform-catalog/                   │
                           │  - catalog.yaml (Schema, Golden Paths, Destinations)   │
                           │  - per-tenant/ (Team tenancy foundations: apps, infra)   │
                           │  - per-service/ (Runtimes, Service-Meta, Capabilities) │
                           └──────────────────────────┬─────────────────────────────┘
                                                      │
                                   CatalogFS (fs.FS)  │
                                                      ▼
┌─────────────────────────┐               ┌───────────────────────┐
│       User Flags        │──────────────▶│   templater.Resolve   │ (Pure function: seeds golden path,
│ (--team, --app, --path) │               └───────────┬───────────┘  overrides flags, sorts capabilities)
└─────────────────────────┘                           │
                                         Resolved     │
                                          Config      ▼
                                          ┌───────────────────────┐
                                          │  templater.Renderer   │ (In-memory execution + Cancellation)
                                          └───────────┬───────────┘
                                                      │
                                                      ▼
                                          ┌───────────────────────┐
                                          │   Writer Interface    │
                                          │ - OSWriter (Disk I/O) │
                                          │ - DryRunWriter (Mock) │
                                          └───────────┬───────────┘
                                                      │
                                                      ▼
                                          ┌───────────────────────┐
                                          │  3-tenant-workloads/  │
                                          │  <team>/{apps,infra,  │
                                          │         gitops}/      │
                                          └───────────────────────┘
```

### Package Responsibilities

```
2-idp-scaffolder/golang/
├── main.go                       → Minimal process entrypoint; delegates to cli.Execute()
├── cmd/cli/
│   ├── root.go                   → Root Cobra command, signal trapping, version pinning, remote fetching
│   ├── onboard_team.go           → Subcommand for initial team tenancy scaffolding (per-tenant/)
│   └── add_service.go            → Subcommand for service golden-path scaffolding (per-service/)
└── internal/
    ├── catalog/
    │   ├── catalog.go            → Parses catalog.yaml, validates schema against template tree
    │   └── catalog_test.go       → Unit tests for catalog schema validation
    └── templater/
        ├── errors.go             → Sentinel errors & structured ValidationError type
        ├── writer.go             → Writer interface, OSWriter, and DryRunWriter abstractions
        ├── resolve.go            → Pure config resolution, deduplication, and sorting
        ├── resolve_test.go       → Table-driven tests asserting resolution semantics
        ├── render.go             → Multi-step template engine with context cancellation & provisioner dispatch
        ├── render_test.go        → Golden-file testing, error-path tests, dry-run & context tests
        └── testdata/             → Golden files for byte-exact regression testing
```

### The Output Layout Contract

The scaffolder targets a tenant-first monorepo layout that splits cleanly into independent polyrepos via `git subtree split`:

```
3-tenant-workloads/<team>/
├── apps/                             ← Root of <team>-apps repo (Application Source)
│   ├── CODEOWNERS                    ← Team owns *
│   └── <app>/                        go.mod, main.go, catalog-info.yaml
├── infra/                            ← Root of <team>-infra repo (Terraform)
│   ├── CODEOWNERS                    ← Platform owns platform/; Team owns apps/
│   ├── platform/                     providers.tf, team-iam.tf (Platform-owned)
│   └── apps/<app>/<env>/             postgres.tf (Team-owned Terraform capability claims)
└── gitops/                           ← Root of <team>-gitops repo (GitOps Delivery)
    ├── CODEOWNERS                    ← Platform owns platform/; Team owns apps/
    ├── platform/
    │   ├── team/                     AppProject, Namespace, NetworkPolicy, ResourceQuota
    │   └── applicationsets/          <team>.yaml (ArgoCD ApplicationSet)
    └── apps/<app>/<env>/             values.yaml, s3.yaml, iam.yaml (ACK CRD claims)
```

---

## 2. Program Entrypoint & Cobra Lifecycle

### `main.go`

```go
package main

import "scaffolder/cmd/cli"

func main() {
	cli.Execute()
}
```

`main.go` contains zero logic. It satisfies the Go `package main` compiler contract and delegates immediately to the `cli` package.

### Cobra Initialization Order

Go executes all `init()` functions across imported packages **before** `main()` is entered:

```
Go Runtime Initialization
  ├── cmd/cli/add_service.go  init() → Registers addServiceCmd with rootCmd & defines flags
  ├── cmd/cli/onboard_team.go init() → Registers onboardTeamCmd with rootCmd & defines flags
  ├── cmd/cli/root.go         init() → Registers persistent flags (--output-root, --catalog-ref, --dry-run, --force)
  └── main()                         → Calls cli.Execute()
```

### `Execute()` with OS Signal Trapping

```go
func Execute() {
	// Listen for Ctrl+C (os.Interrupt / SIGINT) and SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130) // Standard Unix exit code for SIGINT (128 + 2)
		}
		os.Exit(1)
	}
}
```

#### Why this matters:
1. **Signal Trapping (`signal.NotifyContext`)**: Connects Go's cancellation context directly to operating system interrupts. Pressing `Ctrl+C` cancels `ctx`, allowing in-flight network downloads and file writes to abort cleanly.
2. **Standard Exit Codes**: If aborted via `Ctrl+C`, the process exits with `130` instead of a generic failure code.
3. **`ExecuteContext(ctx)`**: Propagates the signal-aware context to `cmd.Context()` for all Cobra lifecycle hooks (`PersistentPreRunE`, `RunE`).

---

## 3. Boot Sequence (`PersistentPreRunE`) & Version Pinning

`PersistentPreRunE` executes before any subcommand's `RunE`. It acts as the dependency injector for the CLI.

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
	// 1. Default output directory to current working directory (cwd)
	if outputRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		outputRoot = wd
	}

	// 2. Fetch remote catalog or use local directory
	if catalogRoot == "" {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Fetching templates from GitHub..."
		s.Start()

		var err error
		catalogRoot, err = fetchRemoteCatalog(cmd.Context(), catalogRef)
		if err != nil {
			s.Stop()
			return err
		}

		s.Stop()
		fmt.Println("✅ Templates fetched!")
	}

	// 3. Ingest and validate catalog specification
	catalogFS := os.DirFS(catalogRoot)
	spec, err := catalog.LoadCatalog(catalogFS)
	if err != nil {
		return err
	}

	// 4. Select Writer implementation (DryRun vs OS)
	var w templater.Writer = templater.OSWriter{}
	if dryRun {
		w = templater.DryRunWriter{}
	}

	// 5. Instantiate Renderer
	renderer = &templater.Renderer{
		CatalogFS: catalogFS,
		Spec:      spec,
		OutputDir: outputRoot,
		Writer:    w,
		Force:     force,
	}

	return nil
}
```

### Build-Time Version Stamping & Cache Management

The catalog reference is locked at build time via linker flags (`-ldflags`):

```go
var catalogRef = "main" // Overridden at build time: -ldflags "-X 'scaffolder/cmd/cli.catalogRef=v1.2.0'"
```

#### Cache Eviction & Remote Download:
```go
func fetchRemoteCatalog(ctx context.Context, version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory for catalog cache: %w", err)
	}
	cacheDir := filepath.Join(home, ".scaffolder-cache", version)

	if catalogRefresh {
		if err := os.RemoveAll(cacheDir); err != nil {
			return "", fmt.Errorf("clearing catalog cache: %w", err)
		}
	}

	url := "git::https://github.com/ok-karthik/internal-developer-platform.git//1-platform-catalog?ref=" + version

	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  cacheDir,
		Mode: getter.ClientModeAny,
	}
	if err := client.Get(); err != nil {
		_ = os.RemoveAll(cacheDir) // Clean up partial download on failure
		return "", fmt.Errorf("fetching catalog %s: %w", version, err)
	}

	return cacheDir, nil
}
```

---

## 4. Catalog Ingestion & Schema Validation

`internal/catalog/catalog.go` parses and validates `catalog.yaml` against the virtual filesystem (`fs.FS`).

```go
type Catalog struct {
	GoldenPaths            []GoldenPath          `yaml:"golden-paths"`
	Runtimes               map[string]Runtime    `yaml:"runtimes"`
	CapabilitiesSourceBase string                `yaml:"capabilities_source_base"`
	Capabilities           map[string]Capability `yaml:"capabilities"`
	Destinations           map[string]string     `yaml:"destinations"`
}
```

### Fast-Fail Validation

The catalog validation executes at parse time (`LoadCatalog`), preventing half-rendered trees on disk:

```go
func LoadCatalog(catalogFS fs.FS) (*Catalog, error) {
	data, err := fs.ReadFile(catalogFS, "catalog.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading catalog.yaml: %w", err)
	}

	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parsing catalog.yaml: %w", err)
	}

	if err := catalog.validate(catalogFS); err != nil {
		return nil, fmt.Errorf("invalid catalog.yaml: %w", err)
	}

	return &catalog, nil
}
```

Validation ensures:
1. Every destination in `requiredDestinations` (`per-tenant/apps`, `per-tenant/infra`, `per-tenant/gitops`, `per-service/apps/runtimes`, etc.) is declared.
2. Every declared runtime points to a directory that actually exists on `CatalogFS`.
3. Every golden path references a runtime declared in `runtimes:`.

---

## 5. Command Layer & Pure Config Resolution

### The `Resolve` Function (`internal/templater/resolve.go`)

Config resolution is a **pure function**—it has zero I/O, does not touch globals, and performs no disk access.

```go
func Resolve(spec *catalog.Catalog, goldenPath string, in Config) (Config, error) {
	out := in
	// 1. Deep clone slice elements to prevent backing array sharing
	out.Capabilities = slices.Clone(in.Capabilities)

	// 2. Seed from golden path if specified
	if goldenPath != "" {
		gp, found := spec.FindGoldenPath(goldenPath)
		if !found {
			return Config{}, &ValidationError{
				Field: "golden-path",
				Value: goldenPath,
				Err:   ErrUnknownGoldenPath,
			}
		}

		if out.Runtime == "" {
			out.Runtime = gp.Runtime
		}
		out.Capabilities = append(out.Capabilities, gp.Capabilities...)
	}

	// 3. Enforce runtime presence
	if out.Runtime == "" {
		return Config{}, &ValidationError{
			Field: "runtime",
			Err:   ErrRuntimeRequired,
		}
	}

	// 4. Validate runtime against catalog
	if _, ok := spec.Runtimes[out.Runtime]; !ok {
		return Config{}, &ValidationError{
			Field: "runtime",
			Value: out.Runtime,
			Err:   ErrUnknownRuntime,
		}
	}

	// 5. Deduplicate and sort capabilities deterministically
	slices.Sort(out.Capabilities)
	out.Capabilities = slices.Compact(out.Capabilities)

	return out, nil
}
```

#### Key Guarantees:
- **Flag Precedence**: Explicit CLI flags override golden-path defaults (`out.Runtime == ""`).
- **Defensive Slice Cloning (`slices.Clone`)**: Prevents mutating the loaded `*catalog.Catalog` in memory.
- **Deterministic Sorting & Deduplication (`slices.Sort` + `slices.Compact`)**: Ensures diffs across runs are identical regardless of flag order, and elegantly drops duplicates.

---

## 6. Output Contract & Destination Mapping

`resolveDestination` replaces placeholder variables `{team}`, `{system}`, `{app}`, `{env}` and joins with `OutputDir`:

```go
func (r *Renderer) resolveDestination(destTemplate string, cfg Config) string {
	dest := destTemplate
	dest = strings.ReplaceAll(dest, "{team}", cfg.TeamName)
	dest = strings.ReplaceAll(dest, "{system}", cfg.SystemName)
	dest = strings.ReplaceAll(dest, "{app}", cfg.AppName)

	env := "dev"
	if cfg.Env != "" {
		env = cfg.Env
	}
	dest = strings.ReplaceAll(dest, "{env}", env)

	return filepath.Join(r.OutputDir, dest)
}
```

### Destination Routing Table (`catalog.yaml`)

| Source Directory | Output Path (`destinations:`) | Triggered By | Frequency |
|---|---|---|---|
| `per-tenant/apps/` | `{team}/apps/` | `onboard-team` | Once per team |
| `per-tenant/infra/` | `{team}/infra/` | `onboard-team` | Once per team |
| `per-tenant/gitops/` | `{team}/gitops/` | `onboard-team` | Once per team |
| `per-service/apps/runtimes/<lang>/` | `{team}/apps/{app}/` | `add-service` | Once per service |
| `per-service/apps/service-meta/` | `{team}/apps/{app}/` | `add-service` | Once per service |
| `per-service/gitops/release/` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | Per service per env |
| `per-service/infra/capabilities/*.tf.tmpl` | `{team}/infra/apps/{app}/{env}/` | `add-service` | Terraform capabilities |
| `per-service/gitops/capabilities/*.yaml.tmpl` | `{team}/gitops/apps/{app}/{env}/` | `add-service` | ACK CRD capabilities |

---

## 7. Directory Traversal & Context Cancellation

`walkAndRender` traverses the template tree on `CatalogFS` and renders each file:

```go
func (r *Renderer) walkAndRender(ctx context.Context, sourceDir string, targetDir string, cfg Config) error {
	handleFile := func(srcPath string, d fs.DirEntry, err error) error {
		// Fast abort if context was canceled (Ctrl+C or HTTP timeout)
		if err := ctx.Err(); err != nil {
			return err
		}

		if err != nil {
			return err
		}

		relPath := strings.TrimPrefix(srcPath, sourceDir)
		relPath = strings.TrimPrefix(relPath, "/")

		renderedRelPath, err := renderPath(relPath, cfg)
		if err != nil {
			return fmt.Errorf("failed to render path %s: %w", relPath, err)
		}

		targetPath := filepath.Join(targetDir, renderedRelPath)

		if d.IsDir() {
			return r.writer().MkdirAll(targetPath)
		}

		return r.processSingleTemplate(ctx, srcPath, targetPath, cfg)
	}

	return fs.WalkDir(r.CatalogFS, sourceDir, handleFile)
}
```

---

## 8. Safe Rendering, Overwrite Protection & Writer Abstraction

### The `Writer` Interface (`internal/templater/writer.go`)

Decouples file rendering from physical disk I/O, enabling `--dry-run` and clean unit testing:

```go
type Writer interface {
	WriteFile(path string, data []byte) error
	MkdirAll(path string) error
}

type OSWriter struct{}

func (w OSWriter) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func (w OSWriter) MkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

type DryRunWriter struct{}

func (w DryRunWriter) WriteFile(path string, data []byte) error {
	fmt.Printf("[WOULD WRITE] %s (%d bytes)\n", path, len(data))
	return nil
}

func (w DryRunWriter) MkdirAll(path string) error {
	return nil
}
```

### In-Memory Rendering & Overwrite Protection (`processSingleTemplate`)

```go
func (r *Renderer) processSingleTemplate(ctx context.Context, srcPath string, targetPath string, data any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	// 1. Overwrite Protection: Skip existing files unless --force is set
	if !r.Force {
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("[SKIP] %s (exists; use --force to overwrite)\n", targetPath)
			return nil
		}
	}

	// 2. Read template from CatalogFS
	rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", srcPath, err)
	}

	// 3. Parse template using custom [[ ]] delimiters
	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").Parse(string(rawBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", srcPath, err)
	}

	// 4. Memory-Buffer Execution: Prevent 0-byte truncated files on disk
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render template %s: %w", srcPath, err)
	}

	// 5. Write via Writer abstraction
	if err := r.writer().WriteFile(targetPath, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write output file %s: %w", targetPath, err)
	}

	fmt.Println(srcPath, "-->", targetPath)
	return nil
}
```

---

## 9. Path Templating Engine

Filenames and directories can contain template delimiters (e.g. `[[ .TeamName ]].yaml.tmpl`). `renderPath` dynamically resolves path tokens:

```go
func renderPath(pathStr string, cfg Config) (string, error) {
	tmpl, err := template.New("path").Delims("[[", "]]").Parse(pathStr)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

---

## 10. Idiomatic Go Design Patterns & Memory Mechanics

### 1. Implicit Interfaces vs Inheritance
Go implements interfaces implicitly. `ValidationError` implements the `error` interface simply by defining `Error() string`:

```go
type ValidationError struct {
	Field string // e.g., "runtime", "capability", "golden-path"
	Value string // e.g., "doesnotexist"
	Err   error  // Sentinel error
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("%s %q: %v", e.Field, e.Value, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Field, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
```

### 2. Error Inspection with `errors.Is` & `errors.As`
By implementing `Unwrap() error`, callers can inspect the root cause:

```go
if errors.Is(err, templater.ErrUnknownCapability) {
    // Branch on specific capability failure
}
```

### 3. Pointers vs Values
- **`*catalog.Catalog` (Pointer)**: Passed as a pointer because the catalog is a large data structure parsed once and shared read-only across operations.
- **`Config` (Value)**: Passed by value to functions like `RenderService(ctx, cfg)`. Since `Config` contains only small string headers and a slice header, passing by value guarantees the callee cannot mutate the caller's state.

---

## 11. Full End-to-End Execution Trace

### Tracing: `add-service -t payments -a checkout-api --golden-path go-service-postgres`

```
1. main.go
   └─ cli.Execute()
        └─ signal.NotifyContext(ctx) [Traps SIGINT/SIGTERM]
        └─ rootCmd.ExecuteContext(ctx)

2. PersistentPreRunE
   ├─ outputRoot: Resolves to CWD (or --output-root)
   ├─ catalogRoot: Fetches ~/.scaffolder-cache/v1.2.0 via getter.Client(Ctx: ctx)
   ├─ LoadCatalog(catalogFS): Unmarshals catalog.yaml & validates schema
   └─ renderer: Instantiates Renderer with OSWriter (or DryRunWriter)

3. addServiceCmd.RunE
   ├─ templater.Resolve(spec, "go-service-postgres", cfg)
   │    ├─ Finds GoldenPath: Runtime="go", Capabilities=["postgres"]
   │    ├─ Clones Capabilities slice via slices.Clone
   │    └─ Sorts Capabilities: ["postgres"]
   │
   └─ renderer.RenderService(ctx, resolvedCfg)
        ├─ 1. Runtime Block:
        │     walkAndRender("per-service/apps/runtimes/go" → "payments/apps/checkout-api")
        │     ├── go.mod.tmpl   → payments/apps/checkout-api/go.mod
        │     └── main.go.tmpl  → payments/apps/checkout-api/main.go
        │
        ├─ 2. Service Meta:
        │     walkAndRender("per-service/apps/service-meta" → "payments/apps/checkout-api")
        │     └── catalog-info.yaml.tmpl → payments/apps/checkout-api/catalog-info.yaml
        │
        ├─ 3. Delivery (Release):
        │     walkAndRender("per-service/gitops/release" → "payments/gitops/apps/checkout-api/dev")
        │     └── values.yaml.tmpl → payments/gitops/apps/checkout-api/dev/values.yaml
        │
        └─ 4. Capability Claims Dispatch:
              - If provisioner == "terraform":
                processSingleTemplate("per-service/infra/capabilities/postgres.tf.tmpl"
                                      → "payments/infra/apps/checkout-api/dev/postgres.tf")
              - If provisioner == "ack":
                processSingleTemplate("per-service/gitops/capabilities/s3.yaml.tmpl"
                                      → "payments/gitops/apps/checkout-api/dev/s3.yaml")
```

---

## 12. Architectural Defect Resolution & Answer Key

Every major bug from the initial prototype has been resolved and verified with automated tests:

| Defect ID | Original Bug | Architectural Solution | Automated Test |
|---|---|---|---|
| **Bug 1** | `--output-root` ignored; wrote to relative CWD | `resolveDestination` prefixes `OutputDir`; defaults to `os.Getwd()` | `TestRenderServiceGolden` |
| **Bug 2** | `--runtime` ignored if `--golden-path` given | `Resolve` checks `out.Runtime == ""` before applying golden-path runtime | `TestResolve/explicit_runtime_overrides_the_golden_path` |
| **Bug 3** | Slice header shared catalog backing array | `slices.Clone(gp.Capabilities)` inside `Resolve` | `TestResolveDoesNotMutateInput` |
| **Bug 4** | Capability order randomized by map iteration | `slices.Sort(deduped)` inside `Resolve` | `TestResolve/capabilities_are_sorted_deterministically` |
| **Bug 5** | Environment `{env}` hardcoded to `"dev"` | `Config.Env` parameter threaded through `resolveDestination` | `TestRenderServiceGolden` |
| **Bug 6** | Unchecked error from `fs.ReadFile` | Error check with `%w` wrapping in `LoadCatalog` and `processSingleTemplate` | `TestLoadCatalog` |
| **Bug 7** | `os.Create` truncated files before template execution | `bytes.Buffer` execution before calling `Writer.WriteFile` | `TestRenderService_BadRuntimeErrorPath` |
| **Bug 8** | Usage dumped on runtime errors | `rootCmd.SilenceUsage = true` | CLI integration |
| **Bug 9** | Overwrites destroyed developer hand-edits | Skip-if-exists check with `--force` override | `TestRenderService_SkipIfExists` |
| **Bug 10** | Uncancellable network/disk I/O | `context.Context` propagation through `getter.Client` and `fs.WalkDir` | `TestRenderService_ContextCanceled` |

---

## 13. Testing Architecture & Golden File Harness

The testing suite contains three testing tiers:

### 1. Table-Driven Resolution Tests (`resolve_test.go`)
Asserts all edge cases of config resolution without disk or network I/O:
```bash
go test ./internal/templater -run TestResolve -v
```

### 2. Golden-File Regression Testing (`render_test.go`)
Renders a complete service and compares every output file byte-for-byte against blessed golden files in `testdata/render_service/`:
```bash
# Verify against golden files
go test ./internal/templater -run TestRenderServiceGolden -v

# Update golden files when templates deliberately change
go test ./internal/templater -run TestRenderServiceGolden -update
```

### 3. Error Path & Cancellation Tests (`render_test.go`)
- `TestRenderService_BadRuntimeErrorPath`: Asserts non-nil error and **zero files written** on invalid runtime.
- `TestRenderService_DryRun`: Asserts `DryRunWriter` writes **zero files** to disk.
- `TestRenderService_SkipIfExists`: Asserts hand-edited files are preserved unless `Force: true`.
- `TestRenderService_ContextCanceled`: Asserts immediate cancellation and **zero files written** when context is canceled.

---

## 14. Suggested Reading Order

For engineers reading or contributing to this codebase:

1. **`internal/templater/errors.go`** — Sentinel errors and `ValidationError` struct.
2. **`internal/templater/writer.go`** — `Writer` interface and mock implementations.
3. **`internal/templater/resolve.go`** — Pure resolution logic, slice cloning, and sorting.
4. **`internal/catalog/catalog.go`** — Catalog YAML ingestion and schema validation.
5. **`internal/templater/render.go`** — In-memory rendering, context cancellation, and destination mapping.
6. **`cmd/cli/root.go`** — OS signal handling, version stamping, and remote catalog fetching.
7. **`cmd/cli/add_service.go` & `cmd/cli/onboard_team.go`** — Cobra command adapters.
8. **`internal/templater/render_test.go`** — Golden test harness and error assertions.
