# Postmortem: Traefik could not reach tenant pods (NetworkPolicy ingress bug)

**Date:** 2026-08-20
**Status:** Resolved (commit `399e831`, "fix: rewrite NetworkPolicy and README
Component Matrix")
**Severity:** Would have been a full outage for every ingress-enabled service,
had this platform carried real traffic when the bug shipped.

## Summary

The tenant `NetworkPolicy` scaffolded by `onboard-team` allowed ingress only
from namespaces labelled `team: <team-name>`. Traefik — the platform's only
ingress controller — runs in its own `traefik` namespace, which carries no
such label. Every tenant service that enabled `ingress.enabled: true` in its
Helm values (the default for a scaffolded service) was therefore completely
unreachable from outside the cluster: Traefik could resolve the route, but
the tenant's own `NetworkPolicy` dropped the packet before it reached the
pod.

## Timeline

All times approximate, reconstructed from the commit history rather than a
live incident channel — this was caught during platform development, not in
a running production system, which is exactly what Phase 0's "truth-up" pass
was for.

- Multi-tenancy NetworkPolicy scaffolding (`default-deny-and-dns`) is added,
  restricting ingress to same-team traffic plus DNS egress — correct for
  east-west isolation, silent on the ingress-controller path.
- `charts/service` gains an `Ingress` template rendering
  `ingressClassName: traefik`, assuming north-south traffic already works.
- A structural audit against `4-platform-engineering/argocd-apps/ingress-routing/traefik.yaml`
  (Traefik's namespace) finds no corresponding `NetworkPolicy` rule allowing
  ingress from that namespace into any tenant namespace.
- Fix lands in commit `399e831`: the single `default-deny-and-dns` blob is
  replaced with five named policies (Phase 0.3), one of which explicitly
  allows ingress from the `traefik` namespace via
  `namespaceSelector: matchLabels: kubernetes.io/metadata.name: traefik`.

## Impact

None in practice — this was found and fixed during platform construction,
before any tenant workload depended on it in a shared environment. Named here
as if it were a production incident because the failure mode is real and the
blast radius, had it shipped live, would have been: **every** ingress-enabled
service on the platform, simultaneously, with every other health signal
green (pods Running, ArgoCD Synced/Healthy, Traefik itself Healthy) — nothing
in the platform's own status surfaces would have indicated a problem. Only an
actual `curl` from outside the cluster, or a `NetworkPolicy`-aware alert,
would have caught it.

## Detection

Static audit of the tenant NetworkPolicy against the ingress controller's
actual namespace, cross-referenced by hand while writing Phase 0 of
`PLAN.md`. **Not** caught by:
- `kubectl apply --dry-run=client` — a `NetworkPolicy` with a selector that
  matches zero namespaces is syntactically valid; dry-run has no way to know
  the selector is wrong.
- ArgoCD sync status — `NetworkPolicy` objects have no controller reporting
  "this blocks all my traffic"; Kubernetes just enforces them silently.
- Any test in `.github/workflows/tenant-workloads-ci-cd.yaml` — CI runs
  `helm template` only, never applies manifests to a real cluster, so a
  NetworkPolicy that renders valid YAML but expresses the wrong policy passes
  every existing check.

## Root Cause

The NetworkPolicy was written to solve same-team east-west isolation and
implicitly assumed that was the only ingress path that mattered. The
ingress-controller-to-tenant-pod path is north-south traffic originating
from a different namespace entirely, and nothing in the original design
enumerated "who else needs to reach a tenant pod besides its own team" as a
question to answer.

## Contributing Factors

- **One policy doing two jobs.** `default-deny-and-dns` combined default-deny,
  DNS, and same-team rules into a single object, which made it easy to reason
  about "same-team traffic is allowed" and easy to miss "nothing else is."
  Phase 0.3's fix of splitting it into five *named* policies (deny-all, DNS,
  same-team, ingress-controller, external-egress) is a direct response to
  this — a named policy documents the intent it is missing, where a blob does
  not prompt the question.
- **No environment exercised the failure.** k3d ships a NetworkPolicy
  controller by default (unlike some local setups), so this bug was live
  locally too — it was not hidden by the local/EKS gap this platform is
  otherwise careful to name. It was simply never tested by actually curling
  an ingress-enabled service from outside the cluster.
- **CI validates rendering, not behavior.** `helm template` plus
  `kubectl apply --dry-run=client` both confirm a manifest is well-formed
  Kubernetes YAML. Neither can confirm the *policy it expresses* achieves the
  intended network behavior — that requires either a live cluster test or a
  human reading the selector against the actual topology.

## What Would Have Caught This Earlier

An end-to-end smoke test in CI that provisions a k3d cluster, applies the
rendered tenant manifests, and `curl`s an ingress-enabled service from
outside the cluster would have caught this on the first PR. That test does
not exist yet — it is a real gap, not a hypothetical one, and is separate
from the Phase 3.6 Terraform-plan CI work (that closes the infrastructure
loop; this would close the network-policy-behavior loop).

**Does Phase 4 catch this?** No, and it is worth being honest about why.
Phase 4's burn-rate alerts (`app-a-alerts.yaml`) fire on the 5xx rate and
latency *observed by app-a itself* — a `NetworkPolicy` that blocks all
ingress before Traefik ever reaches the pod produces **no traffic for app-a
to measure at all**, so `http_server_request_duration_seconds_count` simply
never increments. A burn-rate alert defined as "errors / total" is silent
when total is zero. The gap this leaves: an availability SLO alert alone
cannot distinguish "the service is healthy and idle" from "the service is
completely unreachable." The alert that *would* catch this class of bug is
one on the ingress controller's own side — e.g. a burn-rate-style alert on
Traefik's `traefik_service_requests_total` (or the 5xx/timeout rate it
reports for a backend), which would show requests arriving at the edge and
never succeeding, rather than an app-side alert that never sees them arrive.
That is not built in this repo yet; it is named here as the concrete
follow-up this incident argues for, rather than left implicit.

## Action Items

- [x] Rewrite the tenant NetworkPolicy as five named policies, explicitly
      allowing Traefik-namespace ingress (Phase 0.1/0.3, commit `399e831`).
- [x] Also close the matching egress gap (Phase 0.2) — same audit found that
      egress was scoped to same-team-plus-DNS only, which would have
      firewalled every tenant off from RDS and every AWS API call.
- [ ] Add a CI smoke test that applies rendered manifests to a k3d cluster and
      curls an ingress-enabled service end-to-end (no owner yet).
- [ ] Add a Traefik-side burn-rate alert (edge 5xx/timeout rate) so a total
      ingress outage is caught even when the affected service reports zero
      traffic rather than errors (no owner yet — natural follow-up to
      Phase 4).
