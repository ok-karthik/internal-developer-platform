# 1. Cloud Foundation

Terraform the **platform team** applies, **before** a cluster exists.

Nested by provider (`aws/`, and `local/` for the k3d test harness) because everything in
here is inherently provider-specific — the opposite of `../2-cluster-services/`, which is
portable Kubernetes and is deliberately *not* nested this way. **The size of `aws/`
relative to `2-cluster-services/` is the honest measure of how cloud-coupled this
platform actually is.**

```
1-cloud-foundation/
├── aws/                  # Provider-specific by nature → nested by provider.
│   ├── organization/     #   Organizations, OUs, SCPs, cross-account ACK IAM (Phase 5.2-5.4) ✅ built
│   ├── network/          #   VPC, subnets, one NAT gateway              (Phase 5.1) ✅ built
│   ├── cluster/          #   EKS. Held to `terraform plan`-clean in CI. (Phase 5.1) ✅ built
│   ├── cluster-access/   #   Access Entries + Identity Center  (Phase 7.2b) ✅ built
│   └── workload-identity/#   Pod Identity / IRSA seam          (Phase 7.2c) ✅ built
└── local/                #   k3d. A TEST HARNESS — never the reference. See local/README.md.
```

**Every `aws/` subdirectory above is real, `terraform validate`-clean HCL** — each is its
own root module (own state, own `terraform init -backend=false && terraform validate`),
taking cross-module references (VPC ID, subnet IDs, controller role ARNs, the cluster's
OIDC issuer URL) as input variables rather than reading another module's remote state, so
each stays independently validate-able, and each is `tflint`-clean (no unused-variable
warnings). **None has been `apply`'d against a real AWS account** — per the Target
Environment section, reaching a clean `plan` is the deliverable for this repo; an actual
`apply` is the optional, timed, `destroy`-same-session exercise described there.

Per the Target Environment section of `PLAN.md`: **EKS is the target, k3d is a test
harness.** Where the two genuinely differ (workload identity, load balancers, storage,
cluster auth, IMDS, node isolation — see the comparison table in `PLAN.md`), the EKS
mechanism is what belongs in this directory once it is written, and the k3d approximation
stays confined to `local/`.
