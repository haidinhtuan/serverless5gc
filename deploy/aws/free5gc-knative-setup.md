# free5gc-knative (Core B) — AWS Setup Runbook

Step-by-step record of standing up the student's `free5gc-openfaas` (Knative)
stack on AWS, so it can be recreated. Companion to the comparison eval design
(`docs/plans/2026-06-18-aws-comparison-eval-design.md`).

**Node:** `free5gc-knative` — c5.2xlarge (8 vCPU / 16 GB), 60 GB gp3, Ubuntu 22.04,
eu-central-1a. Provisioned by `deploy/aws` Terraform with `deploy_core_b = true`.
Public IP at time of writing: `18.185.3.181`, private `10.0.1.65`. Login `ubuntu` (sudo).

> The student's repos are cloned into `comparison-repos/` (gitignored). The
> `free5gc-openfaas` repo's `free5gc-helm` submodule must be initialized
> (`git submodule update --init`) — it pins the Knative variant commit
> `c50db3a` where NRF/AUSF/UDM are disabled in the chart.

---

## Step 0 — Provision the node (local)
```bash
cd deploy/aws
# terraform.tfvars contains: deploy_core_b = true
terraform apply -auto-approve
# instance tagged Name=free5gc-knative
```

## Step 1 — K3s + Helm + gtp5g (on the node, as root)
Kernel: `6.8.0-1057-aws`.

```bash
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y build-essential linux-headers-$(uname -r) git make gcc

# GOTCHA: the AWS kernel was built with gcc-12, but build-essential pulls gcc-11.
# gtp5g build fails with "gcc-12: not found". Install gcc-12 and build with it.
apt-get install -y gcc-12

# gtp5g kernel module (free5GC UPF dependency)
cd /usr/src && git clone https://github.com/free5gc/gtp5g.git
cd gtp5g && make CC=gcc-12 && make install
modprobe gtp5g
lsmod | grep gtp5g     # -> gtp5g ... udp_tunnel

# K3s single-node (no traefik), Helm
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml   # root; kubectl is k3s's
```
Result: gtp5g loaded, K3s v1.35.5 Ready, Helm v3.21.1.

## Step 2 — Copy repo + install Knative   (DONE)
```bash
# local -> node (exclude .git to save space)
rsync -az --exclude='.git' comparison-repos/free5gc-openfaas/ ubuntu@<ip>:~/free5gc-openfaas/
# on node, as root with KUBECONFIG:
sudo KUBECONFIG=/etc/rancher/k3s/k3s.yaml bash ~/free5gc-openfaas/scripts/install_knative.sh
```
Result: Knative Serving v1.22 + net-kourier v1.22 Running (activator/autoscaler/controller/webhook/kourier).

## Step 3 — Multus CNI + ipvlan plugin + masterIf   (DONE — the hard Minikube->AWS port)
Three K3s-specific fixes:

1. **Retarget masterIf** in the chart (AWS NIC is `ens5`, not `eth0`; no `eth1`).
   Put every network (incl. N6) on `ens5` — the 10.100.50.x / 172.16.x subnets are
   internal ipvlan overlays used only for on-node pod-to-pod, so no 2nd ENI needed:
   ```bash
   sed -i 's/masterIf: eth0/masterIf: ens5/; s/masterIf: eth1/masterIf: ens5/' \
     ~/free5gc-openfaas/free5gc-helm/charts/free5gc/values.yaml
   ```
2. **Install upstream CNI plugins** — K3s ships only host-local/bridge/flannel, NOT
   ipvlan/macvlan that the NADs need. Extract into the RESOLVED k3s bin dir:
   ```bash
   BIN=$(readlink -f /var/lib/rancher/k3s/data/current/bin)
   curl -sLO https://github.com/containernetworking/plugins/releases/download/v1.5.1/cni-plugins-linux-amd64-v1.5.1.tgz
   sudo tar -xzf cni-plugins-linux-amd64-v1.5.1.tgz -C "$BIN" ./ipvlan ./macvlan ./static ./host-local ./loopback
   ```
