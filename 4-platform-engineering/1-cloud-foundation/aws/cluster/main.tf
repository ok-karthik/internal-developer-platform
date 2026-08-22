# The hub EKS cluster. This is THE production target named in PLAN.md's
# Target Environment section — held to a clean `terraform plan` in CI
# (Phase 3.6's runner), not applied by default. Takes subnet IDs as
# variables rather than reading ../network/'s state directly, so this module
# stays independently `validate`-able — the same pattern every capability
# module in 3-capability-modules/ already uses.
terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
  backend "s3" {
    bucket       = "acme-corp-terraform-state"
    key          = "platform/aws/cluster/terraform.tfstate"
    region       = "us-east-1"
    use_lockfile = true
    encrypt      = true
  }
}

variable "cluster_name" {
  type    = string
  default = "idp-hub"
}

variable "kubernetes_version" {
  type        = string
  default     = "1.31"
  description = "Pinned so AWS cannot silently move the control plane to a new default minor version under you."
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Nodes and the control plane's cross-account ENIs live here."
}

variable "api_allowed_cidrs" {
  type        = list(string)
  description = "CIDRs allowed to reach the public API endpoint. Never 0.0.0.0/0 — see the private-endpoint note below."
  default     = []
}

variable "node_instance_type" {
  type    = string
  default = "t3.medium"
}

variable "node_desired_size" {
  type    = number
  default = 2
}

# --- IAM: cluster role -------------------------------------------------------

resource "aws_iam_role" "cluster" {
  name = "${var.cluster_name}-cluster-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "cluster_policy" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# --- The cluster --------------------------------------------------------------

resource "aws_eks_cluster" "hub" {
  name     = var.cluster_name
  role_arn = aws_iam_role.cluster.arn
  version  = var.kubernetes_version

  vpc_config {
    subnet_ids = var.private_subnet_ids

    # Private API endpoint plus a public CIDR allowlist, not a bare public
    # endpoint open to 0.0.0.0/0 (the default a first `terraform apply` would
    # otherwise produce). Nodes always reach the API privately; the public
    # path exists only for CI/kubectl from an allowlisted range.
    endpoint_private_access = true
    endpoint_public_access  = length(var.api_allowed_cidrs) > 0
    public_access_cidrs     = length(var.api_allowed_cidrs) > 0 ? var.api_allowed_cidrs : null
  }

  # api/audit/authenticator logging — Phase 4 (alerting) and Phase 7
  # (who-authenticated-as-whom) both need this to mean anything. The other
  # two log types (controllerManager, scheduler) are noisy and not needed
  # for either.
  enabled_cluster_log_types = ["api", "audit", "authenticator"]

  encryption_config {
    provider {
      key_arn = aws_kms_key.eks_secrets.arn
    }
    resources = ["secrets"]
  }

  depends_on = [aws_iam_role_policy_attachment.cluster_policy]

  tags = { ManagedBy = "terraform", Name = var.cluster_name }
}

resource "aws_kms_key" "eks_secrets" {
  description             = "Envelope encryption for ${var.cluster_name} Kubernetes Secrets (etcd-level, on top of TLS in transit)"
  deletion_window_in_days = 7
}

# --- Node group ---------------------------------------------------------------

resource "aws_iam_role" "node" {
  name = "${var.cluster_name}-node-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "node_worker" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_iam_role_policy_attachment" "node_cni" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

resource "aws_iam_role_policy_attachment" "node_ecr" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# A launch template exists for exactly one reason: enforcing IMDSv2 with a hop
# limit of 1. The default node group launch (no template) permits IMDSv1 and
# an unlimited hop count — the node-level counterpart to the Phase 0.3 IMDS
# egress NetworkPolicy rule. Without this, the NetworkPolicy blocks pod-level
# SSRF but a compromised process on the HOST network namespace can still walk
# to IMDS at hop 1.
resource "aws_launch_template" "node" {
  name_prefix = "${var.cluster_name}-node-"

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required" # IMDSv2 only
    http_put_response_hop_limit = 1
  }

  tag_specifications {
    resource_type = "instance"
    tags          = { Name = "${var.cluster_name}-node" }
  }
}

resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.hub.name
  node_group_name = "default"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = var.private_subnet_ids
  instance_types  = [var.node_instance_type]

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  scaling_config {
    desired_size = var.node_desired_size
    min_size     = 1
    max_size     = var.node_desired_size + 2
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_worker,
    aws_iam_role_policy_attachment.node_cni,
    aws_iam_role_policy_attachment.node_ecr,
  ]
}

# --- EKS add-ons ----------------------------------------------------------
# These are what AWS itself calls "add-ons" — vpc-cni, coredns, kube-proxy,
# ebs-csi and the pod-identity agent are managed EKS resources, distinct from
# this repo's ArgoCD-deployed cluster ADD-ONS in ../../2-cluster-services/.
# Phase 3.8 renamed that directory away from `addons/` specifically to avoid
# this collision; naming the collision here too is why it does not recur.

resource "aws_eks_addon" "vpc_cni" {
  cluster_name = aws_eks_cluster.hub.name
  addon_name   = "vpc-cni"
}

resource "aws_eks_addon" "coredns" {
  cluster_name = aws_eks_cluster.hub.name
  addon_name   = "coredns"
  depends_on   = [aws_eks_node_group.default]
}

resource "aws_eks_addon" "kube_proxy" {
  cluster_name = aws_eks_cluster.hub.name
  addon_name   = "kube-proxy"
}

resource "aws_eks_addon" "ebs_csi" {
  cluster_name = aws_eks_cluster.hub.name
  addon_name   = "aws-ebs-csi-driver"
  depends_on   = [aws_eks_node_group.default]
}

# Phase 7.2c: EKS Pod Identity is the platform's workload-identity mechanism.
# This agent is what makes PodIdentityAssociation (../workload-identity/pod-identity.tf)
# work — without it, associations exist in AWS but pods get no credentials.
resource "aws_eks_addon" "pod_identity" {
  cluster_name = aws_eks_cluster.hub.name
  addon_name   = "eks-pod-identity-agent"
  depends_on   = [aws_eks_node_group.default]
}

output "cluster_name" {
  value = aws_eks_cluster.hub.name
}

output "cluster_endpoint" {
  value = aws_eks_cluster.hub.endpoint
}

output "oidc_issuer_url" {
  description = "Needed by the IRSA fallback in ../workload-identity/irsa.tf"
  value       = aws_eks_cluster.hub.identity[0].oidc[0].issuer
}

output "cluster_certificate_authority" {
  value = aws_eks_cluster.hub.certificate_authority[0].data
}
