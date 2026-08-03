# Code Walkthrough — the Go scaffolder, line by line

This describes the code **as it exists today** (branch `feature/go-cli`), not as it should be.
Every trace, path, and output block below was captured by actually building and running the binary
— nothing here is illustrative.

The command traced throughout:

```bash
scaffolder add-service --team-name payments --system-name checkout \
                       --app-name checkout-api --golden-path go-service-postgres
```

If you only read one thing, read **§10 (pointers and values)** and **§12 (bugs found while writing this)**.

---

## Contents

| § | Section |
|---|---|
| 1 | The 30-second mental model |
| 2 | Program start — `main.go` and Cobra's real execution order |
| 3 | `PersistentPreRunE` — the boot sequence |
| 4 | `catalog.LoadCatalog` — YAML bytes → Go structs |
| 5 | The command layer — `onboard-team`, `create-system`, `add-service` |
| 6 | `resolveDestination` — the output contract |
| 7 | `walkAndRender` — the closure and the walk |
| 8 | `processSingleTemplate` — bytes → file |
| 9 | `renderPath` — templating the *path*, not the content |
| 10 | Pointers and values — why `*Catalog` but `cfg Config` |
| 11 | Full end-to-end trace, all three verbs |
| 12 | Bugs found while writing this walkthrough |

---

## 1. The 30-second mental model

The scaffolder does exactly one thing:

> **Copy a directory tree, running every file through `text/template` on the way, into a path computed from a lookup table.**

That's it. Everything else is plumbing. Three packages:

```
main.go                       →  calls cli.Execute()
cmd/cli/*.go                  →  parses flags, decides WHICH tree to copy
internal/catalog/catalog.go   →  reads catalog.yaml → structs (the "what we offer" data)
internal/templater/render.go  →  does the copying + templating (the engine)
```

Three inputs meet in the middle:

| Input | Comes from | Example |
|---|---|---|
| **Source tree** | `1-platform-catalog/` (downloaded from GitHub) | `building-blocks/runtimes/go/` |
| **Data** | CLI flags + golden path from `catalog.yaml` | `TeamName: payments, Runtime: go` |
| **Destination** | `destinations:` table in `catalog.yaml` | `{team}-services/{system}/{app}/` |

Source tree + data → rendered bytes. Destination table → where they land.
The reason there's a `destinations:` table at all is so that **no Go file contains a hardcoded output
path** — you can restructure `3-tenant-workloads/` by editing YAML.

---

## 2. Program start — `main.go` and Cobra's real execution order

### `main.go` (7 lines)

```go
package main

import "scaffolder/cmd/cli"

func main() {
	cli.Execute()
}
```

**In:** nothing. **Out:** nothing. It exists only so the package `main` requirement is satisfied and
all real code lives in importable packages.

### The part that trips everyone up: `init()` runs before `main()`

There are four `init()` functions — one per file in `cmd/cli/`. Go runs **all** of them, in file-name
order within the package, **before** `main()` is entered. Nobody calls them.

```
Go runtime starts
  ├─ cmd/cli/add_service.go   init()  → rootCmd.AddCommand(addServiceCmd);   defines its flags
  ├─ cmd/cli/create_system.go init()  → rootCmd.AddCommand(createSystemCmd); defines its flags
  ├─ cmd/cli/onboard_team.go  init()  → rootCmd.AddCommand(onboardTeamCmd);  defines its flags
  ├─ cmd/cli/root.go          init()  → rootCmd.PersistentFlags() (--output-root, --dry-run)
  └─ main()  →  cli.Execute()  →  rootCmd.Execute()
```

So by the time `main()` runs, the command tree is **already fully built**. This is why you never see a
"register commands" function — registration is a side effect of the package loading.

`rootCmd.AddCommand(addServiceCmd)` is what makes `scaffolder add-service` a valid invocation.
Remove that one line and the subcommand vanishes from `--help`.

### `Execute()`

```go
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
```

**In:** `os.Args` (read by Cobra internally). **Out:** process exit code.

`rootCmd.Execute()` does four things in order:

1. Matches `os.Args[1]` (`"add-service"`) against registered subcommands.
2. Parses the remaining args into the flag variables — **this is the moment `cfg.TeamName` gets the
   string `"payments"`**.