3. **Install Multus**, patched for K3s paths. GOTCHA: use the RESOLVED bin dir
   (`readlink -f`), NOT the `data/current` symlink — a hostPath mount can't follow it.
   Only patch the `path:` (hostPath) lines, not the `/host/...` mountPaths:
   ```bash
   BIN=$(readlink -f /var/lib/rancher/k3s/data/current/bin)
   CONF=/var/lib/rancher/k3s/agent/etc/cni/net.d
   curl -sLO https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml
   sed -i "s#path: /opt/cni/bin#path: ${BIN}#; s#path: /etc/cni/net.d#path: ${CONF}#" multus-daemonset.yml
   sudo k3s kubectl apply -f multus-daemonset.yml
   ```
Result: `kube-multus-ds` Running 1/1.

## Step 4 — Deploy via reset.sh   (DONE)
```bash
# GOTCHA: reset.sh runs helm under sudo (root); add the bitnami repo AS ROOT,
# not as the ubuntu user, or redis install fails "repo bitnami not found".
sudo helm repo add bitnami https://charts.bitnami.com/bitnami && sudo helm repo update
sudo KUBECONFIG=/etc/rancher/k3s/k3s.yaml bash ~/free5gc-openfaas/scripts/reset.sh --knative 5
```
Deploys: Redis (bitnami), the 3 Knative NF services, the free5gc-helm chart
(AMF/SMF/UPF×3 ulcl/UDR/PCF/NSSF/NEF/CHF/WebUI/Mongo), bootstrap NF-registration
Job, and N subscribers. Result: RESET_EXIT=0, 16 pods Running, 3 ksvc Ready.

## Step 5 — Verify   (DONE at deployment level)
```bash
sudo k3s kubectl get ksvc -n free5gc                 # ausf/nrf/udm READY=True
sudo k3s kubectl get pods -n free5gc                 # 16 Running + bootstrap Completed
sudo k3s kubectl get svc -n free5gc | grep amf-n2    # NodePort 31412/SCTP (N2 entry)
# NRF registry (Knative AUSF/UDM present => bootstrap worked):
sudo k3s kubectl exec mongodb-0 -n free5gc -- mongosh free5gc --quiet \
  --eval 'db.NfProfile.find({},{nfType:1,_id:0}).toArray()'
```
**N2 endpoint for the load generator:** `<free5gc-knative public IP>:31412` (SCTP).

### End-to-end (UERANSIM from loadgen, 2026-06-18)
Drove the already-built UERANSIM on `loadgen` against free5gc-knative's AMF
(`<priv ip>:31412` SCTP, PLMN 208/93, subscriber `imsi-208930000000001`).

- ✅ **Registration: SUCCESS** — NG Setup → Auth → Security Mode → Registration
  Accept → MM-REGISTERED. This exercises the student's **Knative AUSF/UDM + NRF
  discovery**, so the serverless implementation is verified end-to-end.
  (Observed ~5 s gap before Auth Request = likely Knative cold-start of AUSF/UDM.)
- ❌ **PDU session: FAILS** with `SM forwarding failure ... PAYLOAD_NOT_FORWARDED`.
  Root cause: AMF→NSSF slice selection returns **HTTP 400** because the AMF sends an
  **empty `nf-id`** (`/nnssf-nsselection/...?nf-id=&nf-type=AMF&...`) and free5GC's
  NSSF binds `NfId` as `required`. This is an **upstream free5GC v4.2.2 AMF/NSSF
  defect**, NOT the student's serverless code and NOT a config issue:
  - The SMF is healthy (NRF-registered, PFCP associations with all 3 UPFs UP — so the
    ipvlan-on-ens5 N4 path works).
  - The AMF *has* a valid NfId in NRF (`2394…`) yet sends it empty to the NSSF.
  - Tried & ruled out: rewrote NSSF `nssfcfg` to the correct PLMN/TAI/slice
    (208/93, tac 000001, sd010203, real AMF NfId) + restarted NSSF + restarted AMF
    → identical 400. The 400 is a gin binding failure BEFORE any TAI lookup.
  - Aligns with free5GC issues github.com/free5gc/free5gc#913 / #1025.
  - Fixing PDU would require **patching free5GC** (AMF to populate nf-id, or NSSF to
    relax the binding) and rebuilding those images — an upstream change.

