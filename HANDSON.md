# Backstage Hands-On Workshop (Phase 4)

You're doing this yourself to actually learn Backstage — not having an agent generate
the finished thing. This doc is written so you can work through it with a **cheap/fast
model** (Haiku, or Sonnet at low effort) doing the typing and verification, and only
reach for a frontier model (Sonnet at medium/high effort, or Opus) when a step says so.

**How to use this doc with an agent session:**
1. Paste the numbered step (or a whole milestone) to your assistant.
2. Use the "Model / effort" line at the top of each milestone to pick who runs it.
3. Use the "Verify" block yourself or hand it to the assistant literally — it's written
   as exact commands and exact expected output, so a cheap model can check it without
   needing to understand *why*.
4. If a verify step fails and the fix isn't obvious from the error message, that's when
   you escalate to a smarter model/effort — say so explicitly ("Milestone 3 step 2
   failed, here's the error, need higher effort").

This is Phase 4 of `PLAN.md` in this repo. Read that section again before starting —
it has the four deliverables this workshop builds toward:
1. A local Backstage instance (`Makefile` target).
2. Its catalog pointed at `3-tenant-workloads/*/apps/*/catalog-info.yaml`.
3. A Software Template wrapping the existing scaffolder CLI (never reimplement
   scaffolding logic inside Backstage).
4. ArgoCD + Kubernetes plugins on the component page.

---

## Backstage in one page (read this first)

Skip this if you already grasp the shape. Otherwise, five minutes here saves confusion
in every milestone below.

**The one-sentence version:** Backstage is a website (frontend) plus a small server
(backend) that reads a bunch of YAML files describing "things that exist" (services,
teams, APIs) and shows them in one browsable UI — plus a "Create" page that can run
scaffolding tools for you.

```
                         YOUR BROWSER
                              |
                              v
   +------------------------------------------------------+
   |                 BACKSTAGE FRONTEND                    |
   |   (React app — packages/app in Milestone 1)           |
   |                                                        |
   |   /catalog page    /create page    entity detail page |
   +------------------------------------------------------+
                              |  talks to over HTTP
                              v
   +------------------------------------------------------+
   |                 BACKSTAGE BACKEND                      |
   |   (Node server — packages/backend in Milestone 1)      |
   |                                                        |
   |   Catalog engine        Scaffolder engine     Plugins  |
   |   (reads YAML files,    (runs Templates,      (ArgoCD, |
   |    builds entities)      Milestone 3)          K8s...) |
   +------------------------------------------------------+
        |                        |                    |
        v                        v                    v
  catalog-info.yaml      your existing Go/Python   ArgoCD API,
  files in THIS repo     scaffolder CLI, invoked    Kubernetes
  (Milestone 2)          as a child process          API
                          (Milestone 3)               (Milestone 4)
```

**The key idea that makes the rest of this doc make sense:** Backstage itself does not
know what a "service" or a "team" is. It just knows how to read YAML files shaped like
`kind: Component` / `kind: Template` / `kind: System`, and how to run **actions** —
small functions — as steps inside a Template. Everything you build in this workshop is
either:
- **a YAML file** that describes something (a Component, a Template), or
- **a small piece of glue code** (a scaffolder action) that calls something that
  already exists — in your case, the CLI this repo already has.

That second point is the whole shape of Phase 4: **Backstage is a UI wrapped around
tools you already built**, not a replacement for them. Milestone 3 is the only place
you write real glue code; everything else is config.

```
   WITHOUT Backstage today:                WITH Backstage (what you're building):

   developer                                developer
      |                                         |
      | runs CLI by hand                        | fills a web form
      v                                         v
   go run . add-service ...                Backstage "Create" page
      |                                         |
      v                                         | (Milestone 3 action runs
   files land in                                |  the SAME CLI command
   3-tenant-workloads/                          |  under the hood)
                                                 v
                                            go run . add-service ...
                                                 |
                                                 v
                                            files land in
                                            3-tenant-workloads/
```

Further reading with the official (interactive) diagrams:
- [Architecture overview](https://backstage.io/docs/overview/architecture-overview/) — the frontend/backend split shown above, drawn properly
- [Technical overview](https://backstage.io/docs/overview/technical-overview/) — same idea, more prose

---

## Model / effort cheat-sheet

| Kind of work | Model | Effort | Why |
|---|---|---|---|
| Running a documented command, checking output matches | Haiku | low | Mechanical — no judgment calls |
| Editing a YAML/config file per exact instructions given here | Haiku or Sonnet | low | Still mechanical, but a syntax slip needs a model that at least knows YAML |
| Reading a Backstage error and mapping it to a likely cause | Sonnet | medium | Needs to correlate error text against docs/config, not just pattern-match |
| Writing the Software Template YAML + custom scaffolder action (Milestone 3) | Sonnet | medium–high | Real design surface: how the template step shells out to the existing CLI, what inputs it exposes, error handling |
| Anything where the fix isn't in this doc and isn't in the official docs linked below | Opus / frontier, high effort | Novel debugging, version-specific breakage, or a genuine design decision |

Default assumption for every milestone below unless stated otherwise: **Haiku, low
effort** for execution, **Sonnet, medium effort** if something breaks and the fix isn't
obvious from the error text.

---

## Milestone 0 — Node version

**Model / effort:** Haiku, low.

Backstage's `create-app` checks Node version against its supported range (historically
Active/Maintenance LTS only — check the exact range at the link below, it moves).
Your system Node was v26.5.0 when this doc was written, which is a *Current* release,
not an LTS — too new. You have no `nvm`/`fnm`/`volta` installed.

**Docs:** https://backstage.io/docs/getting-started/#prerequisites

**Steps (nvm path — recommended, lets you pin Node per-project):**
```bash
brew install nvm
mkdir -p ~/.nvm
# Add to ~/.zshrc (or ~/.bashrc):
#   export NVM_DIR="$HOME/.nvm"
#   [ -s "/opt/homebrew/opt/nvm/nvm.sh" ] && \. "/opt/homebrew/opt/nvm/nvm.sh"
source ~/.zshrc

nvm install 22
nvm use 22
```

**Steps (brew path — simpler, global):**
```bash
brew install node@22
brew link --overwrite --force node@22
```

**Verify:**
```bash
node -v   # must print v22.x.x (or whatever LTS the docs link currently names)
npm -v
```
Expect: a v22 (or current-LTS) version string, not v26.

---

## Milestone 1 — Bootstrap a bare Backstage app

**Model / effort:** Haiku, low. This is pure scaffolding — no design decisions.

**Concept (read before running):** Backstage is a monorepo app generated by its own
CLI. It produces `packages/app` (the React frontend) and `packages/backend` (the Node
backend, itself a plugin host). Almost everything you'll touch in later milestones is
either a config file (`app-config.yaml`) or a plugin package added to one of those two.

`create-app` isn't downloading "a Backstage" — it's generating **your own app** that
happens to be built entirely out of Backstage's libraries. That's why the directory
it produces looks like a normal JS monorepo, not a black-box binary:

```
idp-backstage/                  <- an app YOU own and can edit freely
├── app-config.yaml             <- THE file you'll edit most (Milestones 2-4)
├── packages/
│   ├── app/                    <- React frontend
│   │   └── src/                   (pages, plugin registration)
│   └── backend/                <- Node backend
│       └── src/                   (catalog engine, scaffolder engine,
│                                    plugin registration, and — in
│                                    Milestone 3 — YOUR custom action)
└── examples/                   <- sample catalog-info.yaml files Backstage
                                    ships so the default UI isn't empty
```

**Docs:** https://backstage.io/docs/getting-started/

**Steps:**
```bash
cd ~/github   # anywhere OUTSIDE internal-developer-platform — this is a separate app
npx @backstage/create-app@latest
```
It will prompt for an app name — use `idp-backstage`. This takes a few minutes
(installs the full dependency tree).

**Verify:**
```bash
cd idp-backstage
yarn start
```
Expect: it opens `localhost:3000` with the default Backstage UI (a sample catalog with
example components). Ctrl-C to stop once confirmed.

---

## Milestone 2 — Point the catalog at this repo

**Model / effort:** Sonnet, low–medium (editing YAML correctly matters; a wrong glob
just silently shows nothing, which is a bad debugging loop for a cheap model alone).

**Concept:** Backstage's catalog is populated from `locations:` in `app-config.yaml` —
each location is a URL or glob pattern to one or more `catalog-info.yaml` files. This
repo already emits valid `Component` entities:
`3-tenant-workloads/team-a/apps/app-a/catalog-info.yaml` — go read that file, it's
short.

This is the moment the two repos meet — a plain data file this repo's scaffolder has
been writing all along becomes a live UI entry, with zero code:

```
 internal-developer-platform/                    idp-backstage/
 3-tenant-workloads/team-a/apps/app-a/            app-config.yaml
   catalog-info.yaml  ------------------------->    catalog:
   (kind: Component, written by                        locations:
    `add-service` long before                            - type: file
    Backstage ever existed)                                target: .../apps/*/catalog-info.yaml
                                                              |
                                                              v
                                                   Backstage backend's catalog
                                                   processor reads the glob on
                                                   startup + polls for changes
                                                              |
                                                              v
                                                   localhost:3000/catalog
                                                   shows "app-a" as a real,
                                                   clickable entity
```

The important realization: you are not *building* anything here. You're pointing an
existing reader at data that already exists. If `app-a` doesn't show up, the bug is
almost always the glob path being wrong or relative when it needed to be absolute —
not a Backstage problem.

**Docs:**
- https://backstage.io/docs/features/software-catalog/
- https://backstage.io/docs/features/software-catalog/configuration (the `catalog.locations` shape)

**Steps:**
1. In `idp-backstage/app-config.yaml`, find the `catalog:` block.
2. Add a `locations:` entry of type `file` with a glob target pointing at this repo's
   `3-tenant-workloads/*/apps/*/catalog-info.yaml` (use the **absolute path** to your
   `internal-developer-platform` checkout — Backstage's `file` type doesn't walk
   relative to nothing).
3. Restart (`yarn start`).

**Verify:**
- Open `localhost:3000/catalog`.
- Expect: `app-a` appears as a Component, owned by `team-team-a`, system `checkout`.
- Click into it — the entity page should show the fields from `catalog-info.yaml`
  (type: service, lifecycle: production).

If nothing shows up: check the Backstage backend logs for a catalog processor error —
paste the exact error to escalate past Haiku.

---

## Milestone 3 — Software Template wrapping `add-service`

**Model / effort:** Sonnet, medium–high. This is the one milestone with real design
surface — read the "why" below before writing anything.

**Concept:** A Backstage **Software Template** is itself a catalog entity
(`kind: Template`) with a `spec.steps` list. Each step invokes a **scaffolder action**
(`action: <name>`) — built-in ones fetch files, run publish steps, etc. The critical
constraint from `PLAN.md`: *the template calls the existing CLI, it does not
reimplement scaffolding logic*. That means your template's key step should shell out to
`go run . add-service ...` (or the Python CLI), not use Backstage's built-in
`fetch:template` file-templating to duplicate what `1-platform-catalog/` already does.

Backstage doesn't ship a generic "run a shell command" action out of the box for
security reasons — you'll likely need to write a small **custom scaffolder action**
(a TypeScript function registered in `packages/backend`) that calls the CLI as a child
process. This is the part worth the frontier-model budget: getting the action's input
schema right (team name, app name, golden path, capabilities — mirroring the CLI flags
in `.agents/AGENTS.md`'s Execution & Verification Commands section) and handling a
non-zero exit from the CLI without swallowing the error.

Walking through what actually happens end-to-end, click to file-on-disk:

```
 1. Browser: localhost:3000/create
    You pick "IDP: add-service" and fill a form
    (team name, app name, golden path, capabilities)
                    |
                    v
 2. Backstage backend loads the Template entity YAML you wrote.
    Its spec.steps says: run action "idp:add-service" with the
    form values as input.
                    |
                    v
 3. Your custom action (TypeScript, packages/backend) receives
    those inputs as a typed object. It builds a command line —
    the SAME one you'd type by hand:

      go run . add-service \
        --catalog-root  <path to 1-platform-catalog> \
        --output-root   <path to 3-tenant-workloads>  \
        --team-name     <from form>  \
        --app-name      <from form>  \
        --golden-path   <from form>
                    |
                    v
 4. Your action spawns that as a child process (Node's
    `child_process.execFile` or similar), waits for it to exit,
    and checks the exit code — this is the CLI's own contract:
    "every error path exits 1 AND writes nothing" per AGENTS.md.
                    |
          exit 0 ---+--- exit 1
          |               |
          v               v
 5a. Action reports    5b. Action throws/reports failure;
     success back to        Backstage UI shows the CLI's
     the UI                 real stderr, not a generic 500
                    |
                    v
 6. Real files now exist under 3-tenant-workloads/<team>/ in
    THIS repo — check with `git status` there, same as if you'd
    run the CLI yourself.
```

Notice step 3-4 is the *entire* contribution of your custom code: translate a web
form into the exact CLI invocation a human would type, then relay its exit code
honestly. Nothing about *how* a service gets scaffolded lives in Backstage — that
stays in `1-platform-catalog/` where it already is. That division of labor is the
point of "the template calls the CLI, it does not reimplement it."

**Docs:**
- https://backstage.io/docs/features/software-templates/
- https://backstage.io/docs/features/software-templates/writing-templates
- https://backstage.io/docs/features/software-templates/writing-custom-actions

**Steps (do this as a conversation with Sonnet at medium+ effort, not a checklist —
there are real decisions here):**
1. Write a custom action `run-idp-scaffolder` (or similar) in `packages/backend` that
   shells out to the Go CLI with `--catalog-root` and `--output-root` pointed at your
   `internal-developer-platform` checkout (same pattern as `make demo-add-service` in
   this repo's `Makefile` — go read that target for the exact flags).
2. Register the action in the backend's scaffolder module.
3. Write a `Template` entity YAML with a form (team name, app name, golden path,
   capabilities) that calls your action.
4. Add that template as a `locations:` entry so it shows up in Backstage's "Create"
   page.

**Verify:**
- `localhost:3000/create` shows your template.
- Filling the form and submitting runs the real CLI and produces real files under
  `3-tenant-workloads/<team>/` in this repo (check with `git status` in
  `internal-developer-platform`).
- An invalid golden-path name in the form surfaces the CLI's actual error in the
  Backstage UI, not a generic failure.

---

## Milestone 4 — ArgoCD + Kubernetes plugins

**Model / effort:** Haiku/Sonnet, low for installing; Sonnet medium if auth wiring
(kubeconfig, ArgoCD token) breaks.

**Concept:** These are frontend+backend plugin pairs that read live state (ArgoCD sync
status, K8s pod health) and render it on a component's entity page, keyed off
annotations in `catalog-info.yaml` (e.g. an `argocd/app-name` annotation).

The annotation is the only "wiring" — it's how a generic plugin knows *which* live
resource corresponds to *this* catalog entity, out of everything running in your
cluster:

```
 catalog-info.yaml                    entity page for "app-a"
 metadata:                            +----------------------------+
   annotations:                       |  app-a                     |
     argocd/app-name: app-a  ---+     |  [ArgoCD card]              |
     backstage.io/               \    |    queries ArgoCD API for   |
       kubernetes-id: app-a       \   |    app named "app-a"  ----+ |
                                    \  |    shows: Synced/Healthy  | |
                                     \ |                            | |
                                      \|  [Kubernetes card]         | |
                                       |    queries K8s API for     | |
                                       |    resources labeled       | |
                                       |    app-a  --------------+  | |
                                       |    shows: pods, status   |  | |
                                       +--------------------------|--|-+
                                                                   v  v
                                                          ArgoCD API   K8s API
                                                          (your local  (your local
                                                           ArgoCD)      cluster)
```

Same pattern as Milestone 2: no new logic, just pointing an existing reader (the
plugin) at existing data (your annotations + your running cluster) via a shared key.

**Docs:**
- Kubernetes plugin (official): https://backstage.io/docs/features/kubernetes/
- ArgoCD plugin (community, Roadie): https://roadie.io/backstage/plugins/argo-cd/

**Steps:**
1. Install both plugin packages per their docs (`yarn add` into `packages/app` and
   `packages/backend` as each doc specifies — the exact package names/versions change,
   use the linked docs, not this doc, for those).
2. Add the required annotations to
   `1-platform-catalog/building-blocks/service-meta/catalog-info.yaml.tmpl` (the
   **template**, not just the rendered file — this is a platform-owned file per
   `.agents/AGENTS.md`) so every future service gets them automatically. Update
   `3-tenant-workloads/team-a/apps/app-a/catalog-info.yaml` (the rendered copy) to
   match, same as every other blueprint edit in this repo.
3. Point the K8s plugin at your local cluster's kubeconfig; point the ArgoCD plugin at
   your local ArgoCD instance (`make get-argocd-creds` in this repo gives you the
   admin password).

**Verify:**
- `app-a`'s entity page shows an ArgoCD sync-status card and a Kubernetes pods card,
  both reflecting real state from `make setup` if you have it running.

---

## Milestone 5 — Wire it into this repo's demo flow

**Model / effort:** Haiku, low.

**Steps:**
1. Add a `install-backstage` (or `run-backstage`) target to this repo's `Makefile`,
   alongside `install-argocd` — should just `cd` into wherever you put `idp-backstage`
   and run `yarn start` (or `yarn dev`), OR document that Backstage lives in a sibling
   directory and isn't part of this repo's own dependency tree (decide which, and say
   which you picked in the commit message).
2. Update `README.md`'s roadmap: flip the Backstage line from `- [ ]` (future) to done,
   and fix the wording — it currently says "migrating the python CLI into Backstage
   Software Templates," which is wrong per `PLAN.md`'s own callout: **Backstage calls
   the CLI, it does not replace it.**

**Verify:**
```bash
grep -n "Backstage" README.md
```
Expect: no more "migrating the CLI into Backstage" framing; a roadmap line that
reflects what you actually built (don't mark plugins done if you skipped Milestone 4).

---

## When you're done

Report honestly, same standard as the rest of `PLAN.md`: which milestones you finished,
which you skipped and why, and whether verification actually ran or you're taking your
own word for it.
