# ADR 0014: Observability Boundary — Local SLO Loop Only, Central Telemetry Upstream

**Date:** 2026-09-29
**Status:** Accepted

## Context

This repository previously deployed a full LGTM observability stack (Loki, Promtail, Tempo, Prometheus, Alertmanager, Grafana) directly in the platform cluster. That duplicated [`opentelemetry-platform-on-eks`](https://github.com/ok-karthik/opentelemetry-platform-on-eks) (the "central platform"), consumed dedicated cluster compute, and was never fully functional from here.

During audit and specification (Phase 20), three underlying gaps were identified that made the previous stack look more complete than it was:
1. **No metrics path existed:** The OpenTelemetry `Instrumentation` CRs only sent traces to Tempo. Prometheus had no scrape configuration or collector receiving tenant application metrics, so the SLO burn-rate alerts could never fire.
2. **Tenant pods could not reach the `monitoring` namespace:** The default tenant `allow-egress-external` NetworkPolicy excluded `10.0.0.0/8` (covering in-cluster pod IPs in k3d/EKS), with no egress rule allowing traffic to `monitoring`. Even OTLP traces to Tempo were blocked at the network boundary.
3. **Protocol and port mismatch:** Both `Instrumentation` CRs sent all runtimes to `:4317` (gRPC). The OpenTelemetry operator's auto-instrumentation agents for Python and Java default to OTLP/HTTP and require `:4318`.

The local continuous delivery and automated rollback story requires only PromQL burn-rate rules evaluated against Prometheus and routed via Alertmanager. Running long-term log indexing and distributed trace storage here is duplicate overhead.

## Decision

Establish an explicit boundary between the local platform cluster's SLO loop and the upstream central telemetry platform:

| Component | Lives in | Purpose |
|---|---|---|
| **Prometheus + Alertmanager + Grafana** | This repo (`observability/prometheus.yaml`, 48h retention) | Local SLO evaluation, burn-rate alerting, automated rollback gating, and local dashboards |
| **`platform-otlp` Collector** | This repo (`observability/otel-collector.yaml`) | Single tenant telemetry ingress, label enrichment (`service_name`, `namespace`), metrics export to local Prometheus |
| **SLO Rules & Runbooks** | This repo (`observability/slo/`, `docs/runbooks/`) | Multi-window multi-burn-rate alerting declarations and operational runbooks |
| **Long-term Logs & Traces** | `opentelemetry-platform-on-eks` | Central Loki, Tempo, OpenSearch, long-term metric retention, and fleet-wide querying |
| **AIOps & Root Cause Analysis** | `opentelemetry-platform-on-eks` / `sre-agent-guardrails` | Autonomous incident triage, remediation agents, and cross-cluster correlation |

### Operating Modes

1. **Standalone Mode (Default):**
   - The platform cluster runs completely independently on local k3d or test EKS clusters.
   - The `platform-otlp` collector routes metrics to local Prometheus over OTLP/HTTP (`otlphttp/prometheus`) and sends traces to a basic debug exporter.
   - The SLO loop is strictly local: a rollback signal does not depend on any cross-cluster network hop, central gateway NLB, or upstream availability.

2. **Connected Mode (Optional):**
   - When deployed in an AWS VPC peered or routed (via Transit Gateway) to `opentelemetry-platform-on-eks`, the collector can forward telemetry to the central internal NLB (`obs-cluster-otel-gw`).
   - Enabled by setting `UPSTREAM_OTLP_ENDPOINT` and adding `otlp/upstream` to the metric and trace pipeline exporters in `observability/otel-collector.yaml`.

### Rejected Alternatives

- **Keep full LGTM stack locally:** Rejected — duplicate infrastructure and license/compute cost; creates two desynchronized telemetry islands.
- **CloudWatch native metrics & alarms:** Rejected — requires rewriting `PrometheusRule` into AWS CloudWatch Alarms, introduces per-metric ingest costs, and breaks vendor-neutral Kubernetes portability.
- **Always forward (no local Prometheus):** Rejected — breaks standalone local evaluation, and compromises the automated rollback safety path by introducing an external cluster dependency.

## Consequences

- **Intentional duplicate:** There is one small Prometheus instance running in each cluster (48h retention) specifically for the local SLO feedback loop.
- **Standalone trade-off:** In standalone mode, there is no UI for logs or traces; logs are viewed via `kubectl logs`.
- **Connected mode requirements:** Connected mode requires VPC routing to the central platform's internal NLB, which cannot be tested on local k3d.
- **Dependency decoupling:** `sre-agent-guardrails` and upstream AIOps agents interact only with the central telemetry platform; this platform cluster is an optional data source, never a prerequisite.
- **Open gap:** The Go runtime template (`per-service/apps/runtimes/go/`) does not yet include an OpenTelemetry SDK (finding 4); auto-instrumentation injects environment variables only. `app-a` does not emit metric series until an SDK is linked.
- Manifests and collector configurations are validated statically; execution against a live multi-cluster AWS environment remains unverified.

## Revisit Trigger

Move connected mode to the default configuration if the central platform implements multi-tenant authentication and per-tenant rate limits, and this reference architecture ceases to require standalone local execution.