3. Checks `MarkFlagRequired` constraints.
4. Runs `PersistentPreRunE` (root's, then any on the subcommand), then `RunE`.

Note Cobra prints the error itself before returning it, so `Execute()` discards the message and just
sets the exit code.

---

## 3. `PersistentPreRunE` — the boot sequence (`root.go:36-72`)

"Persistent" = inherited by every subcommand. This runs for `add-service`, `create-system`, and
`onboard-team` alike. It's the constructor for the whole program.

**In:** the parsed flags (`outputRoot`). **Out:** package-level `renderer` is non-nil, or an error.

### Step 3.1 — resolve the output root

```go
if outputRoot == "" {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	outputRoot = wd
}
```

| In | Out |
|---|---|
| `--output-root` not passed, CWD = `/tmp/run` | `outputRoot = "/tmp/run"` |

> ⚠️ **This value is never read again anywhere in the program.** See §12, bug 1.

### Step 3.2 — download the catalog

```go
s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
s.Suffix = " Fetching templates from GitHub..."
s.Start()
catalogPath, err := fetchRemoteCatalog("feature/go-cli")
```

`fetchRemoteCatalog` (`root.go:94`) hands a `git::` URL to HashiCorp's `go-getter`, which clones the
repo into a cache dir and extracts the `//1-platform-catalog` subdirectory:

```go
cacheDir := filepath.Join(home, ".scaffolder-cache", version)
url := "git::https://github.com/ok-karthik/....git//1-platform-catalog?ref=" + version
err := getter.Get(cacheDir, url)
return cacheDir, nil
```

| In | Out |
|---|---|
| `version = "feature/go-cli"` | `~/.scaffolder-cache/feature/go-cli` populated with the catalog; returns that path |

Real observed side effect — because `version` contains a `/`, `filepath.Join` creates a **nested**
directory, not a flat one:

```
~/.scaffolder-cache/
└── feature/
    └── go-cli/          ← the catalog lands here
        ├── catalog.yaml
        ├── blueprints/
        └── building-blocks/
```

This is the single most important architectural fact about the current design:

> **The CLI reads templates from GitHub, not from your working copy.**
> Editing `1-platform-catalog/` locally changes *nothing* until you commit and push.
> `go-getter` also caches — a second run may reuse the old clone.

That's a legitimate production choice (the platform team ships a versioned catalog; developers
consume it), but it makes local iteration painful and makes tests impossible without network. A
`--catalog-root` flag that overrides this with `os.DirFS(localPath)` is what unblocks both.

Real output of the whole step:

```
✅ Templates fetched!
```

### Step 3.3 — parse the catalog

```go
spec, err := catalog.LoadCatalog(filepath.Join(catalogPath, "catalog.yaml"))
```

| In | Out |
|---|---|
| `~/.scaffolder-cache/feature/go-cli/catalog.yaml` | `spec *catalog.Catalog` — see §4 |

### Step 3.4 — build the renderer

```go
renderer = &templater.Renderer{
	CatalogFS: os.DirFS(catalogPath),
	Spec:      spec,
}
```

`os.DirFS(catalogPath)` returns an `fs.FS` — a **read-only filesystem rooted at that directory**.
Once you hold it, `"building-blocks/runtimes/go"` is a complete, valid path. There is no `..`, no
leading `/`, no absolute path. Everything is relative to the root you handed to `DirFS`.

Why this matters: `fs.FS` is an *interface*. `os.DirFS` is one implementation (real disk). Another is
`fstest.MapFS` (an in-memory map). Another is `//go:embed`. Because `Renderer` holds the interface
rather than a directory path string, you can swap in a fake filesystem in a test without touching
disk or network. **That single field is the entire testability seam of this program.**

`renderer` is a package-level variable — that's how `RunE` in the other three files reaches it.

---

## 4. `catalog.LoadCatalog` — YAML bytes → Go structs

```go
func LoadCatalog(filePath string) (*Catalog, error) {
	data, err := os.ReadFile(filePath)      // []byte of the whole file
	if err != nil {
		return nil, err
	}
	var catalog Catalog                     // zero-valued struct: all strings "", all maps nil
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}
```

**In:** a file path. **Out:** `*Catalog`, or an error.

`yaml.Unmarshal(data, &catalog)` needs `&catalog` (a pointer) because it *writes into* the struct.
Passing `catalog` by value would hand it a copy, it would fill the copy, and the copy would be
discarded. This is the most common Go bug in decoding code — see §10.

### The mapping, concretely

`catalog.yaml` on the left, the struct field it lands in on the right:

