# Runbook: app-a latency error-budget burn

Covers all four latency burn-rate alerts in
`4-platform-engineering/2-cluster-services/observability/slo/app-a-alerts.yaml`
(`AppALatencyBurnRatePageFast`, `PageSlow`, `TicketSlow`, `TicketVerySlow`).
One runbook, not four — see `app-a-availability-burn.md` for why.

| Alert | Severity | Burn rate | Meaning |
|---|---|---|---|
| `AppALatencyBurnRatePageFast` | page | 14.4x | Budget exhausted in ~2 days if sustained |
| `AppALatencyBurnRatePageSlow` | page | 6x | Budget exhausted in ~5 days if sustained |
| `AppALatencyBurnRateTicketSlow` | ticket | 3x | Budget exhausted in ~10 days if sustained |
| `AppALatencyBurnRateTicketVerySlow` | ticket | 1x | Exactly the sustainable long-run pace |

## Symptom

Checkout (`app-a`, `team-a` namespace) is serving a share of requests slower
than the 500ms threshold large enough to burn through the 99%/30-day latency
error budget (1% of requests per 30 days — see `app-a-slo.yaml`) faster than
sustainable. Unlike the availability runbook, this is about *slow* responses,
not failed ones — pods may show 2xx and Healthy the whole time.

## Impact

Users attempting checkout in `team-a`'s namespace are experiencing degraded
response times. A `page` means the budget burns out in under a week if this
continues; a `ticket` means it is sustained but not yet urgent.

## Diagnose

All commands below are runnable under the Phase 1.1 developer `Role` — no
`pods/exec`. See Mitigate for why that constraint shapes the response.

```bash
# Resource pressure is the most common cause of a latency-only regression
# (no errors, just slow) — check for CPU/memory throttling first.
kubectl -n team-a get pods -l app.kubernetes.io/instance=app-a -o wide
kubectl -n team-a top pods -l app.kubernetes.io/instance=app-a 2>/dev/null || \
  echo "metrics-server not available locally — check the LimitRange ceiling instead:"
kubectl -n team-a describe limitrange

# Trace latency by span in Tempo — find the slow spans, not just the slow
# request. This is the actual diagnostic value of tracing over logs here.
kubectl -n monitoring port-forward svc/tempo 3100:3100 &
curl -sG http://localhost:3100/api/search \
  --data-urlencode 'tags=service.name="app-a"' \
  --data-urlencode 'minDuration=500ms'

# Correlate with logs for the same window (Loki)
kubectl -n monitoring port-forward svc/loki-gateway 3100:80 &
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="team-a", app="app-a"}' \
  --data-urlencode 'start='"$(date -u -v-1H +%s)"'000000000' \
  --data-urlencode 'end='"$(date -u +%s)"'000000000'

# Grafana UI — Explore -> Tempo, filter by duration, is faster than the raw
# API for finding the slowest 1% of traces.
open http://grafana.localhost
```

Common root causes to check for, in rough order of likelihood: a downstream
dependency (Postgres, an external API) slowing down; CPU throttling against
the LimitRange `max` ceiling (Phase 1.4); a bad deploy that regressed a hot
path; connection pool exhaustion under load.

## Mitigate

Same constraint as the availability runbook: **no `pods/exec`**, so
mitigation is a git revert plus an ArgoCD sync, not live tuning on the pod.

```bash
git -C 3-tenant-workloads log --oneline -- team-a/gitops/apps/app-a/dev
git -C 3-tenant-workloads revert <bad-commit-sha>
git -C 3-tenant-workloads push
argocd app sync team-a-appsets
```

If the regression traces back to a downstream dependency rather than app-a's
own code or config, a revert will not help — escalate instead.

## Escalate

- `page` alerts: page the on-call owner for `team-a` immediately if the
  revert above does not resolve it within 15 minutes.
- `ticket` alerts: file a ticket against `team-a`; no immediate page needed
  unless the trend accelerates into a `page`-severity burn rate.
