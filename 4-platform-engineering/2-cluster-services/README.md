# 2. Cluster Services

ArgoCD `Application`/`ApplicationSet` manifests the **platform team** reconciles,
**after** a cluster exists — merged with the raw namespaced resources some of them
deploy (e.g. `ingress-routing/middlewares.yaml`, `observability/otel-instrumentation.yaml`),
because installer and config are one concept, not two directories fighting over the same
naming axis (see `PLAN.md` Phase 3.7(c)).

Portable Kubernetes, so — unlike `../1-cloud-foundation/` — **not** nested by provider.
This directory is what makes a cluster a platform.

`bootstrap.yaml` (one level up) is the root of the App-of-Apps pattern: it applies this
whole directory with `directory.recurse: true` and `destination.namespace: argocd`. Every
namespaced resource in here — `ConfigMap`, `Ingress`, `Middleware`, `Instrumentation`, and
any future kind — must set `metadata.namespace` explicitly; only `Application`/
`ApplicationSet` objects may rely on the bootstrap default. Every file also carries an
explicit `argocd.argoproj.io/sync-wave` (`0` = CRD-providing installers, `1` = remaining
installers, `2` = namespaced config, `3` = policies and the tenant `ApplicationSet`) — see
`.agents/AGENTS.md` for the full rule and rationale.