```yaml
capabilities_source_base: "git::https://...cloud-services-terraform-modules"
#  └─ tag `yaml:"capabilities_source_base"`  →  Catalog.CapabilitiesSourceBase  (string)

golden-paths:                       #  →  Catalog.GoldenPaths  ([]GoldenPath — a LIST)
  - name: go-service-postgres       #     GoldenPaths[0].Name
    runtime: go                     #     GoldenPaths[0].Runtime
    capabilities: [postgres]        #     GoldenPaths[0].Capabilities  ([]string)
    delivery: standard-helm         #     GoldenPaths[0].Delivery

capabilities:                       #  →  Catalog.Capabilities  (map[string]Capability — a MAP)
  postgres:                         #     key "postgres"
    module: aws-postgres            #     .Module
    version: v1.0.2                 #     .Version

destinations:                       #  →  Catalog.Destinations  (map[string]string)
  runtimes: "{team}-services/{system}/{app}/"
```

Why is `golden-paths` a **list** but `capabilities` a **map**? Because golden paths are *iterated*
("show me every paved road") and capabilities are *looked up by name* ("what module is `postgres`?").
Pick the data structure that matches the access pattern; the YAML shape follows from that, not the
other way round.

The `yaml:"..."` struct tags are string metadata read at runtime via reflection. Without the tag,
`gopkg.in/yaml.v3` lowercases the field name — so `Runtime` would match `runtime` by accident but
`CapabilitiesSourceBase` would never match `capabilities_source_base`. Tag everything; don't rely on
the fallback.

### `FindGoldenPath` — and why the loop is written that way

```go
func (c *Catalog) FindGoldenPath(name string) (*GoldenPath, bool) {
	for i := range c.GoldenPaths {
		if c.GoldenPaths[i].Name == name {
			return &c.GoldenPaths[i], true
		}
	}
	return nil, false
}
```

| In | Out |
|---|---|
| `"go-service-postgres"` | `&GoldenPath{Name:"go-service-postgres", Runtime:"go", Capabilities:["postgres"]}, true` |
| `"nonexistent"` | `nil, false` |

The `for i := range` form, rather than `for _, gp := range`, is deliberate: `&c.GoldenPaths[i]` points
*into the catalog's own slice*, so the caller sees the real element. `&gp` would point at the loop's
temporary copy.

The `(value, bool)` return is the idiomatic Go "found?" signature — same shape as `m[k]` on a map.
Callers write `if gp, found := ...; !found { ... }`.

### `validate()` — dead code, and wrong

```go
func (c *Catalog) validate() error { ... }
```

Lowercase `v` = unexported, so only this package could call it — and **nothing does**. `LoadCatalog`
returns without calling it. Go does not warn about unused methods (only unused *locals* and
*imports*), so it compiles silently. See §12, bug 2 — if you wire it up today it fails immediately,
because its `requiredDestinations` list doesn't match the keys in `catalog.yaml`.

---

## 5. The command layer

### 5a. `onboard-team` — the simplest one

```go
RunE: func(cmd *cobra.Command, args []string) error {
	fmt.Printf("Onboarding new team: %s\n", cfg.TeamName)
	return renderer.RenderTenantFoundation(cfg)
},
```

`cfg` was already populated by Cobra during flag parsing, because `init()` bound the flag directly to
the struct field:

```go
onboardTeamCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "...")
//                                 ^^^^ address of the field — Cobra writes through this pointer
```

`StringVarP` takes `*string`. Cobra stores that pointer and, when it parses `--team-name payments`,
does `*ptr = "payments"`. That's why `RunE` can just read `cfg.TeamName` with no wiring in between.

`MarkFlagRequired("team-name")` makes Cobra error out before `RunE` if it's absent.

**Real output:**

```
✅ Templates fetched!
Onboarding new team: payments
blueprints/team/gitops/CODEOWNERS.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/team/CODEOWNERS
blueprints/team/gitops/appproject.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/team/appproject.yaml
blueprints/team/gitops/namespace.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/team/namespace.yaml
blueprints/team/gitops/networkpolicy.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/team/networkpolicy.yaml
blueprints/team/gitops/policy-exceptions.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/team/policy-exceptions.yaml
blueprints/team/infra/providers.tf.tmpl --> ../../3-tenant-workloads/payments-infra/platform/providers.tf
blueprints/team/infra/team-iam.tf.tmpl --> ../../3-tenant-workloads/payments-infra/platform/team-iam.tf
```

### 5b. `create-system`

Same shape, two flags, one tree. **Real output:**

```
Creating system checkout for team payments
blueprints/system/gitops/applicationset.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/platform/systems/checkout/applicationset.yaml
```

### 5c. `add-service` — the only one with real logic

