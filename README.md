# Serverless5GC

A serverless 5G core network implementation using Function-per-Procedure decomposition on OpenFaaS. Serverless5GC maps 31 individual 3GPP procedures across 12 network functions to independent serverless functions, enabling fine-grained scale-to-zero and pay-per-invocation cost efficiency.

## Research Question

> Can a serverless (Function-as-a-Service) deployment model reduce the operational cost of running a 5G core network compared to traditional containerized deployments, while maintaining equivalent control plane performance?

## Architecture

Serverless5GC follows a **Function-per-Procedure** model: each 3GPP procedure (e.g., UE Registration, PDU Session Establishment) maps to one OpenFaaS function. NF identity is logical -- the "AMF" is a collection of functions sharing state in Redis.

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
                                                                    │  ...31 functions │
                                                                    └────────┬─────────┘
                                                                             │
                           ┌─────────────────────────────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │      State Stores       │
              │  Redis: UE/PDU context  │
              │  etcd: NRF registry     │
              └─────────────────────────┘
```

### Key Design Decisions

- **Stateless functions** -- all UE context, PDU session state, and security parameters are externalized to Redis. Functions are pure request handlers with no in-memory state between invocations.
- **SCTP-HTTP proxy** -- a custom Go binary terminates SCTP from the gNB (TS 38.412), decodes the NGAP header to determine the procedure, and forwards the payload as an HTTP POST to the corresponding OpenFaaS function.
- **NRF in etcd** -- NF service discovery is stored in etcd with prefix-based queries; NRF functions are thin wrappers over etcd CRUD operations.

## 5G Network Functions

12 NFs decomposed into 31 OpenFaaS functions:

| NF | Functions | 3GPP Reference |
|----|-----------|----------------|
| **AMF** | `amf-initial-registration`, `amf-deregistration`, `amf-service-request`, `amf-pdu-session-relay`, `amf-handover`, `amf-auth-initiate` | TS 23.502 |
| **SMF** | `smf-pdu-session-create`, `smf-pdu-session-update`, `smf-pdu-session-release`, `smf-n4-session-setup` | TS 23.502 |
| **UDM** | `udm-generate-auth-data`, `udm-get-subscriber-data` | TS 29.503 |
| **UDR** | `udr-data-read`, `udr-data-write` | TS 29.504 |
| **AUSF** | `ausf-authenticate` | TS 29.509 |
| **NRF** | `nrf-register`, `nrf-discover`, `nrf-status-notify` | TS 29.510 |
| **PCF** | `pcf-policy-create`, `pcf-policy-get` | TS 29.512 |
| **NSSF** | `nssf-slice-select` | TS 29.531 |
| **NWDAF** | `nwdaf-analytics-subscribe`, `nwdaf-data-collect` | TS 29.520 |
| **CHF** | `chf-charging-create`, `chf-charging-update`, `chf-charging-release` | TS 32.291 |
| **NSACF** | `nsacf-slice-availability-check`, `nsacf-update-counters` | TS 29.536 |
| **BSF** | `bsf-binding-register`, `bsf-binding-discover`, `bsf-binding-deregister` | TS 29.521 |

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.22+ |
| FaaS Platform | [OpenFaaS](https://www.openfaas.com/) on [K3s](https://k3s.io/) |
| Session State | [Redis 7](https://redis.io/) |
| NRF Registry | [etcd 3.5](https://etcd.io/) |
| RAN Simulator | [UERANSIM](https://github.com/aligungr/UERANSIM) |
| Infrastructure | Self-hosted VMs (Frankfurt) |

## Evaluation Results

We evaluated Serverless5GC against Open5GS (C-based) and free5GC (Go-based) across 4 traffic scenarios on dedicated VMs (4 vCPU, 8 GB RAM). All results are 3-run averages.

### Registration Latency (ms)

| Scenario | Serverless5GC p50 | Open5GS p50 | free5GC p50 |
|----------|------------------:|------------:|------------:|
| Low (100 UEs) | 463 | 406 | 3,234 |
| Medium (500 UEs) | 406 | 410 | 3,257 |
| High (1000 UEs) | 522 | 606 | 2,893 |
| Burst (500 UEs) | 435 | 403 | 3,008 |

Serverless5GC achieves **latency parity with C-based Open5GS** and is **5--8x faster than Go-based free5GC**. Internal function execution (~16.5 ms for 8 invocations) accounts for only ~3.5% of end-to-end latency.

### Cold-Start Storm Resilience

We tested worst-case cold-start behavior by deleting all 31 function pods and simultaneously starting UE registrations:

| Scenario | Cold p50 | Warm p50 | Delta |
|----------|:--------:|:--------:|:-----:|
| Low (100 UEs) | 5,021 | 463 | +4,558 ms (11x) |
| Medium (500 UEs) | 688 | 406 | +282 ms |
| High (1000 UEs) | 694 | 522 | +172 ms |
| Burst (500 UEs) | 644 | 435 | +209 ms |

**100% success rate** across all cold-start runs. Zero NAS T3510 timer expirations. System converges to warm-start performance within 4--5 seconds.

### Cost Projection

| Model | Monthly Cost | Savings |
|-------|:-----------:|:-------:|
| Self-hosted VMs | ~$142/month | Baseline |
| FaaS pricing (AWS Lambda) | $13--21/month | **85--90%** |

Per-registration cost: **$0.0000016** ($1.60 per million registrations).

### Key Findings

1. **Latency parity**: 406--522 ms median matches Open5GS (403--606 ms), 5--8x faster than free5GC
2. **Perfect reliability**: 100% registration success across all scenarios (6,300/6,300)
3. **Linear scaling**: Per-function execution stable within +/-4.1% from 1 to 20 reg/s
4. **Cold-start resilience**: 100% success under worst-case cold-start storms, convergence in 4--5s
5. **Cost efficiency**: 85--90% projected savings under FaaS pricing model
6. **Stateless scaling**: Memory virtually independent of traffic (2,105--2,138 MB, +1.6%)

## Getting Started

### Build

```bash
# Run unit tests
make test

