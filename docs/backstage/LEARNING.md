# Backstage, explained for this repo

A plain-language guide. Read it top to bottom once; after that, use it as a lookup.
Written 2026-10-04 from a planning conversation. Decisions referenced here: ADR 0015
(Go-only scaffolder, no REST API yet) and ADR 0016 (Backstage dispatches to GitHub Actions).

---

## 1. Backstage in two minutes

Backstage is a website for developers inside a company. It has three parts that matter here:

| Part | What it is | What you write |
|---|---|---|
| **Software Catalog** | A list of every service, team and system, with links | A `catalog-info.yaml` per service (our scaffolder already writes it) |
| **Software Templates** | A form ("create a new service") that runs some steps | A `template.yaml` (ours: `docs/backstage/software-template.yaml`) |
| **Plugins** | Extra pages and extra template steps | Usually just install + config. Writing your own is where TypeScript comes in |

**What you actually need to know as a platform engineer is mostly YAML:**
`catalog-info.yaml`, `template.yaml`, and `app-config.yaml` (Backstage's own settings).
The big TypeScript project that `npx @backstage/create-app` generates is mostly boilerplate
you do not touch.

## 2. How a template works

A template has three sections:

```yaml
parameters:   # the FORM — fields the developer fills in
steps:        # what happens after "Create" — a list of ACTIONS, run in order
output:       # links shown at the end
```

An **action** is one step's code, e.g. `fetch:template` (copy files), `publish:github`
(create a repo), `github:actions:dispatch` (start a GitHub Actions workflow). Backstage ships
many **built-in** actions. A **custom** action is one your company writes in TypeScript.

## 3. Where does the real work happen? Five patterns

When a template "creates a service", something has to generate files and put them in git.
Teams do this in one of five ways. There is no survey with hard numbers — this is the
common picture:

| # | Pattern | How common | Who uses it |
|---|---|---|---|
| 1 | **Built-in actions only** — Backstage copies a skeleton (`fetch:template`) and creates a repo or PR (`publish:github`, `publish:github:pull-request`) | Most common | Most teams in their first year. Template files live inside Backstage. |
| 2 | **Dispatch to CI** — Backstage starts a pipeline (GitHub Actions, GitLab, Jenkins, Argo Workflows) that does the work | Very common | Teams creating infrastructure (Terraform) or running long jobs. Cloud credentials stay in CI. |
| 3 | **Custom action → internal API** — a thin TypeScript action calls a platform API over HTTP | Common in mature orgs | Big companies with a dedicated platform team (or a product like Port / Humanitec). |
| 4 | **Custom action → Kubernetes resource** — Backstage writes a Crossplane claim / custom resource (or a PR with one); a controller does the work | Growing | Teams using Crossplane or their own operators ("Kubernetes as the platform API"). |
| 5 | **Custom action runs a CLI inside Backstage** | Least common | Quick internal hacks. Avoided because the Backstage image must carry every tool, Backstage needs strong credentials, and slow commands block it. |

**Rule of thumb:** Backstage is the front door, not the engine. Quick file generation →
built-in actions. Anything with credentials or that takes time → hand it to CI or a
controller. Once a real platform API exists → thin custom action calls it.

## 4. `idp:run-cli` vs `github:actions:dispatch`

Both are template steps. The difference is **where our Go CLI runs**.

| | `idp:run-cli` (custom action, pattern 5) | `github:actions:dispatch` (built-in, pattern 2) |
|---|---|---|
| Where the CLI runs | Inside the Backstage server | In a GitHub Actions run |
| Who writes it | You, in TypeScript | Already exists; you write YAML |
| What Backstage needs | Our Go binary in its image, plus git push credentials | A GitHub token allowed to start workflows |
| What the user sees | Live logs in Backstage | Backstage says "started"; user follows a link to the run and the PR |
| Main downside | TypeScript; heavier, more privileged Backstage | Backstage does not wait for the run or show if it passed |

`github:actions:dispatch` is basically Backstage pressing GitHub's **"Run workflow"** button
for you, with the form values as the workflow's inputs.

`idp:run-cli` was never built — it is a placeholder name. **Until Phase 23 lands, our
`docs/backstage/software-template.yaml` still says `action: idp:run-cli`** (around line 60);
that is the custom action this section is talking about, and Phase 23 replaces it with
`github:actions:dispatch`.

## 5. What this repo uses, and why

**Pattern 2: `github:actions:dispatch`.** Plan: `docs/plans/2026-10-04-backstage-dispatch-workflow.md`.

```
Developer fills the Backstage form (tenant, app, golden path, capabilities, system)
        │  github:actions:dispatch
        ▼
.github/workflows/scaffold-service.yaml
        │  validate inputs → go run . add-service → commit on a branch → open a PR
        ▼
Pull request with the new service under 3-tenant-repos/<tenant>/...
        │  human review + merge
        ▼
tenant-repos-ci-cd.yaml builds the image, renders manifests → ArgoCD deploys
```

**What the workflow writes:** exactly what `make demo-add-service` writes — the service's
source and `catalog-info.yaml` in `workloads-repo/`, its Terraform claims in
`workloads-repo/infra/`, and its `values.yaml` (+ ACK claims) in `gitops-repo/` — but as a PR.

Why it fits here:
- This is one repo simulating many (ADR 0010), so one workflow in one repo covers every tenant.
- It reuses the exact command `make demo-add-service` and CI already run. No new Go.
- The new service arrives as a reviewed PR — the GitOps story this repo is about.
- No TypeScript. Backstage never needs Go installed or push rights.
- Not using Backstage's own templating (`fetch:template`), because that would bring back a
  second template engine — the problem ADR 0015 just removed.

## 6. Things that will surprise you (know them up front)

1. **Backstage says "done" when the workflow has only *started*.** The dispatch action does
   not wait or report the result (open issues [#29727](https://github.com/backstage/backstage/issues/29727),
   [#33104](https://github.com/backstage/backstage/issues/33104)). The template's output links
   point to the run and the PR.
2. **CI does not start on a PR that GitHub Actions opened.** GitHub blocks it so workflows
   can't trigger each other in a loop. Close and reopen the PR to run `ci.yaml`, or later use a
   GitHub App token.
3. **The repo setting "Allow GitHub Actions to create and approve pull requests" is off.**
   Turn it on (Settings → Actions → General) or the PR step fails.
4. **The Go CLI does not check names.** `--app-name ../../x` would write outside the tenant.
   The workflow validates inputs first; moving that check into the CLI is a follow-up.
5. **Never paste `${{ inputs.x }}` inside a `run:` script.** Pass it through `env:` and use
   `"$X"`. Otherwise someone can type shell code into the Backstage form (script injection).

## 7. Where Backstage lives (three different things)

| Part | Where | Why |
|---|---|---|
| Backstage **app code** (`dev-portal/`: Node, yarn, `packages/`) | Its own repo — as it is now in `~/github/backstage/dev-portal` | It is a separate application with its own build. Companies keep it separate. |
| Backstage **deployment** (Helm values, ArgoCD Application, ingress, Keycloak client) | Later: `4-platform-engineering/2-cluster-services/developer-portal/` | It is a cluster add-on like Grafana or Keycloak, using the image built from `dev-portal`. |
| Backstage **content** (templates, `catalog-info.yaml`) | This repo, next to what it describes | `catalog-info.yaml` is already written into every service by the scaffolder. |

Same split as Terraform in ADR 0012: this repo deploys and references other repos, it
doesn't hold their code. So your idea was half right: the *deployment* moves into
`4-platform-engineering/`, the *app code* does not.

## 8. Your `dev-portal` folder: what to look at, what to ignore

- **Look at:** `app-config.yaml`. That is where Backstage is told what to show
  (`catalog.locations`) and how to reach GitHub (`integrations.github`). It already points at
  this repo's `app-a/catalog-info.yaml` and `docs/backstage/software-template.yaml`.
- **Glance at:** `packages/backend/src/index.ts` — each `backend.add(import(...))` line is a
  plugin. `plugin-scaffolder-backend-module-github` is the one that provides
  `github:actions:dispatch`; it is already there.
- **Ignore for now:** everything else under `packages/` and `plugins/`, `node_modules/`, `yarn.lock`.
- **Before running it in a cluster:** change the absolute `/Users/...` paths in
  `app-config.yaml` to GitHub URLs, because a pod can't see your laptop.
- **Token:** `${GITHUB_TOKEN}` must be a fine-grained PAT with *Actions: read and write* on
  this repo for the dispatch to work.

## 9. Backstage or OpenChoreo?

Stay with Backstage. They are not really rivals: **OpenChoreo's portal is built on Backstage**,
and existing Backstage users can add it as plugins.

- **OpenChoreo** (CNCF Sandbox since January 2026, 1.0 in 2026, started by WSO2) is a complete,
  opinionated platform: scaffolding, tenancy, CI, GitOps and observability. It would replace
  most of what this repo builds by hand — which would hide the work this repo exists to show.
- **Backstage** (CNCF Incubating) is the market standard and the name in job ads.
- OpenChoreo is worth *reading* later: its controllers are written in Go — good material
  when you start learning operators.

## 10. Related decisions from the same conversation

- **Go only (ADR 0015).** The Python engine was deleted, not archived; Go moved up to
  `2-idp-scaffolder/`. Go is what platform teams expect for CLIs and operators.
- **No REST API yet (ADR 0015).** Developers, CI and Backstage all run the CLI. Add a Go API
  (`cmd/api/`, `net/http`) only when a second consumer can't run a binary — that is pattern 3
  in section 3. Pattern 4 (Crossplane / your own operator) is the natural next step after that,
  and good Go practice.

## 11. Reading list (short ones first)

1. Roadie — the dispatch action, one page with an example:
   https://roadie.io/backstage/scaffolder-actions/github-actions-dispatch/
2. CNCF blog — the same pattern with Terraform (Backstage form → GitHub Actions):
   https://www.cncf.io/blog/2024/01/29/creating-infra-using-backstage-templates-terraform-and-github-actions/
3. Backstage docs — writing templates (parameters / steps / output):
   https://backstage.io/docs/features/software-templates/writing-templates
4. Backstage docs — built-in actions list: https://backstage.io/docs/features/software-templates/builtin-actions/
5. Backstage docs — `catalog-info.yaml` fields: https://backstage.io/docs/features/software-catalog/descriptor-format
6. For contrast only — writing a custom action (the TypeScript you are skipping):
   https://backstage.io/docs/features/software-templates/writing-custom-actions
7. GitHub — why a bot-opened PR doesn't trigger CI ("triggering a workflow from a workflow"):
   https://docs.github.com/en/actions/using-workflows/triggering-a-workflow
8. GitHub — script injection and safe use of inputs:
   https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions
9. OpenChoreo: https://openchoreo.dev — and the InfoQ 1.0 summary:
   https://www.infoq.com/news/2026/04/openchoreo-10/

## 12. Glossary

| Word | Meaning |
|---|---|
| Entity | Anything in the catalog: a Component (service), System, Group, User, API, Template |
| Component | One service. Described by its `catalog-info.yaml` |
| System | A group of components that work together (our `--system` flag) |
| Template | A form + steps that create something |
| Action | One step's code inside a template |
| Location | A pointer in `app-config.yaml` telling Backstage where to read YAML files |
| `workflow_dispatch` | A GitHub Actions trigger meaning "start me by button or API, with these inputs" |

---

## Appendix — names for the plan → execute → review AI workflow

From the same conversation, for your CV/notes:
- **Agentic workflow** — the general term.
- Anthropic's "Building effective agents" names the two patterns yours combines:
  **orchestrator–workers** (Opus plans and hands out work) and **evaluator–optimizer**
  (Opus reviews, work gets fixed until it passes): https://www.anthropic.com/research/building-effective-agents
- **Planner–executor–critic** — common plain name for the loop itself.
- **Harness engineering** — the setup around the agents (agent definitions, skills,
  worktrees, "executors never push", verify steps on real output). Newer term.
- "Loop engineering" is not an established term.
- CV wording: *"multi-agent plan/execute/review workflow with model tiering (Opus plans and
  reviews, Sonnet executes)."*
