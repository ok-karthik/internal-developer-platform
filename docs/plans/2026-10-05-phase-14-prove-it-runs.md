# Phase 14 — Prove it runs (a hands-on guide you run yourself)

**Who runs it:** the owner, by hand, on the laptop. Claude explains; nothing here is delegated.
**Planned by:** Opus, 2026-10-05. Takes about 1–2 hours the first time.
**Goal:** see the platform work for real on a local Kubernetes cluster — ArgoCD deploys
`app-a` and `app-b`, they answer HTTP, an alert reaches its receiver, and a runbook gets walked.

---

## What this proves — and what it does not

| Proves | Does NOT prove (and why) |
|---|---|
| The GitOps loop: git → ArgoCD → running pods | Anything on AWS: no real RDS, S3 or IAM is created (the ACK controllers have no AWS credentials locally) |
| Tenancy objects apply cleanly: namespace, quota, LimitRange, NetworkPolicies, RBAC, AppProject | Real load balancers, zones, node isolation — k3d is one Docker container |
| Ingress routes a hostname to the right pod | The real burn-rate **PromQL** firing from real traffic — see "The honest gap" below |
| Alertmanager routes a `page` alert to its receiver, and the runbook can be followed | |

**The honest gap.** The SLO alerts read `http_server_request_duration_seconds_count` for
`app-a`. The Go service template has **no OpenTelemetry SDK**, so `app-a` emits no such metric
(recorded as "Phase 20 finding 4" in `docs/plans/EXECUTION_LOG.md`). So in Step 6 we fire a
**synthetic** alert straight into Alertmanager (`make fire-synthetic-alert`). That proves routing,
receiver and runbook — not the PromQL. Making the real alert fire is a separate code change
(proposed as Phase 25 at the end).

## What was measured — do not re-derive (2026-10-05, `main` @ latest)

| Fact | Value |
|---|---|
| Cluster tool | k3d, cluster name `nexus-platform`, Traefik built-in disabled, ports 80/443 mapped (`Makefile` `CREATE_CLUSTER_CMD`) |
| Docker | **not running** right now (OrbStack socket unreachable) |
| `make setup` chain | `create-cluster install-argocd bootstrap wait-for-apps configure-aws get-argocd-creds` |
| `configure-aws` | needs `./aws-creds.ini`, which was deleted in Phase 16 → **skip this target** (not needed locally) |
| `wait-for-apps` | waits until **every** app is Synced+Healthy; ACK controllers are always `Degraded` locally → it times out after ~8 min → **skip it**, watch by hand instead |
| Last run (2026-09-22) | ArgoCD's `argocd-application-controller` was OOMKilled 14× on a 6 GB VM; fixed by giving OrbStack 10 GB. No `resources:` for it in `gitops-orchestration/values.yaml` |
| ArgoCD reads config from | GitHub `HEAD` of this repo — local uncommitted edits are NOT deployed. Push first |
| App images | `ghcr.io/ok-karthik/internal-developer-platform/tenant-a/app-a` and `/app-b`, tag `latest` — both publicly pullable (checked anonymously: HTTP 200) |
| App hosts | Ingress `app-a-dev.local`, `app-b-dev.local` (class `traefik`) |
| Platform UIs | `http://argocd.localhost`, `https://grafana.localhost` (`*.localhost` resolves to your machine automatically) |
| Cluster registration | `local/cluster-secret.yaml` → cluster labelled `environment: dev`, so only `dev/` apps deploy |
| App Application names | `tenant-a-<app>-dev-<cluster>`; last run showed `tenant-a-app-a-dev-local` |
| Alert path | `make fire-synthetic-alert` → Alertmanager (`monitoring/alertmanager-operated:9093`) → `page-receiver` → pod `alert-webhook-receiver` logs |
| Runbooks | `docs/runbooks/app-a-availability-burn.md`, `app-a-latency-burn.md` |
| `asciinema` | installed on 2026-09-22 |

---

## Where things live (so you know what you are looking at)

