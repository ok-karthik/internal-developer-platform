# Runbook: app-a availability error-budget burn

Covers all four availability burn-rate alerts in
`4-platform-engineering/2-cluster-services/observability/slo/app-a-alerts.yaml`
(`AppAAvailabilityBurnRatePageFast`, `PageSlow`, `TicketSlow`, `TicketVerySlow`).
One runbook, not four near-identical ones — the diagnosis and mitigation steps
are the same regardless of window; only urgency and escalation differ, which
is called out per-alert below.

| Alert | Severity | Burn rate | Meaning |
|---|---|---|---|
| `AppAAvailabilityBurnRatePageFast` | page | 14.4x | Budget exhausted in ~2 days if sustained |
| `AppAAvailabilityBurnRatePageSlow` | page | 6x | Budget exhausted in ~5 days if sustained |
| `AppAAvailabilityBurnRateTicketSlow` | ticket | 3x | Budget exhausted in ~10 days if sustained |
| `AppAAvailabilityBurnRateTicketVerySlow` | ticket | 1x | Exactly the sustainable long-run pace |

## Symptom

Checkout (`app-a`, `team-a` namespace) is returning 5xx to a share of users
large enough to burn through the 99.9%/30-day availability error budget
(43m12s of allowed downtime-equivalent per 30 days — see `app-a-slo.yaml`)
faster than sustainable.

## Impact

Users attempting checkout in `team-a`'s namespace are seeing failed requests.
A `page` alert means this is severe enough to exhaust the entire monthly
budget in under a week if it continues; a `ticket` means it is sustained but
not yet urgent.

## Diagnose

All commands below are runnable under the Phase 1.1 developer `Role`
(`3-tenant-workloads/team-a/gitops/platform/team/rbac.yaml`) — read/list/watch
plus `pods/log`. **There is no `pods/exec`** — see Mitigate.

```bash
# Pod-level health — is it crashing, or serving errors while healthy?
kubectl -n team-a get pods -l app.kubernetes.io/instance=app-a -o wide
kubectl -n team-a get events --sort-by=.lastTimestamp | tail -20

# Application logs for the failing pods (pods/log is explicitly granted)
kubectl -n team-a logs -l app.kubernetes.io/instance=app-a --tail=200 --since=1h

# Recent 5xx-tagged log lines via Loki (Grafana Explore, or the HTTP API directly
# — port-forward is the local-cluster equivalent of the Grafana Explore URL):
kubectl -n monitoring port-forward svc/loki-gateway 3100:80 &
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="team-a", app="app-a"} |= "status=5"' \
  --data-urlencode 'start='"$(date -u -v-1H +%s)"'000000000' \
  --data-urlencode 'end='"$(date -u +%s)"'000000000'

# Trace a specific failing request in Tempo (grab a trace ID from a 5xx log
# line above, or from Grafana Explore -> Tempo, since traces are wired via
# the Instrumentation CR to http://tempo.monitoring.svc.cluster.local:4317):
kubectl -n monitoring port-forward svc/tempo 3100:3100 &
curl -s http://localhost:3100/api/traces/<trace-id> | jq .

# Grafana UI (has both datasources pre-wired, easier for correlating
# logs <-> traces than raw curl):
open http://grafana.localhost   # after `make bootstrap` + ingress is up
```

## Mitigate

The developer Role has **no `pods/exec`** by design (Phase 1.1) — mitigation
is a git revert plus an ArgoCD sync, never a shell into a pod. This is the
GitOps loop being load-bearing under pressure:

```bash
# Identify the last-known-good commit for app-a's manifests/values
git -C 3-tenant-workloads log --oneline -- team-a/gitops/apps/app-a/dev

# Revert the suspect commit (image bump, config change, etc.)
git -C 3-tenant-workloads revert <bad-commit-sha>
git -C 3-tenant-workloads push

# Force an immediate sync instead of waiting for ArgoCD's poll interval
argocd app sync team-a-appsets
```

If the cause is external (a downstream dependency, the database), git revert
will not help — escalate instead of guessing.

## Escalate

- `page` alerts: page the on-call owner for `team-a` immediately if the
  revert above does not resolve it within 15 minutes.
- `ticket` alerts: file a ticket against `team-a`; no immediate page needed
  unless the trend accelerates into a `page`-severity burn rate.
