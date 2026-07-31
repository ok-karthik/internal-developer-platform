# Go Implementation Plan: Plan-then-Write Refactoring

Claude suggested a brilliant architectural refactor for the Scaffolder CLI: separating the "Planning" of the templates from the actual "Writing" to disk. 

This enables:
1. **Atomicity:** If a template fails halfway through, nothing is written to disk.
2. **Testing:** You can write Go tests entirely in memory without creating temp directories.
3. **Dry Runs:** `--dry-run` simply skips the final `WriteTo` call.

I have updated the templates in `1-platform-catalog` for you. Now, here is your step-by-step guide to implement the Go code.

---

## 1. Update the Catalog (`internal/catalog/catalog.go`)

Update your `Catalog` struct so we can parse the Terraform modules dynamically from the YAML.

```go
type Capability struct {
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
}

type Catalog struct {
	CapabilitiesSourceBase string                `yaml:"capabilities_source_base"`
	GoldenPaths            []GoldenPath          `yaml:"golden-paths"`
	Destinations           map[string]string     `yaml:"destinations"`
	Capabilities           map[string]Capability `yaml:"capabilities"`
}
```

---

## 2. Refactor the Templater (`internal/templater/render.go`)

This is the biggest change. We are replacing global `RenderX` functions with a `Renderer` struct that builds a `Plan`.

### Step 2a: Add the Plan and Renderer Structs
At the top of the file, add:

```go
// Renderer is responsible for reading templates from a filesystem (CatalogFS) and planning them.
type Renderer struct {
	CatalogFS fs.FS
	Spec      *catalog.Catalog
}

// Plan holds all rendered files in memory before they are written to disk.
type Plan struct {
	Files map[string][]byte // Destination Path -> File Content Bytes
}

// WriteTo atomically writes all planned files to the given root directory.
func (p *Plan) WriteTo(root string) error {
	for path, content := range p.Files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("failed to create dir for %s: %w", fullPath, err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
	}
	return nil
}
```

### Step 2b: Update Config struct
Change the `Capabilities` field from a `[]string` to a `map[string]catalog.Capability`. Also add `CapabilitiesSourceBase string`. This allows your Terraform templates to resolve the dynamic URLs!

```go
type Config struct {
	TeamName               string
	SystemName             string
	AppName                string
	Runtime                string
	CapabilitiesSourceBase string
	Capabilities           map[string]catalog.Capability
}
```

### Step 2c: Refactor `walkAndRender` to `walkAndPlan`
Change your existing `walkAndRender` function signature to:
`func (r *Renderer) walkAndPlan(sourceDir string, targetDir string, cfg Config, plan *Plan) error`

**Important inner changes to `walkAndPlan`:**
1. Remove the `d.Name() == "copier.yml"` check (it's dead weight).
2. Use `fs.ReadFile(r.CatalogFS, srcPath)` instead of `os.ReadFile` (Make sure you adjust the `srcPath` appropriately since `fs.FS` uses `/` and doesn't like relative `../` roots).
3. Instead of writing to disk (`os.Create`), store the result in the plan: `plan.Files[targetPath] = buf.Bytes()`.

### Step 2d: Convert Render Functions to Plan Methods
Convert `RenderService`, `RenderSystem`, and `RenderTenantFoundation` to methods on the `Renderer`:
`func (r *Renderer) PlanService(cfg Config) (*Plan, error)`

In `PlanService`, initialize a new plan:
```go
plan := &Plan{Files: make(map[string][]byte)}
```
Then replace your `walkAndRender` calls with `r.walkAndPlan(..., plan)`. 

*(Don't forget to explicitly render `catalog-info.yaml.tmpl`! It sits at `blueprints`... wait no, it's at `building-blocks/runtimes/catalog-info.yaml.tmpl`, so you will need to add a line to explicitly read it and add it to `plan.Files[targetPath]`).*

---

## 3. Update the CLI Commands (`cmd/cli/*.go`)

In `add_service.go`, `create_system.go`, and `onboard_team.go`, change how you trigger the scaffolding.

**For `add_service.go`:**
When building the `Config`, you now need to populate the capabilities map:
```go
cfg.Capabilities = make(map[string]catalog.Capability)
for _, capName := range capabilitiesList { // Your parsed list from flags
    if capData, ok := cat.Capabilities[capName]; ok {
        cfg.Capabilities[capName] = capData
    } else {
        return fmt.Errorf("unknown capability: %s", capName)
    }
}
```

Then, execute the scaffolding using the new Renderer:
```go
renderer := &templater.Renderer{
    CatalogFS: os.DirFS("../../1-platform-catalog"), // When running from golang/
    Spec:      cat,
}

plan, err := renderer.PlanService(cfg)
if err != nil {
    return fmt.Errorf("failed to plan service: %w", err)
}

// TODO: Add an if !dryRun check here later!
if err := plan.WriteTo("../../3-tenant-workloads"); err != nil {
    return fmt.Errorf("failed to write plan: %w", err)
}
fmt.Println("Service successfully generated!")
```

Do the same for `create_system.go` and `onboard_team.go` (calling their respective `Plan...` methods).
