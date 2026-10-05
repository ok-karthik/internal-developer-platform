# PLAN.md — open work only

Finished phases, word for word, live in [`docs/plans/EXECUTION_LOG.md`](docs/plans/EXECUTION_LOG.md).
When a phase is done: move its section there and leave one line in "Recently done".

**How work is planned here:** Opus measures and writes a plan in `docs/plans/YYYY-MM-DD-<slug>.md`
→ the `implementer` agent (Sonnet) executes it in a git worktree, one commit per part →
the `reviewer` agent (Opus) checks it → the owner pushes and merges. Details, agent files and what the
reviews caught: [`docs/ai-workflow.md`](docs/ai-workflow.md). A plan lists measured
facts, exact scope, rules it could break, a verify sequence, and "done means".

---

## Open work

| Phase | What | State | Plan / details |
|---|---|---|---|
| **23** | Backstage → `github:actions:dispatch` → `scaffold-service.yaml` → Go CLI → PR | Built + reviewed — **PR #41 open**; after merge do the owner steps in the plan | [`2026-10-04-backstage-dispatch-workflow.md`](docs/plans/2026-10-04-backstage-dispatch-workflow.md) · background: [`docs/backstage/LEARNING.md`](docs/backstage/LEARNING.md) |
| **14** | Prove it runs: recording, fire an alert, walk a runbook | In progress since 2026-09-22 | below · full record in the log, "Phase 14" |
| **18** | Fleet / Karpenter: pieces that need the foundation repo | Waiting on `enterprise-aws-infrastructure` | below · log, "Phase 18" |
| **19** | Two small leftovers | Not started | below · log, "Phase 19" |

Order: 23 next (24b, done, made the CLI reject bad names before Backstage form input reaches it);
14 whenever you have a free evening with the laptop plugged in.

**Working rules:** everything happens in this checkout on `main` — no worktrees or files outside
the repo, no PRs for our own changes; one local commit per plan part.

---

## Phase 14 — what is left

Five real bugs were found and fixed by running `make setup` (details in the log). Still to do:

1. **Resume check:** `tenant-a-app-a-dev-local` was `OutOfSync`/`Degraded` on the pre-fix
   values when work stopped. Confirm it reaches `Synced`/`Healthy` with a running `app-a` pod.
2. **ArgoCD memory:** give `argocd-application-controller` a `resources:` block in
   `gitops-orchestration/values.yaml` (it was OOMKilled 14 times on a 6 GB VM), and write the
   minimum Docker/OrbStack memory into `local/README.md`.
3. **(a)** ~90-second `asciinema` recording: `make setup` → `onboard-tenant` → `add-service` →
   ArgoCD syncs → the app answers HTTP. Embed near the top of the README.
4. **(b)** Fire a Phase 4 burn-rate alert on purpose and capture it `firing` in Prometheus.
5. **(c)** Walk `docs/runbooks/app-a-availability-burn.md` (or the latency one) end to end and
   fix whatever it got wrong.
6. **(d)** Commit the artefacts to `docs/demo/` and link them from the README Quickstart.
7. Not investigated, probably cosmetic: `keycloak`/`sealed-secrets` sync `Unknown`;
   `argocd`/`metrics-server` `OutOfSync` with healthy resources.

## Phase 18 — what is left (blocked on the foundation repo)

Built and verified offline in this repo; none of it has run on a real cluster.
1. The SSM contract needs the Karpenter / cluster keys (node role, queue, endpoint, CA) —
   being added to `governance/discovery-publisher` in `enterprise-aws-infrastructure`.
2. `network/vpc` there must tag subnets `karpenter.sh/discovery`.
3. The ExternalSecret that builds the ArgoCD cluster Secret from SSM is **not written**. It needs
   a Parameter Store `ClusterSecretStore` and the hub→spoke auth is unverified. Do not put it under
   `2-cluster-services/` (it would break `make setup` on k3d).

## Phase 19 — what is left

1. 19.7 gate 6: `make setup` on k3d against the new `3-tenant-repos/*/gitops-repo/` paths
   (now on `main`, so this can run — overlaps with Phase 14 step 1).
2. A real `renovate` dry run (the regexes were only checked by hand).

---

## Follow-ups noticed, not yet planned

- Docs undersell the CLI: `--dry-run` works and re-runs skip existing files, but README and
  `.agents/AGENTS.md` items 11/13 say otherwise and point to a "Go TODO" that does not exist.
- `.agents/AGENTS.md` ADR links are written from the repo root, so they break on GitHub.
- `CODE_WALKTHROUGH.md:8` still shows `--team-name payments`.
- Backstage `owner: team-<tenant>` is not a Group that exists; needs Group entities.
- ~30 code comments, templates and fixtures say "PLAN.md Phase 14/18.x" — that text now lives in
  `docs/plans/EXECUTION_LOG.md`. Reword them to "Phase 18.x (docs/plans/EXECUTION_LOG.md)"
  in one pass that regenerates fixtures and golden files together.
- Backstage deployment into the cluster (`4-platform-engineering/2-cluster-services/developer-portal/`)
  — see `docs/backstage/LEARNING.md` section 7.

- ArgoCD: `argocd.yaml` uses chart `targetRevision: '*'` (unpinned) and does not set
  `resourceTrackingMethod`; long Application names are only safe under annotation tracking. Pin both.
- What name the external `postgres` module builds from `team_name`/`app_name`/`env` (other repo).
- Catalog load checks that every runtime has a template folder, but not that every capability has
  its `.tf.tmpl`/`.yaml.tmpl`. A capability declared without one fails half-way through `add-service`,
  after the runtime files are written. Add the same `fs.Stat` check in `internal/catalog`.

- Backstage docs: `docs/backstage/app-config.fragment.yaml` and `README.md` point the catalog at
  `3-tenant-repos/*/apps/*/catalog-info.yaml`; the real path is `.../workloads-repo/services/<app>/`.
  README also cites a "PLAN.md Phase 8.0" that no longer exists.
- Backstage template: add `returnWorkflowRunDetails: true` so the output links to the exact run
  (supported by the installed scaffolder-backend-module-github 0.9.7). Good `dev-portal` exercise.
- AI workflow ideas (see `docs/ai-workflow.md` §6): a read-only `triage` agent for Phase 14's SLO
  alerts; a small Go MCP server exposing `add-service` / `onboard-tenant` to agents.

## Recently done

- **Phase 24b** (2026-10-04) — joined-name cap (63), runtime/capabilities checked before writing, owner required.
- **Phase 24** (2026-10-04) — the CLI validates tenant/app/env/system/owner names; unsafe names write nothing.
- **Phase 21** (2026-10-04) — rich Backstage metadata in catalog-info; app-a fixture regenerated.
- **Phase 22** (2026-10-04) — Go is the only scaffolder engine; Python deleted; ADR 0015. PR #39.
- **Phase 20** (2026-09-29) — local SLO loop only, central telemetry upstream; ADR 0014.
- **Phase 19** (2026-09-21) — tenant two-repo layout, naming sweep, module pins (two leftovers above).
