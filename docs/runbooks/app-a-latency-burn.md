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

Checkout (`app-a`, `tenant-a` namespace) is serving a share of requests slower
than the 500ms threshold large enough to burn through the 99%/30-day latency
error budget (1% of requests per 30 days — see `app-a-slo.yaml`) faster than
sustainable. Unlike the availability runbook, this is about *slow* responses,
not failed ones — pods may show 2xx and Healthy the whole time.

## Impact

Users attempting checkout in the `tenant-a` namespace are experiencing degraded
response times. A `page` means the budget burns out in under a week if this
continues; a `ticket` means it is sustained but not yet urgent.

## Diagnose

All commands below are runnable under the Phase 1.1 developer `Role` — no
`pods/exec`. See Mitigate for why that constraint shapes the response.

```bash
# Resource pressure is the most common cause of a latency-only regression
# (no errors, just slow) — check for CPU/memory throttling first.
kubectl -n tenant-a get pods -l app.kubernetes.io/instance=app-a -o wide
kubectl -n tenant-a top pods -l app.kubernetes.io/instance=app-a 2>/dev/null || \
  echo "metrics-server not available locally — check the LimitRange ceiling instead:"
kubectl -n tenant-a describe limitrange

# Application logs for the pods (pods/log is explicitly granted)
kubectl -n tenant-a logs -l app.kubernetes.io/instance=app-a --tail=200 --since=1h

# Traces exist only in connected mode (ADR 0014), forwarded to the central platform
# (https://github.com/ok-karthik/opentelemetry-platform-on-eks) and viewable in its
# central Grafana (Tempo). Standalone mode does not run a trace backend.

# Grafana UI (view Prometheus burn-rate alerts and latency SLO dashboard):
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
git -C 3-tenant-repos log --oneline -- tenant-a/gitops-repo/services/app-a/dev
git -C 3-tenant-repos revert <bad-commit-sha>
git -C 3-tenant-repos push
argocd app sync tenant-a-appsets
```

If the regression traces back to a downstream dependency rather than app-a's
own code or config, a revert will not help — escalate instead.

## Escalate

- `page` alerts: page the on-call owner for `team-a` immediately if the
  revert above does not resolve it within 15 minutes.
- `ticket` alerts: file a ticket against `team-a`; no immediate page needed
  unless the trend accelerates into a `page`-severity burn rate.