This is the "seed and override" pattern: a golden path *seeds* defaults, explicit flags *override*
them.

**Step 1 — seed from the golden path** (`add_service.go:22-31`)

```go
if goldenPathFlag != "" {
	gp, found := renderer.Spec.FindGoldenPath(goldenPathFlag)
	if !found {
		return fmt.Errorf("golden path '%s' not found in catalog", goldenPathFlag)
	}
	cfg.Runtime = gp.Runtime
	cfg.Capabilities = gp.Capabilities
}
```

| In | Out |
|---|---|
| `--golden-path go-service-postgres` | `cfg.Runtime = "go"`, `cfg.Capabilities = ["postgres"]` |

> ⚠️ Two problems here, both real: the assignment is unconditional so it **clobbers** an explicit
> `--runtime`, and `cfg.Capabilities = gp.Capabilities` shares the catalog's slice backing array
> rather than copying it. §12, bugs 3 and 4.

**Step 2 — require a runtime**

```go
if cfg.Runtime == "" {
	return fmt.Errorf("a valid runtime or --golden-path must be specified (...)")
}
```

**Step 3 — merge extra capabilities** (`add_service.go:39-61`)

```go
extraCaps := strings.Split(capabilitiesString, ",")   // "postgres,s3" → ["postgres","s3"]
capMap := make(map[string]bool)
for _, cap := range cfg.Capabilities { capMap[cap] = true }
for _, cap := range extraCaps        { capMap[cap] = true }
cfg.Capabilities = []string{}
for cap := range capMap { cfg.Capabilities = append(cfg.Capabilities, cap) }
```

A `map[string]bool` used as a set — the standard Go idiom, since Go has no `set` type. Writing a key
twice is a no-op, which is the dedup.

| In | Out |
|---|---|
| `cfg.Capabilities=["postgres"]`, `--capabilities s3,postgres` | `["postgres","s3"]` **in random order** |

> ⚠️ `for cap := range capMap` iterates in a **deliberately randomized** order — Go shuffles map
> iteration on purpose to stop you depending on it. Harmless today (each capability writes its own
> file) but it will make golden tests fail intermittently. §12, bug 5.

**Real output:**

```
Generating app 'checkout-api' [Runtime: go, Capabilities: [postgres]]
building-blocks/runtimes/go/go.mod.tmpl --> ../../3-tenant-workloads/payments-services/checkout/checkout-api/go.mod
building-blocks/runtimes/go/main.go.tmpl --> ../../3-tenant-workloads/payments-services/checkout/checkout-api/main.go
building-blocks/delivery/release/values.yaml.tmpl --> ../../3-tenant-workloads/payments-gitops/systems/checkout/checkout-api/dev/values.yaml
Adding infrastructure capability: postgres
building-blocks/capabilities/postgres.tf.tmpl --> ../../3-tenant-workloads/payments-infra/checkout/checkout-api/dev/postgres.tf
```

---

## 6. `resolveDestination` — the output contract (`render.go:119-129`)

```go
func resolveDestination(destTemplate string, cfg Config) string {
	dest := destTemplate
	dest = strings.ReplaceAll(dest, "{team}",   cfg.TeamName)
	dest = strings.ReplaceAll(dest, "{system}", cfg.SystemName)
	dest = strings.ReplaceAll(dest, "{app}",    cfg.AppName)
	dest = strings.ReplaceAll(dest, "{env}",    "dev")
	return filepath.Join("../../3-tenant-workloads", dest)
}
```

**In:** a template string from `catalog.yaml`, plus the config. **Out:** a filesystem path.

| `destTemplate` | Result for `payments/checkout/checkout-api` |
|---|---|
| `{team}-gitops/platform/team/` | `../../3-tenant-workloads/payments-gitops/platform/team` |
| `{team}-services/{system}/{app}/` | `../../3-tenant-workloads/payments-services/checkout/checkout-api` |
| `{team}-infra/{system}/{app}/{env}/` | `../../3-tenant-workloads/payments-infra/checkout/checkout-api/dev` |

Note `{...}` here is **not** `text/template` — it's plain string replacement. Deliberate: the
destinations table is authored by platform engineers editing YAML, and `{team}` is friendlier than
`[[ .TeamName ]]`. Two different substitution systems in one program is a bit confusing, but the
audiences are different.

Two things this function gets wrong today, both traced live:

- `../../3-tenant-workloads` is relative to **the process's CWD**, not to the repo. Running from
  `/tmp/run` writes to `/tmp/3-tenant-workloads`. Confirmed by experiment. §12, bug 1.