# Build the SCTP proxy
make build-proxy

# Build all 31 function images
bash deploy/openfaas/build-functions.sh
```

### Deploy on K3s + OpenFaaS

```bash
# 1. Install K3s
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# 2. Install OpenFaaS
helm repo add openfaas https://openfaas.github.io/faas-netes/
helm install openfaas openfaas/openfaas \
    --namespace openfaas --create-namespace \
    --set functionNamespace=openfaas-fn \
    --set generateBasicAuth=true --wait

# 3. Deploy infrastructure (Redis, etcd)
kubectl apply -f deploy/k3s/

# 4. Deploy functions
bash deploy/openfaas/build-functions.sh --push
faas-cli up -f deploy/openfaas/stack.yml

# 5. Start SCTP proxy
./cmd/sctp-proxy/sctp-proxy
```

### Run Evaluation

```bash
# Provision subscribers
bash eval/scripts/provision-subscribers.sh <serverless_ip> 1000

# Run a scenario
SERVERLESS_IP=<ip> LOADGEN_IP=<ip> bash eval/scripts/run-coldstart.sh <scenario> [run]

# Run full campaign (4 scenarios x 3 runs, adjust for your environment)
bash eval/scripts/run-campaign.sh
```

## Project Structure

```
serverless5gc/
├── cmd/sctp-proxy/          # SCTP-HTTP bridge binary
├── functions/               # 31 OpenFaaS function handlers
│   ├── amf/                 # 6 AMF procedures
│   ├── smf/                 # 4 SMF procedures
│   ├── udm/, udr/, ausf/   # Auth chain
│   ├── nrf/, pcf/, nssf/   # Supporting NFs
│   ├── nwdaf/, chf/        # R17: Analytics, Charging
│   ├── nsacf/, bsf/        # R17: Admission, Binding
├── pkg/                     # Shared libraries
│   ├── crypto/              # Milenage 5G-AKA (TS 35.208)
│   ├── models/              # 3GPP data types
│   ├── nas/                 # NAS codec (TS 24.501)
│   ├── ngap/                # NGAP decoder (TS 38.413)
│   ├── state/               # KVStore (Redis/etcd)
├── deploy/
│   ├── openfaas/            # Function definitions + Dockerfile
│   ├── k3s/                 # Infrastructure manifests
│   └── baselines/           # Open5GS + free5GC configs
├── eval/
│   ├── scenarios/           # Traffic scenario definitions
│   ├── scripts/             # Evaluation automation
│   └── results/             # Raw experiment data
└── test/integration/        # Integration tests
```

## References

- 3GPP TS 23.501/502 (5G System Architecture & Procedures)
- 3GPP TS 24.501 (NAS Protocol), TS 38.413 (NGAP)
- 3GPP TS 33.501 (Security), TS 35.208 (Milenage)
- [Open5GS](https://open5gs.org/), [free5GC](https://free5gc.org/), [UERANSIM](https://github.com/aligungr/UERANSIM)
- [OpenFaaS](https://www.openfaas.com/), [K3s](https://k3s.io/)

## License

This project is developed for academic research purposes.
