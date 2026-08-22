# Identity & Single Sign-On

Short version: the platform's tenancy rules (who can touch what) need a real identity
system behind them, or they're just YAML nobody enforces. This doc covers how that
identity system works and where it plugs in.

## The problem this solves

Earlier in this platform's build-out, the RBAC rules were written to trust a group called
`oidc:<team>` — but nothing actually *issued* that group. No login system existed. The
rule was correct on paper and fictional in practice.

## Four separate questions, four separate answers

"Can team-a touch team-b's stuff?" isn't one question — it's four, each enforced by a
different system. Mixing them up is the most common mistake when reasoning about access
control here:

| Question | Answered by |
|---|---|
| What can a human do with `kubectl`? | Kubernetes RBAC (`Role`/`RoleBinding`) |
| Which ArgoCD apps can a human sync? | ArgoCD's own permission file (`policy.csv`) |
| What *kinds* of resources can be deployed, and where? | ArgoCD's `AppProject` |
| What can a *pod's own credentials* do in AWS? | Pod Identity / IRSA (this is not about humans at all) |

Every one of these is driven by **one group name**, chosen once and reused everywhere:
`platform:<team>:<tier>` — e.g. `platform:team-a:developer`, `platform:team-a:oncall`.

## Keycloak — a stand-in for a real corporate login system

This repo runs [Keycloak](https://www.keycloak.org/) as its identity provider, seeded
automatically (no manual clicking through an admin UI). It's a stand-in for whatever a real
company already has — Entra ID, Okta, Google Workspace — plus AWS IAM Identity Center.

The chain a real AWS company runs looks like this:

```
Corporate login (Entra ID / Okta)
        │  (SAML)
        ▼
AWS IAM Identity Center   — the hub that connects a corporate login to AWS
        │  (vends a role)
        ▼
An IAM role, one per team/account
        │
        ▼
EKS Access Entry   — maps that IAM role to a Kubernetes group
        │
        ▼
RoleBinding   — the same Kubernetes RBAC rule described above, unchanged
```

**On a real AWS cluster, human login uses IAM Access Entries, not raw OIDC settings on the
API server.** The raw OIDC flags exist and work (that's what this repo's local test cluster
uses, since it's simpler to demo without a real AWS account), but production AWS clusters
use the IAM path instead. ArgoCD, Grafana, and Backstage don't have an AWS-native option, so
they just log in against Keycloak directly, like any other web app would.

## Verify it yourself

These commands prove the rule, not just describe it:

```bash
kubectl auth can-i get pods          -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # yes
kubectl auth can-i create pods       -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # no
kubectl auth can-i get pods          -n team-b --as=dev --as-group=oidc:platform:team-a:developer  # no — wrong team
kubectl auth can-i create pods/exec  -n team-a --as=dev --as-group=oidc:platform:team-a:developer  # no — see Break-glass below
kubectl auth can-i create pods/exec  -n team-a --as=sre --as-group=oidc:platform:team-a:oncall      # yes
argocd account can-i sync applications 'team-b/*'   # no
```

## One group, five tools

| Tool | How it checks the group |
|---|---|
| Kubernetes API | `RoleBinding` (via OIDC locally, via IAM Access Entry on real AWS) |
| ArgoCD | its own `policy.csv`, one line per group |
| Grafana | maps the group to a Viewer/Editor role |
| Backstage | uses it to decide which services you own |
| AWS console | IAM Identity Center Permission Set |

Same string — `platform:team-a:developer` — in all five places. Add a sixth tool, and you
add one client + one mapping, not a new access model.

**The failure mode to watch for: drift.** If someone renames a group in Keycloak and
forgets to update `policy.csv`, access breaks (or worse, stays open) silently — nothing
errors out.

## Break-glass access

Emergency access (`pods/exec`, reading secrets) is a *separate* role,
`platform-breakglass`, bound only to an on-call group that nobody is a permanent member
of. This repo builds the permission rule; the part where membership is granted for a shift
and automatically revoked afterward is a login-system workflow this repo doesn't
implement.

## Secrets: two approaches, kept side by side

- **Sealed Secrets** — the encrypted value lives in git. Works offline, easy to audit,
  rotating a secret means re-encrypting and committing.
- **External Secrets Operator (ESO)** — git holds only a *reference*; the real value is
  fetched from AWS Secrets Manager at sync time. Rotation is automatic, but the cluster now
  needs AWS to be reachable to start up.

Both are installed. Neither is strictly better — it depends on whether you'd rather have
an offline-capable system or automatic rotation.