- `{env}` is hardcoded to `"dev"`, so the dev→prod promotion model can't be expressed. §12, bug 6.

---

## 7. `walkAndRender` — the closure and the walk (`render.go:38-74`)

This is the function you said feels complex. It's 15 real lines; the density is the closure.

```go
func (r *Renderer) walkAndRender(sourceDir string, targetDir string, cfg Config) error {
	handleFile := func(srcPath string, d fs.DirEntry, err error) error {
		...
	}
	return fs.WalkDir(r.CatalogFS, sourceDir, handleFile)
}
```

### What a closure is, concretely

`handleFile` is a function **value** assigned to a variable. Because it is *declared inside*
`walkAndRender`, it can read `sourceDir`, `targetDir`, `cfg`, and `r` — those are "captured".

That matters because `fs.WalkDir`'s signature is fixed:

```go
func WalkDir(fsys fs.FS, root string, fn fs.WalkDirFunc) error
type WalkDirFunc func(path string, d fs.DirEntry, err error) error
```

`WalkDir` will only ever hand your function three arguments. There is no `cfg` parameter and no way
to add one. The closure is how `cfg` gets in. Without it you'd need a struct with a method and
fields, which is strictly more code for the same effect.

### The body, step by step

Input for this trace: `sourceDir = "building-blocks/runtimes/go"`,
`targetDir = "../../3-tenant-workloads/payments-services/checkout/checkout-api"`.

`fs.WalkDir` calls `handleFile` **once per entry, root included**, depth-first, in lexical order:

| Call | `srcPath` | `d.IsDir()` |
|---|---|---|
| 1 | `building-blocks/runtimes/go` | `true` |
| 2 | `building-blocks/runtimes/go/go.mod.tmpl` | `false` |
| 3 | `building-blocks/runtimes/go/main.go.tmpl` | `false` |

**Line 45 — the error parameter**

```go
if err != nil {
	return err
}
```

`WalkDir` passes a non-nil `err` when it couldn't read a directory. This *isn't* an error from your
previous call — it's an error about `srcPath` itself. Returning it aborts the walk. Returning `nil`
instead would mean "skip this and keep going".

**Lines 50-51 — relative path**

```go
relPath := strings.TrimPrefix(srcPath, sourceDir)
relPath = strings.TrimPrefix(relPath, "/")
```

| `srcPath` | after line 50 | after line 51 |
|---|---|---|
| `building-blocks/runtimes/go` | `""` | `""` |
| `building-blocks/runtimes/go/go.mod.tmpl` | `/go.mod.tmpl` | `go.mod.tmpl` |

