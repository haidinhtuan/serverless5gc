# Serverless 5G Core (serverless5gc)

A Function-as-a-Service (FaaS) implementation of a 3GPP-compliant 5G core network using OpenFaaS, designed for academic evaluation of serverless cost efficiency against traditional containerized 5G core deployments.

## Research Question

> Can a serverless (Function-as-a-Service) deployment model reduce the operational cost of running a 5G core network compared to traditional containerized deployments, and under what traffic conditions does the cost advantage hold?

## Architecture

The system follows a **Function-per-Procedure** model: each 3GPP procedure (e.g., UE Registration, PDU Session Establishment) maps to one OpenFaaS function. NF identity is logical -- the "AMF" is a collection of functions sharing state in Redis.

```
                                                                    ┌──────────────────┐
┌──────────┐     SCTP      ┌────────────┐    HTTP    ┌─────────┐   │  Function Pool   │
│ UERANSIM │──────────────>│ SCTP-HTTP  │──────────>│ OpenFaaS │──>│                  │
│ (gNB+UE) │  (TS 38.412) │   Proxy    │  (NGAP    │ Gateway  │   │  amf-register    │
└──────────┘               └────────────┘   payload) └─────────┘   │  amf-deregister  │
                                                                    │  smf-pdu-create  │
                                                                    │  ausf-auth       │
                                                                    │  udm-auth-data   │
                                                                    │  nrf-discover    │
                                                                    │  ...20 functions │
                                                                    └────────┬─────────┘
                                                                             │
                           ┌─────────────────────────────────────────────────┘
                           │
              ┌────────────▼────────────┐     ┌─────────────────────────────┐
              │      State Stores       │     │    UPF (long-running pod)   │
              │  Redis: UE/PDU context  │     │    go-upf via PFCP (N4)    │
              │  etcd: NRF registry     │     │    GTP-U data plane        │
              └─────────────────────────┘     └─────────────────────────────┘
```

### Key Design Decisions

- **Stateless functions** -- all UE context, PDU session state, and security parameters are externalized to Redis. Functions are pure request handlers with no in-memory state between invocations.
- **SCTP-HTTP proxy** -- a custom Go binary terminates SCTP from the gNB (TS 38.412), decodes the NGAP header to determine the procedure, and forwards the payload as an HTTP POST to the corresponding OpenFaaS function.
- **UPF as container** -- the user plane (GTP-U packet forwarding) requires persistent kernel-level data paths incompatible with FaaS execution. This is itself a research finding.
- **NRF in etcd** -- NF service discovery is stored in etcd with prefix-based queries; NRF functions are thin wrappers over etcd CRUD operations.

## 5G Network Functions

The system implements 8 NFs decomposed into 21 OpenFaaS functions:

| NF | Functions | 3GPP Reference |
|----|-----------|----------------|
| **AMF** | `amf-initial-registration`, `amf-deregistration`, `amf-service-request`, `amf-pdu-session-relay`, `amf-n2-handover`, `amf-auth-initiate` | TS 23.502 4.2.2, 4.2.3, 4.9.1 |
| **SMF** | `smf-pdu-session-create`, `smf-pdu-session-update`, `smf-pdu-session-release`, `smf-n4-session-setup` | TS 23.502 4.3.2-4.3.4 |
| **UDM** | `udm-generate-auth-data`, `udm-get-subscriber-data` | TS 29.503 |
| **UDR** | `udr-data-read`, `udr-data-write` | TS 29.504 |
| **AUSF** | `ausf-authenticate` | TS 29.509, TS 33.501 |
| **NRF** | `nrf-register`, `nrf-discover`, `nrf-status-notify` | TS 29.510 |
| **PCF** | `pcf-policy-create`, `pcf-policy-get` | TS 29.512 |
| **NSSF** | `nssf-slice-select` | TS 29.531 |

### 3GPP Compliance

The implementation covers key protocol aspects for fair benchmarking:

