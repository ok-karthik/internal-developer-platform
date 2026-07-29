variable "team_name" {
  type        = string
  description = "Team/Tenant name"
}

variable "app_name" {
  type        = string
  description = "Application name"
}

variable "vpc_id" {
  type        = string
  description = "The ID of the VPC"
  default     = "" # Add default empty string so it doesn't break if not passed when generating standalone
}

variable "subnet_ids" {
  type        = list(string)
  description = "A list of subnet IDs for the database subnet group"
  default     = []
}

provider "random" {}

resource "random_password" "db_password" {
  length           = 16
  special          = true
  override_special = "!@#$%&*"
  keepers          = { engine_version = "16" }
}

resource "random_string" "db_suffix" {
  length  = 4
  lower   = true
  special = false
}

resource "aws_db_subnet_group" "db_subnet_group" {
  # count creates this only if subnet_ids are provided
  count      = length(var.subnet_ids) > 0 ? 1 : 0
  name       = "${var.team_name}-${var.app_name}-db-subnet-group"
  subnet_ids = var.subnet_ids

  tags = {
    Name      = "${var.team_name}-${var.app_name}-db-subnet-group"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_security_group" "db_sg" {
  # count creates this only if vpc_id is provided
  count       = var.vpc_id != "" ? 1 : 0
  name        = "${var.team_name}-${var.app_name}-db-sg"
  description = "Security group for PostgreSQL database"
  vpc_id      = var.vpc_id

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/8"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name      = "${var.team_name}-${var.app_name}-db-sg"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
  }
}

resource "aws_db_instance" "app_db" {
  identifier        = "${var.team_name}-${var.app_name}-db-${random_string.db_suffix.result}"
  engine            = "postgres"
  instance_class    = "db.t3.micro"
  username          = "admin"
  password          = random_password.db_password.result
  allocated_storage = 20

  # Optional link to subnet group and security group if they exist
  db_subnet_group_name   = length(var.subnet_ids) > 0 ? aws_db_subnet_group.db_subnet_group[0].name : null
  vpc_security_group_ids = var.vpc_id != "" ? [aws_security_group.db_sg[0].id] : null

  # Guardrails
  publicly_accessible = false
  storage_encrypted   = true
  skip_final_snapshot = true

  tags = {
    Name      = "${var.team_name}-${var.app_name}-db"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
    Owner     = var.team_name
  }
}
