# Hub-account networking for the EKS cluster in ../cluster/. Standalone root
# module (own state, own `terraform validate`) rather than a shared module,
# because Phase 5.1's cluster and this VPC are applied by the same team at
# roughly the same cadence — splitting further would only add remote-state
# indirection with no isolation benefit.
terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
}

variable "env" {
  type        = string
  description = "Environment this VPC belongs to (Phase 5.4: one account per environment)"
  default     = "hub"
}

variable "vpc_cidr" {
  type        = string
  default     = "10.100.0.0/16"
  description = "Hub VPC CIDR. Deliberately outside 10.0.0.0/8 to avoid colliding with the tenant VPCs the Python scaffolder allocates (3-tenant-workloads/cloud_vpcs_allocated.yaml)."
}

variable "azs" {
  type        = list(string)
  default     = ["eu-central-1a", "eu-central-1b", "eu-central-1c"]
  description = "AZs to spread subnets across. EKS requires subnets in at least 2 AZs."
}

locals {
  name = "idp-hub-${var.env}"
}

resource "aws_vpc" "hub" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = local.name, ManagedBy = "terraform", Environment = var.env }
}

resource "aws_internet_gateway" "hub" {
  vpc_id = aws_vpc.hub.id
  tags   = { Name = "${local.name}-igw" }
}

# Public subnets — one per AZ, for the NAT gateway and any internet-facing LB.
resource "aws_subnet" "public" {
  count                   = length(var.azs)
  vpc_id                  = aws_vpc.hub.id
  availability_zone       = var.azs[count.index]
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  map_public_ip_on_launch = true

  tags = {
    Name                     = "${local.name}-public-${var.azs[count.index]}"
    "kubernetes.io/role/elb" = "1" # required for the AWS Load Balancer Controller to pick this subnet
  }
}

# Private subnets — one per AZ. Nodes and the EKS control-plane ENIs live here.
resource "aws_subnet" "private" {
  count             = length(var.azs)
  vpc_id            = aws_vpc.hub.id
  availability_zone = var.azs[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + length(var.azs))

  tags = {
    Name                              = "${local.name}-private-${var.azs[count.index]}"
    "kubernetes.io/role/internal-elb" = "1"
  }
}

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "${local.name}-nat-eip" }
}

# ONE NAT gateway, not one per AZ. This is a deliberate cost choice for a demo
# hub account (~$0.045/hour vs 3x that) — it is also a single point of failure
# for all private-subnet egress. Production would use one NAT gateway per AZ
# so an AZ failure does not take down every node's internet-bound traffic.
resource "aws_nat_gateway" "hub" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = { Name = "${local.name}-nat" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.hub.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.hub.id
  }
  tags = { Name = "${local.name}-public-rt" }
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.hub.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.hub.id
  }
  tags = { Name = "${local.name}-private-rt" }
}

resource "aws_route_table_association" "public" {
  count          = length(aws_subnet.public)
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "private" {
  count          = length(aws_subnet.private)
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

output "vpc_id" {
  value = aws_vpc.hub.id
}

output "private_subnet_ids" {
  value = aws_subnet.private[*].id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}
