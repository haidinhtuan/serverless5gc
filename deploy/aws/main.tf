# Replicates the IONOS meridian-lab topology on AWS EC2:
#   node-a (serverless 5GC: K3s, OpenFaaS CE, Redis, etcd, sctp-proxy, monitoring)
#   node-b (loadgen: UERANSIM)
# Both nodes sit on one public subnet and reach each other over the VPC.

data "aws_availability_zones" "available" {
  state = "available"
}

# Latest Ubuntu 22.04 LTS (Jammy). Pinned to 22.04 on purpose: UERANSIM 3.2.6
# does not compile on Ubuntu 24.04.
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# ---------------------------------------------------------------------------
# Network
# ---------------------------------------------------------------------------
resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.project_name}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "${var.project_name}-igw" }
}

resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.subnet_cidr
  availability_zone       = data.aws_availability_zones.available.names[0]
  map_public_ip_on_launch = true

  tags = { Name = "${var.project_name}-public" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = { Name = "${var.project_name}-public-rt" }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# ---------------------------------------------------------------------------
# Security group
# ---------------------------------------------------------------------------
resource "aws_security_group" "nodes" {
  name        = "${var.project_name}-nodes"
  description = "serverless 5GC nodes: SSH, OpenFaaS gateway, N2 SCTP, metrics, intra-VPC"
  vpc_id      = aws_vpc.main.id

  tags = { Name = "${var.project_name}-nodes" }
}

# All traffic between nodes in the VPC (K3s control/data plane, Redis, etcd,
# and the internal OpenFaaS gateway on :8080 used by the cold-start path).
resource "aws_security_group_rule" "intra_vpc" {
  type              = "ingress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = [var.vpc_cidr]
  security_group_id = aws_security_group.nodes.id
  description       = "All intra-VPC traffic"
}

locals {
  # protocol numbers: tcp, and 132 = SCTP (no named protocol in AWS SG rules)
  admin_tcp_ports = {
    ssh           = 22
    openfaas_gw   = 31112
    cadvisor      = 8081
    node_exporter = 9100
    prometheus    = 9090
    openfaas_prom = 30175
  }
}

resource "aws_security_group_rule" "admin_tcp" {
  for_each          = local.admin_tcp_ports
  type              = "ingress"
  from_port         = each.value
  to_port           = each.value
  protocol          = "tcp"
  cidr_blocks       = [var.admin_cidr]
  security_group_id = aws_security_group.nodes.id
  description       = each.key
}

# N2 / NGAP over SCTP (sctp-proxy NodePort 38412). UERANSIM on node-b connects
# to node-a; the intra-VPC rule already covers node-to-node, this opens it to
# the admin CIDR too for external loadgen / debugging.
resource "aws_security_group_rule" "sctp_n2" {
  type              = "ingress"
  from_port         = 38412
  to_port           = 38412
  protocol          = "132"
  cidr_blocks       = [var.admin_cidr]
  security_group_id = aws_security_group.nodes.id
  description       = "N2 NGAP SCTP"
}

resource "aws_security_group_rule" "egress_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.nodes.id
  description       = "All egress"
}

# ---------------------------------------------------------------------------
# Key pair
# ---------------------------------------------------------------------------
resource "aws_key_pair" "main" {
  key_name   = "${var.project_name}-key"
  public_key = file(pathexpand(var.ssh_public_key_path))
}

# ---------------------------------------------------------------------------
# Instances
# ---------------------------------------------------------------------------
resource "aws_instance" "node_a" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.nodes.id]
  key_name               = aws_key_pair.main.key_name

  root_block_device {
    volume_type = "gp3"
    volume_size = var.node_a_volume_size
  }

  tags = {
    Name = "node-a"
    Role = "serverless-5gc"
  }
}

resource "aws_instance" "node_b" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.nodes.id]
  key_name               = aws_key_pair.main.key_name

  root_block_device {
    volume_type = "gp3"
    volume_size = var.node_b_volume_size
  }

  tags = {
    Name = "node-b"
    Role = "loadgen"
  }
}

# ---------------------------------------------------------------------------
# Optional Elastic IPs (stable addresses across stop/start)
# ---------------------------------------------------------------------------
resource "aws_eip" "node_a" {
  count    = var.use_elastic_ips ? 1 : 0
  instance = aws_instance.node_a.id
  domain   = "vpc"
  tags     = { Name = "node-a-eip" }
}

resource "aws_eip" "node_b" {
  count    = var.use_elastic_ips ? 1 : 0
  instance = aws_instance.node_b.id
  domain   = "vpc"
  tags     = { Name = "node-b-eip" }
}