**Bottom line (initial):** registration worked; PDU was blocked. Root cause traced to
the **Knative scale-to-zero NRF** breaking free5GC's AMF↔NRF identity bootstrap (AMF
registration response lacked a parseable `Location`, so the AMF overwrote its own NfId
with empty → NSSF 400 + SMF "N1N2 client is nil"). Trigger = the student's serverless
NRF, exposing upstream free5GC strictness.

### PDU FIX (2026-06-18) — now fully working
Two upstream-free5GC patches (forked to GitHub, images on Docker Hub `haidinhtuan/`):
1. **NSSF** `github.com/haidinhtuan/free5gc-nssf` — `internal/sbi/processor/nsselection_network_slice_information.go`:
   `NfId ... binding:"required,uuid"` → `binding:"omitempty,uuid"`.
   Image `haidinhtuan/nssf:v4.2.2-nfidfix` (FROM free5gc/nssf:v4.2.2, replace /free5gc/nssf binary).
2. **AMF** `github.com/haidinhtuan/free5gc-amf` — `pkg/service/init.go`: only adopt the
   NRF-returned nfId when non-empty (`} else if nfId != "" {`).
   Image `haidinhtuan/amf:v4.2.2-nfidfix` (FROM free5gc/amf:v4.2.2, replace /free5gc/amf binary).

Build pattern (per NF): `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o <nf> ./cmd`
then `FROM free5gc/<nf>:v4.2.2` + `COPY <nf> /free5gc/<nf>`; push; `kubectl set image`.

Supporting config/ops (all required):
- Rewrote NSSF `nssfcfg` to the real PLMN/TAI/slice (208/93, tac 000001, sd010203) +
  the real AMF NfId (see Step 4 configmap swap).
- Pinned Knative NRF `min-scale=1` (`kubectl patch ksvc nrf-nnrf ... autoscaling.knative.dev/min-scale:"1"`)
  so the AMF `wait-nrf` init + registration are reliable.
- AMF deploy strategy → `Recreate` (the chart pins a fixed N2 ipvlan IP 10.100.50.249;
  RollingUpdate fails with `address already in use`). Delete old AMF pod to free the IP.

**VERIFIED end-to-end:** registration + `PDU Session establishment is successful PSI[1]`
+ `uesimtun0` data-plane interface up; SMF processing URR usage reports. The student's
NF-as-a-Function 5GC now completes a full attach on AWS.

---

## Gotchas log
- AWS kernel compiler is **gcc-12**; must `apt-get install gcc-12` or gtp5g build fails.
- AWS primary NIC is **ens5** (not eth0); chart masterIf must be retargeted.
- K3s lacks **ipvlan/macvlan** CNI plugins; install upstream containernetworking plugins.
- Multus hostPath must use the **resolved** k3s bin dir, not the `data/current` symlink.
- K3s's REAL CNI bin dir is **`/var/lib/rancher/k3s/data/cni`** (symlinks to its multicall
  `cni` binary). Multus + ipvlan/macvlan must live there, or new pods fail with
  `failed to find plugin "multus"`. Patch the Multus daemonset `cnibin` hostPath to
  `/var/lib/rancher/k3s/data/cni` and `cp ipvlan macvlan static` into it.
- The generated `00-multus.conflist` hardcodes `kubeconfig: /etc/cni/net.d/multus.d/...`,
  but K3s keeps it under `/var/lib/rancher/k3s/agent/etc/cni/net.d`. Symlink the default
  path so it resolves:
  `sudo mkdir -p /etc/cni && sudo ln -sfn /var/lib/rancher/k3s/agent/etc/cni/net.d /etc/cni/net.d`
  Verify: a fresh pod gets `AddedInterface ... from cbr0` and an IP.
