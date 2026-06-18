locals {
  node_a_ip = var.use_elastic_ips ? aws_eip.node_a[0].public_ip : aws_instance.node_a.public_ip
  node_b_ip = var.use_elastic_ips ? aws_eip.node_b[0].public_ip : aws_instance.node_b.public_ip
}

output "node_a_ip" {
  description = "Public IP of node-a (serverless 5GC)."
  value       = local.node_a_ip
}

output "node_b_ip" {
  description = "Public IP of node-b (loadgen)."
  value       = local.node_b_ip
}

output "node_a_private_ip" {
  value = aws_instance.node_a.private_ip
}

output "node_b_private_ip" {
  value = aws_instance.node_b.private_ip
}

output "core_b_ip" {
  description = "Public IP of core-b (free5gc-openfaas), or empty if not deployed."
  value       = var.deploy_core_b ? aws_instance.core_b[0].public_ip : ""
}

output "core_b_private_ip" {
  value = var.deploy_core_b ? aws_instance.core_b[0].private_ip : ""
}

# Ready-to-source env block in the same format the setup/eval scripts consume.
output "env_file" {
  description = "Contents for vm-ips-aws.env (run gen-env-from-tf.sh to write it)."
  value       = <<-EOT
    # AWS EC2 VM IPs - generated from terraform output
    NODE_A_IP="${local.node_a_ip}"
    NODE_B_IP="${local.node_b_ip}"

    # Aliases for compatibility with existing scripts
    SERVERLESS_IP="${local.node_a_ip}"
    LOADGEN_IP="${local.node_b_ip}"
  EOT
}
