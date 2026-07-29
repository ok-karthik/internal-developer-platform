variable "team_name" {
  type        = string
  description = "Team/Tenant name"
}

variable "app_name" {
  type        = string
  description = "Application name"
}

variable "vpc_cidr" {
  type        = string
  default     = "10.0.0.0/16"
  description = "CIDR block for the VPC"
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  instance_tenancy     = "default"
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name      = "${var.team_name}-${var.app_name}-vpc"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
    Owner     = var.team_name
  }
}

resource "aws_subnet" "public_subnet_1" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, 0)
  map_public_ip_on_launch = true

  tags = {
    Name      = "${var.team_name}-${var.app_name}-public-subnet-1"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_subnet" "public_subnet_2" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, 1)
  map_public_ip_on_launch = true

  tags = {
    Name      = "${var.team_name}-${var.app_name}-public-subnet-2"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name      = "${var.team_name}-${var.app_name}-igw"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_route_table" "public_rt" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }

  tags = {
    Name      = "${var.team_name}-${var.app_name}-public-rt"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_route_table_association" "public_rta_1" {
  subnet_id      = aws_subnet.public_subnet_1.id
  route_table_id = aws_route_table.public_rt.id
}

resource "aws_route_table_association" "public_rta_2" {
  subnet_id      = aws_subnet.public_subnet_2.id
  route_table_id = aws_route_table.public_rt.id
}

output "vpc_id" {
  value       = aws_vpc.main.id
  description = "The ID of the VPC"
}

output "public_subnet_ids" {
  value       = [aws_subnet.public_subnet_1.id, aws_subnet.public_subnet_2.id]
  description = "The IDs of the public subnets"
}
