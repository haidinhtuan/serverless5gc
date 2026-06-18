variable "region" {
  description = "AWS region to deploy into (eu-central-1 is closest to the IONOS de/txl baseline)."
  type        = string
  default     = "eu-central-1"
}

variable "aws_profile" {
  description = "Named AWS CLI profile to authenticate with (use a member account, not the org management account)."
  type        = string
  default     = "serverless5gc"
}

variable "project_name" {
  description = "Name prefix applied to all resources."
  type        = string
  default     = "serverless5gc"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR block for the public subnet."
  type        = string
  default     = "10.0.1.0/24"
}

variable "instance_type" {
  description = "EC2 instance type for both nodes. c5.xlarge = 4 vCPU / 8 GB, matching the IONOS VMs."
  type        = string
  default     = "c5.xlarge"
}

variable "node_a_volume_size" {
  description = "Root volume size (GB) for node-a (serverless 5GC)."
  type        = number
  default     = 50
}

variable "node_b_volume_size" {
  description = "Root volume size (GB) for node-b (loadgen / UERANSIM)."
  type        = number
  default     = 40
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key imported as the EC2 key pair."
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "admin_cidr" {
  description = "CIDR allowed to reach SSH and the management/metrics ports. Set to your own IP/32; defaults to open which is insecure."
  type        = string
  default     = "0.0.0.0/0"
}

variable "use_elastic_ips" {
  description = "Allocate Elastic IPs so node addresses survive stop/start."
  type        = bool
  default     = false
}
