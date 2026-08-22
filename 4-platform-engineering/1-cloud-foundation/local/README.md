# Local (k3d)

The local cluster is provisioned imperatively by the root `Makefile` (`make create-cluster`,
`CLUSTER_PROVIDER=k3d` by default — see `CREATE_CLUSTER_CMD` in `Makefile`). There is no
committed k3d config file because the CLI flags are the whole configuration (Traefik
disabled so the addon in `../../2-cluster-services/ingress-routing/` owns ingress, ports
80/443 mapped to the loadbalancer).

**This is a test harness, not the platform.** It exists to demonstrate the GitOps
reconciliation loop cheaply and locally. It does not exercise IAM/workload identity,
real load balancers, zone-bound storage, cluster auth, IMDS, or node isolation — see the
comparison table in `PLAN.md`'s Target Environment section for the full list of what a
green local run does not prove. When `../aws/cluster/` lands, model production there
first and treat this directory as the approximation, never the reverse.
