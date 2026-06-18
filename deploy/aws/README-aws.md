# AWS EC2 Deployment

Terraform that replicates the IONOS `meridian-lab` topology on AWS EC2 for the
serverless 5GC evaluation. Two instances mirror the IONOS nodes:

| Node | Role | Type | Disk |
|------|------|------|------|
| node-a | serverless 5GC (K3s, OpenFaaS CE, Redis, etcd, sctp-proxy, monitoring) | `c5.xlarge` (4 vCPU / 8 GB) | 50 GB gp3 |
| node-b | loadgen (UERANSIM) | `c5.xlarge` (4 vCPU / 8 GB) | 40 GB gp3 |

Only provisioning differs from IONOS. Once the instances are up, the existing
`setup-serverless.sh`, `setup-loadgen.sh`, the 12-function `faas-cli` deploy, and
`run-eval.sh` are reused unchanged — Terraform emits the IPs in the same
`SERVERLESS_IP` / `LOADGEN_IP` format those scripts expect.

The instances run **Ubuntu 22.04 LTS** on purpose: UERANSIM 3.2.6 does not build
on Ubuntu 24.04.

## 1. One-time account setup

Do not run workloads in the organization management account. Create or use a
dedicated member account, then configure a local CLI profile.

```bash
# In the member account, create an IAM user (or use IAM Identity Center / SSO)
# with EC2 + VPC permissions, then:
aws configure --profile serverless5gc
#   AWS Access Key ID:     <member-account key>
#   AWS Secret Access Key: <member-account secret>
#   Default region:        eu-central-1
#   Default output:        json

# Verify you are in the right account (NOT 064021255155, the management account):
aws sts get-caller-identity --profile serverless5gc
```

Install Terraform (>= 1.5): https://developer.hashicorp.com/terraform/install

## 2. Configure

```bash
cd deploy/aws
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars: set admin_cidr to "$(curl -s ifconfig.me)/32"
```

## 3. Provision

```bash
terraform init
terraform plan
terraform apply
```

Write the env file the other scripts consume:

```bash
./gen-env-from-tf.sh
source vm-ips-aws.env
```

## 4. Configure the nodes (reuses existing scripts)

```bash
# from repo root, with vm-ips-aws.env sourced
ssh ubuntu@"$SERVERLESS_IP" 'sudo bash -s' < deploy/ionos/setup-serverless.sh
ssh ubuntu@"$LOADGEN_IP"    'sudo bash -s' < deploy/ionos/setup-loadgen.sh

# deploy the 12 evaluated functions (public Docker Hub images).
# deploy-eval-subset.sh / smoke-eval-subset.sh SSH as root@; on AWS the user is
# ubuntu and kubectl needs sudo, so run the equivalent steps directly:
scp deploy/openfaas/stack-eval.yml ubuntu@"$SERVERLESS_IP":/tmp/stack-eval.yml
ssh ubuntu@"$SERVERLESS_IP" 'bash -s' <<'EOF'
set -e
sudo k3s kubectl rollout restart deploy/gateway -n openfaas
sudo k3s kubectl rollout status  deploy/gateway -n openfaas --timeout=120s
# wait for the gateway endpoint to actually answer before deploying
until curl -sf http://127.0.0.1:31112/healthz >/dev/null; do sleep 2; done
export OPENFAAS_URL=http://127.0.0.1:31112
faas-cli deploy -f /tmp/stack-eval.yml
sudo k3s kubectl wait --for=condition=available deploy -n openfaas-fn -l faas_function --timeout=300s
faas-cli list
EOF
```

Note: AWS Ubuntu AMIs log in as `ubuntu` (with sudo), whereas IONOS used `root`.
The `setup-*.sh` scripts run fine under `sudo bash -s`. The OpenFaaS CE gateway
must be restarted (count-cache) and then allowed to become healthy before
`faas-cli deploy`, or the first deploy fails with `connection refused` on :31112.

## 5. Teardown

```bash
cd deploy/aws
terraform destroy
```

## Security group ports

| Port | Proto | Purpose |
|------|-------|---------|
| 22 | TCP | SSH (admin CIDR) |
| 31112 | TCP | OpenFaaS gateway NodePort |
| 38412 | SCTP | N2 / NGAP (sctp-proxy) |
| 8081 | TCP | cAdvisor |
| 9100 | TCP | node-exporter |
| 9090 | TCP | Prometheus |
| 30175 | TCP | OpenFaaS Prometheus NodePort |
| all | all | intra-VPC (K3s, Redis, etcd) |

SCTP works instance-to-instance inside the VPC (security groups support IP
protocol 132). The topology is direct node-b -> node-a, so no AWS load balancer
is involved (NLB does not support SCTP).