Two `TrimPrefix` calls because the first leaves a leading slash on everything except the root.
(`filepath.Rel` would do this in one call, but it's the wrong package for `io/fs` paths — see §10.)

**Line 54 — render the path itself**

```go
renderedRelPath, err := renderPath(relPath, cfg)
```

Handles template variables *in directory and file names*, e.g. a template dir literally named
`[[ .AppName ]]`. No such directory exists in the catalog today, so for every current input this is
an identity function. It's here for the case where it isn't. See §9.

**Line 60 — join**

```go
targetPath := filepath.Join(targetDir, renderedRelPath)
```

| `renderedRelPath` | `targetPath` |
|---|---|
| `""` | `../../3-tenant-workloads/payments-services/checkout/checkout-api` |
| `go.mod.tmpl` | `../../3-tenant-workloads/payments-services/checkout/checkout-api/go.mod.tmpl` |

**Lines 63-68 — the fork**

```go
if d.IsDir() {
	return os.MkdirAll(targetPath, 0755)
}
return r.processSingleTemplate(srcPath, targetPath, cfg)
```

Directory → create it (`MkdirAll` is a no-op if it already exists, so no existence check needed).
File → render it. Because `WalkDir` is depth-first and visits a directory *before* its children,
every parent directory is guaranteed to exist by the time a file inside it is written. That's the
whole reason `processSingleTemplate` can call `os.Create` without a `MkdirAll` first.

---

## 8. `processSingleTemplate` — bytes → file (`render.go:76-103`)

```go
func (r *Renderer) processSingleTemplate(srcPath string, targetPath string, data any) error {
	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", targetPath, err)
	}
	defer outFile.Close()

	rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").Parse(string(rawBytes))
	if err != nil {
		return err
	}

	if err := tmpl.Execute(outFile, data); err != nil {
		return err
	}

	fmt.Println(srcPath, "-->", targetPath)
	return nil
}
```

| In | Out |
|---|---|
| `srcPath="building-blocks/runtimes/go/go.mod.tmpl"`, `targetPath=".../checkout-api/go.mod.tmpl"`, `data=Config{AppName:"checkout-api"}` | file `.../checkout-api/go.mod` containing `module checkout-api\n\ngo 1.21` |

Points worth understanding:

**`data any`** — not `data Config`. This is what lets the same function accept both a `Config` (for
runtime/blueprint templates) and a `CapabilityView` (for capability templates). `any` is an alias for
`interface{}`; `text/template` resolves `.AppName` by reflection at execute time, so it genuinely
doesn't care about the static type.

**`.TrimSuffix(targetPath, ".tmpl")`** — this is the only thing that strips the extension. Files
without `.tmpl` (like `python/Procfile` or the Helm chart's `deployment.yaml`) pass through unchanged
in name — but note they are *still parsed as templates*. That's fine because the delimiters are
`[[ ]]`, so Helm's `{{ }}` is untouched. It would break on any file containing a literal `[[`.

**`.Delims("[[", "]]")` must come before `.Parse()`.** It's chainable, so `.Parse(...).Delims(...)`
also compiles — and silently does nothing, leaving `[[ .AppName ]]` verbatim in your output. That
symptom always means the delimiters were set too late.

**`template.New(filepath.Base(srcPath))`** — the name is only a label used in error messages here,
since `Parse` + `Execute` on the same object never needs to look a template up by name. It *would*
matter with `ParseFiles`/`ParseFS`, which register under the basename and require the name to match.

**`defer outFile.Close()`** — runs when the function returns, on every path including the error
returns. This is Go's version of `finally`. Note it discards the error from `Close()`, which on a
buffered write can hide a disk-full failure.

**Two real defects in this function**: `err` from `fs.ReadFile` is never checked before being
overwritten on the next line, and `os.Create` runs *before* parsing so a bad template leaves a
zero-byte file behind. §12, bugs 7 and 8.

---

## 9. `renderPath` — templating the path (`render.go:106-116`)

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

Identical machinery to §8, with one difference: the output goes to a `strings.Builder` in memory
instead of a file. Both satisfy `io.Writer`, which is all `Execute` requires — the same reason you
can render to a file, a buffer, or straight to `os.Stdout` with no change to the template code.

`&buf` is passed by pointer because `Execute` appends to the builder; a copy would be written to and
thrown away.

| In | Out |
|---|---|
| `"go.mod.tmpl"` | `"go.mod.tmpl"` (no actions → identity) |
| `"[[ .AppName ]]/config.yaml"` | `"checkout-api/config.yaml"` |

---

## 10. Pointers and values — "pass by reference everywhere"

This is worth pinning down, because Go's rules here are simple but unusual.

**Go has no pass-by-reference. Everything is a copy.** A pointer is just a value that happens to be
an address; copying a pointer copies the address, so both copies reach the same object. That's the
entire model.

### Where this codebase uses a pointer, and why

| Code | Why |
|---|---|
| `yaml.Unmarshal(data, &catalog)` | The callee must **write into** your struct. Without `&`, it fills a copy that's thrown away. |
| `func (c *Catalog) validate()` | Pointer receiver — consistency with the rest of `*Catalog`'s methods, and avoids copying the struct (which holds three maps and a slice) on every call. |
| `renderer *templater.Renderer` | One shared instance built in `PersistentPreRunE` and read by three commands. A value would mean three copies. |
| `&cfg.TeamName` in `StringVarP` | Cobra stores the address and writes `"payments"` through it later. This is the only way flag binding can work. |
| `tmpl.Execute(&buf, cfg)` | `Execute` writes into the builder. |
| `return &c.GoldenPaths[i]` | Points into the real slice element, so callers see the catalog's actual data. |

### Where it deliberately uses a value

| Code | Why |
|---|---|
| `func (r *Renderer) RenderService(cfg Config)` | `Config` is four strings and a slice — cheap to copy — and the renderer has **no business mutating the caller's config**. Passing by value makes that guarantee structural. |
| `handleFile` capturing `cfg` | Same reason. Each template render sees the same immutable snapshot. |

**The rule of thumb**: pointer if the callee must modify it or if copying is expensive; value
otherwise. `Config` by value is the *right* call and reads as intentional once you see it that way.

### The one that will bite you: slices are half-copies

```go
cfg.Capabilities = gp.Capabilities   // add_service.go:30
```

A slice is a small header — `{pointer to array, length, capacity}`. Copying the header copies the
pointer, **not** the elements. So `cfg.Capabilities` and `gp.Capabilities` now share one backing
array. Writing `cfg.Capabilities[0] = "x"` would mutate the catalog's golden path in memory.

The current code happens to be safe because the merge branch reassigns to a brand-new `[]string{}`
rather than appending in place. That's luck, not design. The explicit version:

```go
cfg.Capabilities = append([]string(nil), gp.Capabilities...)   // copy the elements
```

Maps have the same property, permanently — there is no such thing as copying a map by assignment.

---

## 11. Full end-to-end trace

### `onboard-team --team-name payments`

```
main() → rootCmd.Execute()
  └─ PersistentPreRunE
       ├─ outputRoot = os.Getwd()                      (then never used)
       ├─ fetchRemoteCatalog("feature/go-cli")         → ~/.scaffolder-cache/feature/go-cli
       ├─ LoadCatalog(.../catalog.yaml)                → *Catalog
       └─ renderer = &Renderer{os.DirFS(...), spec}
  └─ RunE → RenderTenantFoundation(cfg)
       ├─ walkAndRender("blueprints/team/gitops",
       │                resolveDestination("{team}-gitops/platform/team/", cfg))
       │     → ../../3-tenant-workloads/payments-gitops/platform/team/
       │       {CODEOWNERS, appproject.yaml, namespace.yaml, networkpolicy.yaml, policy-exceptions.yaml}
       └─ walkAndRender("blueprints/team/infra",
                        resolveDestination("{team}-infra/platform/", cfg))
             → ../../3-tenant-workloads/payments-infra/platform/{providers.tf, team-iam.tf}
```

### `add-service ... --golden-path go-service-postgres`

```
RunE
  ├─ FindGoldenPath("go-service-postgres") → cfg.Runtime="go", cfg.Capabilities=["postgres"]
  └─ RenderService(cfg)
       ├─ 1. runtime      walkAndRender("building-blocks/runtimes/go",
       │                                ".../payments-services/checkout/checkout-api")
       │                    → go.mod, main.go
       ├─ 2. delivery     walkAndRender("building-blocks/delivery/release",
       │                                ".../payments-gitops/systems/checkout/checkout-api/dev")
       │                    → values.yaml
       └─ 3. capabilities MkdirAll(".../payments-infra/checkout/checkout-api/dev")
             for "postgres":
               spec = Spec.Capabilities["postgres"]        → {aws-postgres, v1.0.2}
               view = CapabilityView{cfg, "postgres", "aws-postgres", "v1.0.2", <sourceBase>}
               processSingleTemplate("building-blocks/capabilities/postgres.tf.tmpl",
                                     ".../dev/postgres.tf", view)
```

### The `CapabilityView` idea (`render.go:29-35`)

```go
type CapabilityView struct {
	Config                          // embedded
	Name                   string
	Module                 string
	Version                string
	CapabilitiesSourceBase string
}
```

**Embedding** `Config` (a type with no field name) promotes its fields, so `[[ .TeamName ]]` still
resolves inside a capability template even though `CapabilityView` has no `TeamName` field of its
own.

The point of this type: `postgres.tf.tmpl` receives data scoped to *itself*, so it writes

```
source = "[[ .CapabilitiesSourceBase ]]/[[ .Module ]]?ref=[[ .Version ]]"
```

instead of having to dig its own entry out of a map with `index .Capabilities "postgres"`. A template
that has to know its own name in the catalog is a smell — it means the same file can't be reused and
a rename in YAML silently breaks rendering.

**Verified output** of that line:

```hcl
module "postgres" {
  source = "git::https://github.com/ok-karthik/platform-engineering-idp-gitops-reference-architecture.git//4-platform-engineering/cloud-services-terraform-modules/aws-postgres?ref=v1.0.2"
  team_name = "payments"
  app_name  = "checkout-api"
}
```

---

## 12. Bugs found while writing this walkthrough

All confirmed by running the binary, not by reading.

### Bug 1 — `--output-root` is a dead flag; output goes outside your repo

`outputRoot` is assigned in `PersistentPreRunE` and read nowhere. `resolveDestination` hardcodes
`filepath.Join("../../3-tenant-workloads", dest)`, which resolves against the **process CWD**.

Reproduced: run from `/tmp/scratch/run` → files were written to `/tmp/scratch/../3-tenant-workloads`,
nowhere near the repo, with no warning.

*Fix:* thread `outputRoot` into `Renderer` and join against it. Combined with `--catalog-root`
(§3.2), this is Phase 1 of the plan.

### Bug 2 — `validate()` is never called, and would fail if it were

`catalog.go:36` requires these keys:

```
building-blocks/runtimes         building-blocks/service-meta
building-blocks/capabilities     building-blocks/delivery/release
```

`catalog.yaml:36` provides these:

```
runtimes    capabilities    delivery/release      (and no service-meta at all)
```

`render.go` looks up the *short* form, so the program works — but the moment you add
`if err := catalog.validate(); err != nil` to `LoadCatalog`, every run fails with
`missing destination key: building-blocks/runtimes`.

*Fix:* pick one convention. Recommend the **long** form (key = the literal source directory), because
then the renderer can derive the source path from the key instead of hardcoding both sides. Update
`catalog.yaml`'s four keys and `render.go`'s four lookups, add the `service-meta` entry (bug 9), then
call `validate()` from `LoadCatalog`.

### Bug 3 — `--runtime` is silently ignored when `--golden-path` is given

`add_service.go:29` assigns unconditionally, so the golden path overwrites the flag Cobra already
parsed.

Reproduced live:

```
$ scaffolder add-service --app-name py-api --golden-path go-service-postgres --runtime python
Generating app 'py-api' [Runtime: go, Capabilities: [postgres]]
building-blocks/runtimes/go/go.mod.tmpl --> .../py-api/go.mod
building-blocks/runtimes/go/main.go.tmpl --> .../py-api/main.go
```

Asked for Python, got Go, no warning. This inverts the documented "golden path seeds, flags override"
contract.

*Fix:* `if cfg.Runtime == "" { cfg.Runtime = gp.Runtime }`.

### Bug 4 — `cfg.Capabilities = gp.Capabilities` shares the catalog's backing array

See §10. Latent rather than live today, but one `append` away from mutating the loaded catalog.

### Bug 5 — capability order is randomized

`for cap := range capMap` — Go randomizes map iteration. Harmless now; guaranteed to make golden
tests flap. Sort before use: `sort.Strings(cfg.Capabilities)`.

### Bug 6 — `{env}` is hardcoded to `"dev"`

`render.go:124`. The dev→prod promotion model can't be expressed at all. Needs `Config.Env` with
`"dev"` as the flag default.

### Bug 7 — unchecked error from `fs.ReadFile`

```go
rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
tmpl, err := template.New(...).Parse(string(rawBytes))   // err from ReadFile overwritten here
if err != nil { ... }
```

If the read fails, `rawBytes` is `nil`, `string(nil)` is `""`, an empty template parses fine, and you
get a **silently empty output file**. Add the check between the two lines. (`go vet` won't catch
this; `errcheck` or `staticcheck` will.)

### Bug 8 — `os.Create` before `Parse` leaves zero-byte files on failure

`os.Create` truncates immediately. A template that fails to parse leaves an empty file behind. This is
exactly the case Plan-then-Write (Phase 2) removes: render everything into memory first, write only
once all renders succeeded.

### Bug 9 — `catalog-info.yaml.tmpl` is still orphaned

It sits at `building-blocks/runtimes/catalog-info.yaml.tmpl`, a **sibling** of `go/` and `python/`.
`RenderService` walks `building-blocks/runtimes/<runtime>/`, so the walk never reaches it.

Confirmed: `payments-services/checkout/checkout-api/` contains `go.mod` and `main.go` only. Every
service generated so far is missing its Backstage catalog entry.

*Fix:* move it to `building-blocks/service-meta/` with its own destinations entry, rather than
special-casing it in Go.

### Bug 10 — nothing renders the Helm chart

`building-blocks/delivery/chart/` contains `Chart.yaml.tmpl` and `values.yaml.tmpl`, and no code path
touches them. That directory is meant to be consumed by CI (`helm template` against the generated
`values.yaml`), which is the half of the GitOps loop that's still missing.

---

## Suggested reading order for the code itself

1. `main.go`, then all four `init()` functions — understand that the command tree is built before `main`.
2. `internal/catalog/catalog.go` top to bottom — it's the only pure-data file, no I/O beyond one `ReadFile`.
3. `cmd/cli/onboard_team.go` — 33 lines, the whole flag→render path with no branching.
4. `render.go:106` `renderPath` — the smallest complete use of `text/template`.
5. `render.go:76` `processSingleTemplate` — the same thing, writing to a file.
6. `render.go:38` `walkAndRender` — now the closure is the only new idea.
7. `cmd/cli/add_service.go` — the only file with real branching logic.