- **NAS (TS 24.501)** -- Registration Request/Accept encoding, Security Mode Command/Complete, 5GMM cause codes
- **NGAP (TS 38.413)** -- APER header parsing, procedure code routing, IE extraction (RAN-UE-NGAP-ID, NAS-PDU, User Location Info)
- **PFCP (TS 29.244)** -- Session establishment/modification/release with QER and URR IEs, Association Setup
- **5G-AKA (TS 33.501)** -- Milenage-based authentication vector generation using 3GPP test vectors (TS 35.208)
- **SBI (TS 29.5xx)** -- 3GPP-compliant HTTP paths (e.g., `/nausf-auth/v1/ue-authentications`), RFC 7807 ProblemDetails error responses
- **UE State Machine (TS 23.502)** -- RM (DEREGISTERED/REGISTERED) and CM (IDLE/CONNECTED) state transitions with timestamps
- **3GPP Timers** -- T3510 (Registration), T3512 (Periodic Registration), T3580 (5GSM)

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.22+ | Fast cold starts, strong networking, free5GC library compatibility |
| FaaS Platform | [OpenFaaS](https://www.openfaas.com/) on [K3s](https://k3s.io/) | Self-hosted, reproducible, built-in Prometheus metrics |
| Session State | [Redis 7](https://redis.io/) | Sub-ms latency for UE context lookups |
| NRF Registry | [etcd](https://etcd.io/) | Strongly consistent KV store for NF discovery |
| User Plane | [go-upf](https://github.com/free5gc/go-upf) (free5GC) | Go-based UPF with PFCP and GTP-U support |
| SCTP Proxy | Custom Go binary | SCTP termination from gNB, NGAP routing to HTTP |
| RAN Simulator | [UERANSIM](https://github.com/aligungr/UERANSIM) | Industry-standard open-source gNB/UE simulator |
| Monitoring | [Prometheus](https://prometheus.io/) + [Grafana](https://grafana.com/) + [cAdvisor](https://github.com/google/cadvisor) | Resource and cost metric collection |
| Infrastructure | Cloud VMs (Debian/Ubuntu) | Reproducible VM provisioning |

### Go Dependencies

| Package | Purpose |
|---------|---------|
| [`github.com/free5gc/util`](https://github.com/free5gc/util) | Milenage algorithm, 3GPP crypto utilities |
| [`github.com/wmnsk/go-pfcp`](https://github.com/wmnsk/go-pfcp) | PFCP protocol implementation (N4 interface) |
| [`github.com/ishidawataru/sctp`](https://github.com/ishidawataru/sctp) | SCTP socket support for Go |
| [`github.com/openfaas/templates-sdk/go-http`](https://github.com/openfaas/golang-http-template) | OpenFaaS Go HTTP function template |
| [`github.com/redis/go-redis/v9`](https://github.com/redis/go-redis) | Redis client |
| [`go.etcd.io/etcd/client/v3`](https://github.com/etcd-io/etcd) | etcd client |
| [`github.com/prometheus/client_golang`](https://github.com/prometheus/client_golang) | Prometheus metric exporter |

## Project Structure

```
serverless5gc/
├── cmd/
│   └── sctp-proxy/              # SCTP-HTTP bridge binary
│       ├── main.go              # SCTP listener, CLI flags
│       └── proxy.go             # NGAP routing, HTTP forwarding
├── functions/                   # 21 OpenFaaS function handlers
│   ├── amf/
│   │   ├── registration/        # TS 23.502 4.2.2.2 Initial Registration
│   │   ├── deregistration/      # TS 23.502 4.2.2.3 Deregistration
│   │   ├── service-request/     # TS 23.502 4.2.3 Service Request
│   │   ├── pdu-session-relay/   # TS 23.502 4.3.2 PDU Session relay
│   │   ├── handover/            # TS 23.502 4.9.1 N2 Handover
│   │   └── auth-initiate/       # TS 33.501 6.1.3 Auth initiation
│   ├── smf/
│   │   ├── pdu-session-create/  # Nsmf_PDUSession_CreateSMContext
│   │   ├── pdu-session-update/  # Nsmf_PDUSession_UpdateSMContext
│   │   ├── pdu-session-release/ # Nsmf_PDUSession_ReleaseSMContext
│   │   └── n4-session-setup/    # PFCP session establishment to UPF
│   ├── udm/
│   │   ├── generate-auth-data/  # 5G-AKA auth vector generation
│   │   └── get-subscriber-data/ # Subscription data retrieval
│   ├── udr/
│   │   ├── data-read/           # Subscriber record read
│   │   └── data-write/          # Subscriber record write
│   ├── ausf/
│   │   └── authenticate/        # 5G-AKA verification
│   ├── nrf/
│   │   ├── register/            # NF profile registration (etcd)
│   │   ├── discover/            # NF discovery by type/service
│   │   └── status-notify/       # NF status change notification
│   ├── pcf/
│   │   ├── policy-create/       # SM policy creation (QoS per slice)
│   │   └── policy-get/          # Policy retrieval
│   └── nssf/
│       └── slice-select/        # Network slice selection (NSSAI)
├── pkg/
│   ├── crypto/                  # Milenage 5G-AKA (TS 35.208)
│   ├── models/                  # 3GPP data types (UEContext, PDUSession, etc.)
│   ├── nas/                     # NAS codec, cause codes, timers (TS 24.501)
│   ├── ngap/                    # NGAP decoder and IE extraction (TS 38.413)
│   ├── pfcp/                    # PFCP client with QER/URR (TS 29.244)
│   ├── sbi/                     # Inter-NF HTTP client via OpenFaaS gateway
│   ├── state/                   # KVStore interface + Redis/etcd/mock impls
│   └── statemachine/            # UE RM/CM state machine (TS 23.502)
├── deploy/
│   ├── openfaas/
│   │   ├── stack.yml            # All 21 function definitions
│   │   └── Dockerfile.template  # Multi-stage Go builder
│   ├── k3s/
│   │   ├── redis-deployment.yaml
│   │   ├── etcd-deployment.yaml
│   │   ├── sctp-proxy-deployment.yaml
│   │   └── upf-deployment.yaml
│   ├── baselines/
│   │   ├── open5gs/             # Full Open5GS 2.7.0 (13 services)
│   │   │   ├── docker-compose.yml
│   │   │   └── config/         # 12 NF config files (PLMN 001/01)
│   │   └── free5gc/             # Full free5GC v3.4.1 (10 services)
│   │       ├── docker-compose.yml
│   │       └── config/         # 9 NF config files
│   ├── monitoring/
│   │   ├── prometheus.yml
│   │   ├── grafana-dashboard.json
│   │   └── docker-compose.yml
│   └── cloud/
│       ├── provision.sh         # Full 5-VM provisioning
│       └── smoke-test.sh        # 2-VM smoke test with auto-teardown
├── eval/
│   ├── scenarios/
│   │   ├── low.yaml             # 100 UEs, 1 reg/sec
│   │   ├── medium.yaml          # 1,000 UEs, 10 reg/sec
│   │   ├── high.yaml            # 10,000 UEs, 100 reg/sec
│   │   ├── idle.yaml            # 0 UEs (scale-to-zero)
│   │   └── burst.yaml           # 0 to 5,000 UEs ramp
│   ├── scripts/
│   │   ├── run-scenario.sh      # Scenario execution orchestrator
│   │   └── cost-exporter/       # Prometheus cost metric exporter
│   │       └── main.go
│   └── analysis/
│       ├── analyze.py           # Metric analysis and cost projection
│       ├── charts.py            # Visualization generation
│       └── requirements.txt
├── go.mod
├── go.sum
└── Makefile
```

## Getting Started

### Prerequisites

- Go 1.22+
- Docker
- [K3s](https://k3s.io/) (or any Kubernetes distribution)
- [OpenFaaS CLI (`faas-cli`)](https://docs.openfaas.com/cli/install/)
- [Helm 3](https://helm.sh/docs/intro/install/)
- [UERANSIM](https://github.com/aligungr/UERANSIM) (for testing)
- Cloud VM provider with SSH access (for deployment)

### Build

```bash
# Run unit tests (178 tests across 27 packages)
make test

# Build the SCTP proxy
make build-proxy

# Lint
make lint
```

### Deploy on K3s + OpenFaaS

```bash
# 1. Install K3s
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# 2. Install Helm and OpenFaaS
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
kubectl create namespace openfaas
kubectl create namespace openfaas-fn
helm repo add openfaas https://openfaas.github.io/faas-netes/
helm install openfaas openfaas/openfaas \
    --namespace openfaas \
    --set functionNamespace=openfaas-fn \
    --set generateBasicAuth=true \
    --wait

# 3. Deploy infrastructure (Redis, etcd, UPF, SCTP proxy)
kubectl apply -f deploy/k3s/

# 4. Deploy functions
cd deploy/openfaas
faas-cli up -f stack.yml
```

### Deploy Baselines

```bash
# Open5GS (v2.7.0)
cd deploy/baselines/open5gs
docker compose up -d

# free5GC (v3.4.1)
cd deploy/baselines/free5gc
docker compose up -d
```

### Cloud Provisioning

```bash
# Full 5-VM deployment
./deploy/cloud/provision.sh

# Quick 2-VM smoke test (auto-teardown)
./deploy/cloud/smoke-test.sh
```

## Evaluation

### Running Scenarios

```bash
# Set environment
export MONITORING_IP=<prometheus-vm-ip>
export LOADGEN_IP=<ueransim-vm-ip>
export TARGET_AMF_IP=<target-amf-ip>

# Run a scenario
./eval/scripts/run-scenario.sh <scenario> <target> [run_number]
# scenario: idle | low | medium | high | burst
# target:   serverless | open5gs | free5gc
# run_number: 1 (default), run 3 times for statistical significance
```

### Traffic Scenarios

| Scenario | UEs | Registration Rate | PDU Sessions | Duration |
|----------|-----|-------------------|-------------|----------|
| Idle | 0 | 0 | 0 | 30 min |
| Low (IoT/rural) | 100 | 1 reg/sec | 50 active | 30 min |
| Medium (suburban) | 1,000 | 10 reg/sec | 500 active | 30 min |
| High (stadium) | 10,000 | 100 reg/sec | 5,000 active | 30 min |
| Burst | 0 to 5,000 | 0 to 50 reg/sec ramp | Variable | 30 min |

### Cost Model

The evaluation projects measured resource utilization onto public cloud pricing:

```
Serverless Cost = SUM(invocations x avg_duration_sec x memory_GB) x $0.0000166667/GB-sec
                + SUM(invocations) x $0.0000002/request

Traditional Cost = (vCPU_reserved x hours x $0.04048/vCPU-hr)
                 + (memory_GB_reserved x hours x $0.004445/GB-hr)
```

Pricing sourced from [AWS Lambda](https://aws.amazon.com/lambda/pricing/) and [AWS Fargate](https://aws.amazon.com/fargate/pricing/).

### Analysis

```bash
cd eval/analysis
pip install -r requirements.txt

# Generate cost comparison, latency analysis, and charts
python analyze.py
python charts.py
```

## Testing

The project has 178 unit tests across 27 test packages:

```bash
# Run all unit tests
make test

# Run tests with verbose output
go test ./... -v -count=1

# Run integration tests (requires Redis/etcd)
go test ./... -v -count=1 -tags=integration

# Run a specific package
go test ./pkg/nas/... -v
go test ./functions/amf/registration/... -v
```

### Test Coverage by Component

| Component | Tests | Coverage |
|-----------|-------|----------|
| NAS codec & cause codes | 15 | Message encode/decode, all 5GMM causes |
| NGAP decoder | 9 | Procedure routing, IE extraction |
| PFCP client | 8 | Session CRUD, QER/URR, Association |
| Milenage crypto | 4 | 3GPP TS 35.208 test vectors |
| SBI client | 6 | HTTP calls, error handling |
| Redis/etcd state | 6 | KVStore CRUD, mock store |
| UE state machine | 8 | RM/CM transitions, invalid states |
| SBI types | 3 | ProblemDetails, JSON serialization |
| AMF functions | 27 | Registration, deregistration, auth |
| SMF functions | 35 | PDU session lifecycle, PFCP setup |
| NRF functions | 7 | Register, discover, notify |
| Auth functions | 24 | UDR/UDM/AUSF chain, 5G-AKA |
| PCF/NSSF functions | 12 | Policy, slice selection |
| NAS timers | 5 | Timer values per TS 24.501 |
| 3GPP compliance | 9 | SBI paths, cause codes, state machine |

## Monitoring

Deploy the monitoring stack for real-time resource tracking:

```bash
cd deploy/monitoring
docker compose up -d
```

- **Prometheus**: `http://<monitoring-ip>:9090` -- scrapes OpenFaaS, cAdvisor, node metrics
- **Grafana**: `http://<monitoring-ip>:3000` -- pre-configured cost comparison dashboard

The custom cost exporter (`eval/scripts/cost-exporter/`) exposes projected Lambda/Fargate cost as Prometheus metrics based on real-time function invocation data.

## Expected Research Findings

1. **Cost crossover point** -- the UE count at which serverless becomes more expensive than traditional deployments
2. **Idle cost advantage** -- serverless at zero load (scale-to-zero) vs baselines with idle containers consuming resources
3. **Burst handling** -- serverless auto-scaling vs pre-provisioned baseline capacity
4. **UPF unsuitability** -- data plane (GTP-U forwarding) is fundamentally incompatible with FaaS execution, backed by empirical data
5. **Cold start impact** -- latency overhead on the first UE registration after idle period

## References

### 3GPP Specifications

| Spec | Title | Relevance |
|------|-------|-----------|
| [TS 23.501](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3144) | System Architecture for 5G System | Overall 5GC architecture, NF roles |
| [TS 23.502](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3145) | Procedures for 5G System | Registration, PDU session, handover procedures |
| [TS 24.501](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3370) | NAS Protocol for 5GS | NAS message formats, cause codes, timers |
| [TS 29.244](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3111) | PFCP Interface | N4 interface between SMF and UPF |
| [TS 29.500](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3338) | 5GC SBI Technical Realization | HTTP/2 based SBI framework |
| [TS 29.502](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3340) | SMF Services | Nsmf_PDUSession service operations |
| [TS 29.503](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3341) | UDM Services | Nudm_SDM, Nudm_UECM services |
| [TS 29.504](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3342) | UDR Services | Nudr_DataRepository service |
| [TS 29.509](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3347) | AUSF Services | Nausf_UEAuthentication service |
| [TS 29.510](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3348) | NRF Services | Nnrf_NFManagement, Nnrf_NFDiscovery |
| [TS 29.512](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3350) | PCF Services | Npcf_SMPolicyControl |
| [TS 29.531](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3369) | NSSF Services | Nnssf_NSSelection |
| [TS 33.501](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3169) | Security Architecture | 5G-AKA, NAS security |
| [TS 35.208](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=2276) | Milenage Algorithm | Authentication algorithm test vectors |
| [TS 38.412](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3228) | NG Signalling Transport | SCTP transport for NGAP |
| [TS 38.413](https://portal.3gpp.org/desktopmodules/Specifications/SpecificationDetails.aspx?specificationId=3223) | NGAP | NG Application Protocol procedures |

### Open-Source Projects

| Project | Role |
|---------|------|
| [OpenFaaS](https://github.com/openfaas/faas) | FaaS platform for deploying serverless 5GC functions |
| [K3s](https://github.com/k3s-io/k3s) | Lightweight Kubernetes distribution for OpenFaaS |
| [Open5GS](https://github.com/open5gs/open5gs) | C-based 5G core -- evaluation baseline |
| [free5GC](https://github.com/free5gc/free5gc) | Go-based 5G core -- evaluation baseline |
| [UERANSIM](https://github.com/aligungr/UERANSIM) | 5G UE and gNB simulator for load generation |
| [go-pfcp](https://github.com/wmnsk/go-pfcp) | Go PFCP protocol library for N4 interface |
| [go-upf](https://github.com/free5gc/go-upf) | Go-based UPF with GTP-U and PFCP support |

### Cloud and Pricing

| Resource | URL |
|----------|-----|
| AWS Lambda Pricing | https://aws.amazon.com/lambda/pricing/ |
| AWS Fargate Pricing | https://aws.amazon.com/fargate/pricing/ |

## License

This project is developed for academic research purposes.
