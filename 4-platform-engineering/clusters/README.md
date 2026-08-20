# Clusters

The cluster is infrastructure too — this directory is where its definition lives once
there is one to check in.

**Today:** the local cluster is provisioned imperatively by the root `Makefile`
(`make create-cluster`, `CLUSTER_PROVIDER=k3d` by default — see `CREATE_CLUSTER_CMD` in
`Makefile`). There is no committed k3d config file because the CLI flags are the whole
configuration (Traefik disabled so the addon in `../addons/ingress-routing/` owns
ingress, ports 80/443 mapped to the loadbalancer).

**Later:** the production target is EKS via Terraform. When that lands, the module(s)
belong here, versioned the same way as everything under
`../cloud-services-terraform-modules/`.