| Thing | File / place |
|---|---|
| How the cluster is created | `Makefile` → `CREATE_CLUSTER_CMD` |
| How ArgoCD is installed | `Makefile` → `install-argocd` (Helm) + `4-platform-engineering/2-cluster-services/gitops-orchestration/values.yaml` |
| The "app of apps" that installs every add-on | `4-platform-engineering/bootstrap.yaml` → everything under `4-platform-engineering/2-cluster-services/` |
| The tenant's boundary (namespace, quota, RBAC, policies) | `3-tenant-repos/tenant-a/gitops-repo/platform/` |
| What ArgoCD deploys per app | `3-tenant-repos/tenant-a/gitops-repo/services/<app>/dev/manifests/` (rendered by CI) |
| SLOs and alerts | `4-platform-engineering/2-cluster-services/observability/slo/` |
| Alert routing | `4-platform-engineering/2-cluster-services/observability/prometheus.yaml` (Alertmanager config) |

---

## Step 0 — Prepare the laptop (5 min)

1. Start **OrbStack**. Give it **at least 10 GB** of memory:
   `orbctl config set memory_mib 10240`, then restart OrbStack.
2. Check: `docker info --format '{{.MemTotal}}'` → about `10.7e9` or more (bytes).
3. If an old cluster exists, start clean: `k3d cluster list` → if `nexus-platform` is listed,
   `make destroy`.
4. `git pull` so you have the latest `main`.

## Step 1 — Give ArgoCD's controller a memory budget (repo change, 5 min)

**Why:** last time the controller used up the VM's memory and was killed 14 times. A request
reserves memory for it; a limit stops it taking everything. You will commit this yourself.

Edit `4-platform-engineering/2-cluster-services/gitops-orchestration/values.yaml`: under the
existing `controller:` key (next to `metrics:`), add:

```yaml
  resources:
    requests:
      cpu: 250m
      memory: 1Gi
    limits:
      memory: 2Gi
```

Then in `4-platform-engineering/1-cloud-foundation/local/README.md` add one line:
`Needs at least 10 GB of memory for Docker/OrbStack (orbctl config set memory_mib 10240).`

Check and ship it (ArgoCD later reads values from GitHub, so push):
```bash
helm template x argo/argo-cd -f 4-platform-engineering/2-cluster-services/gitops-orchestration/values.yaml \
  | awk '/kind: StatefulSet/{f=1} f&&/resources:/{print; for(i=0;i<5;i++){getline; print}; exit}'
# expect: limits memory 2Gi, requests cpu 250m / memory 1Gi  (tested 2026-10-05)
git add 4-platform-engineering/2-cluster-services/gitops-orchestration/values.yaml 4-platform-engineering/1-cloud-foundation/local/README.md
git commit -m "fix(argocd): give the application controller a memory request and limit"
git push
```
(If `helm template` says the repo is missing: `helm repo add argo https://argoproj.github.io/argo-helm`.)

## Step 2 — Create the cluster and install ArgoCD (5–10 min)

```bash
make create-cluster          # k3d cluster "nexus-platform"
make install-argocd          # Helm installs ArgoCD with values.yaml
kubectl -n argocd get pods   # wait until all are Running
kubectl -n argocd get statefulset argocd-application-controller \
  -o jsonpath='{.spec.template.spec.containers[0].resources}'; echo   # your 1Gi / 2Gi
```

## Step 3 — Bootstrap the platform and watch it come up (10–20 min)

```bash
make bootstrap               # registers the cluster + applies the app-of-apps
make get-argocd-creds        # prints the admin password for http://argocd.localhost
watch -n 10 'kubectl -n argocd get app -o custom-columns=NAME:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status'
```
**What to look for:** add-ons turn `Synced` / `Healthy` one wave at a time. Open
`http://argocd.localhost` too — the tree view shows *why* something is stuck.

**Expected, not a bug:** `ack-iam-controller`, `ack-s3-controller` stay `Degraded` (no AWS
credentials). `keycloak`, `sealed-secrets` may show sync `Unknown` with health `Healthy`.

**If something else is stuck:** in the ArgoCD UI open the app → red resource → "Events"/"Logs".
Write down the exact message — every real bug in the 2026-09-22 run was found this way.
Also check `kubectl -n argocd get pods` for `OOMKilled` / restarts.

## Step 4 — See both services deployed (10 min)

```bash
kubectl -n argocd get app | grep tenant-a          # tenant-a-app-a-dev-..., tenant-a-app-b-dev-...
kubectl -n tenant-a get pods,svc,ingress
```
**What to look for:** one `app-a` and one `app-b` pod `Running`. If a pod is stuck:
`kubectl -n tenant-a describe pod <name>` → read **Events** at the bottom
(`ImagePullBackOff`, quota, LimitRange and PodSecurity messages show up there).

Ask them for a page (Traefik routes by hostname; we send the hostname as a header):
```bash
curl -s -H 'Host: app-a-dev.local' http://127.0.0.1/          # <h1>Greetings from Go app app-a!</h1>
curl -s -H 'Host: app-b-dev.local' http://127.0.0.1/          # ...app-b!
curl -s -H 'Host: app-a-dev.local' http://127.0.0.1/healthz   # OK
```
If you get a redirect (`301/308`), retry with `https://127.0.0.1/` and `-k`.

**This is the moment the whole chain is proven:** a Backstage-style request → PR → CI image →
rendered manifests → ArgoCD → a running pod answering HTTP.

## Step 5 — Look at the tenancy walls (10 min, learning)

```bash
kubectl get ns tenant-a --show-labels                 # pod-security.kubernetes.io/enforce=restricted
kubectl -n tenant-a get resourcequota,limitrange,networkpolicy,rolebinding
kubectl -n tenant-a auth can-i create deployments --as=system:serviceaccount:tenant-a:default   # no
```
Match each object to its template in `1-platform-catalog/per-tenant/gitops/platform/tenancy/`.

## Step 6 — Fire an alert and follow the runbook (20 min)

1. Check the real rules are loaded:
   ```bash
   kubectl -n monitoring get prometheusrule | grep -i app-a
   kubectl -n monitoring port-forward svc/prometheus-operated 9090:9090
   ```
   Open `http://localhost:9090/alerts` → the `AppA...BurnRate...` alerts are listed as
   **inactive** (expected: no metrics yet — the honest gap). Stop the port-forward (ctrl-c).
2. Fire the synthetic page alert and see it delivered:
   ```bash
   make fire-synthetic-alert
   kubectl -n monitoring logs deploy/alert-webhook-receiver | tail -20   # SyntheticTestAlert, severity page
   ```
3. Open `docs/runbooks/app-a-availability-burn.md` and **do every diagnosis step** against the
   live cluster. Note each step that is wrong, unclear or impossible locally — that list is the
   point of this step. Fix the runbook text afterwards (commit it).

## Step 7 — Record it and commit the evidence (20 min)

1. Make `docs/demo/`. Record ~90 seconds showing Steps 4 and 6 (the curl answers, the alert in
   the receiver log):
   ```bash
   mkdir -p docs/demo && asciinema rec docs/demo/2026-10-phase14.cast
   ```
   (type `exit` to stop). Optionally screenshots: ArgoCD tree for `tenant-a-app-a-...`, the
   Prometheus alerts page → `docs/demo/*.png`.
2. Write `docs/demo/README.md`: date, what was run, what was seen, what was NOT proven
   (copy the table at the top), and the runbook problems you found.
3. Link it from the main `README.md` Quickstart, commit, push.

## Done means

- [ ] ArgoCD controller has requests/limits, pushed; local README states the memory minimum.
- [ ] All add-ons Synced/Healthy except the two expected ACK controllers.
- [ ] `app-a` and `app-b` answer HTTP through Traefik.
- [ ] Synthetic page alert seen in the receiver log; real rules seen loaded (inactive).
- [ ] Runbook walked; corrections committed.
- [ ] `docs/demo/` has the recording + README, linked from the main README.
- [ ] Tell Claude: what broke, the exact error lines. Each real bug becomes a fix + a test/rule.

Then ask Claude to update `PLAN.md` (Phase 14 → Recently done; Phase 19 gate 6 is covered by this).

## Next, proposed: Phase 25 — make the real alert fire

Add the OpenTelemetry Go SDK to the Go runtime template
(`1-platform-catalog/per-service/apps/runtimes/go/main.go.tmpl`) so every Go service exports
`http_server_request_duration_seconds` to the platform collector, plus an `/error` test endpoint
gated by an env var. Then Phase 14's Step 6 can drive real 5xx traffic and watch
`AppAAvailabilityBurnRatePageFast` go pending → firing in Prometheus. Good plan → Sonnet → review job.
