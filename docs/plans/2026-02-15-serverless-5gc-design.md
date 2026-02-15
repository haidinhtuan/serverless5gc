# Serverless 5G Core - Design Document

**Date:** 2026-02-15
**Purpose:** Academic paper demonstrating cost efficiency of serverless 5G core vs traditional deployments
**Status:** Approved

## 1. Research Question

Can a serverless (Function-as-a-Service) deployment model reduce the operational cost of running a 5G core network compared to traditional containerized deployments, and under what traffic conditions does the cost advantage hold?

## 2. Architecture: Function-per-Procedure

Each 3GPP procedure maps to one OpenFaaS function. NF identity is logical - the "AMF" is a collection of functions sharing state in Redis.

```
┌─────────┐    SCTP     ┌────────────┐   HTTP    ┌──────────────────┐
│ UERANSIM │──────────>│ SCTP-HTTP  │────────>│   OpenFaaS        │
│ (gNB+UE) │           │   Proxy    │          │   Gateway         │
└─────────┘            └────────────┘          └────────┬───────────┘
                                                        │
                   ┌────────────────────────────────────┼──────────────────┐
                   │                                    │                  │
              ┌────▼────┐  ┌──────────┐  ┌─────────────▼──┐  ┌───────────▼─┐
              │AMF funcs│  │SMF funcs │  │UDM/UDR/AUSF   │  │NRF funcs    │
              │         │  │          │  │funcs           │  │             │
              │-register│  │-pdu-est  │  │-auth-vector    │  │-nf-register │
              │-deregist│  │-pdu-rel  │  │-sub-data       │  │-nf-discover │
              │-service │  │-pdu-mod  │  │-context        │  │-nf-status   │
              │-handover│  │          │  │                │  │             │
              └────┬────┘  └────┬─────┘  └──────┬────────┘  └─────────────┘
                   │            │               │
              ┌────▼────────────▼───────────────▼──────────┐
              │              State Store                    │
              │  Redis (session state) + etcd (NRF)        │
              └────────────────────────────────────────────┘

              ┌─────────────────────────────────────┐
              │ UPF (long-running container, go-upf) │
              │ Connected via N4/PFCP to SMF funcs   │
              └─────────────────────────────────────┘
```

### Key Design Decisions

- **Stateless functions**: All state externalized to Redis/etcd. Functions are pure request handlers.
- **SCTP-HTTP proxy**: Custom Go binary terminates SCTP from gNB, forwards NGAP as HTTP POST to OpenFaaS gateway.
- **UPF as container**: User plane (GTP-U packet forwarding) is fundamentally incompatible with FaaS. This is itself a paper finding.
- **NRF in etcd**: Service discovery stored in etcd; NRF functions are thin wrappers over etcd operations.

## 3. NF Function Inventory (~20 functions)

### AMF Functions
| Function | 3GPP Reference | Trigger |
|----------|---------------|---------|
| `amf-initial-registration` | TS 23.502 4.2.2.2 | Initial UE Message (NGAP) |
| `amf-deregistration` | TS 23.502 4.2.2.3 | Deregistration Request |
| `amf-service-request` | TS 23.502 4.2.3 | Service Request (NAS) |
| `amf-pdu-session-relay` | TS 23.502 4.3.2 | PDU Session Establishment relay |
| `amf-n2-handover` | TS 23.502 4.9.1 | Handover Required (NGAP) |
| `amf-auth-initiate` | TS 33.501 6.1.3 | Part of registration |

### SMF Functions
| Function | 3GPP Reference | Trigger |
|----------|---------------|---------|
| `smf-pdu-session-create` | TS 23.502 4.3.2 | Nsmf_PDUSession_CreateSMContext |
| `smf-pdu-session-update` | TS 23.502 4.3.3 | Nsmf_PDUSession_UpdateSMContext |
| `smf-pdu-session-release` | TS 23.502 4.3.4 | Nsmf_PDUSession_ReleaseSMContext |
| `smf-n4-session-setup` | PFCP Session Est. | Internal (post PDU create) |

### UDM/UDR/AUSF Functions
| Function | Purpose |
|----------|---------|
| `udm-generate-auth-data` | Generate 5G-AKA auth vectors |
| `udm-get-subscriber-data` | Return subscription/slice data |
| `udr-data-read` | Read subscriber records |
| `udr-data-write` | Write/update subscriber records |
| `ausf-authenticate` | 5G-AKA authentication |

### NRF Functions
| Function | Purpose |
|----------|---------|
| `nrf-register` | Register NF instance in etcd |
| `nrf-discover` | Discover NFs by type/service |
| `nrf-status-notify` | NF status change notifications |

### PCF / NSSF Functions
| Function | Purpose |
|----------|---------|
| `pcf-policy-create` | Create SM policy for PDU session |
| `pcf-policy-get` | Get policy rules |
| `nssf-slice-select` | Network slice selection |

## 4. Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.22+ | Fast cold starts, networking, free5GC compatibility |
| FaaS Platform | OpenFaaS (on K3s) | Self-hosted, reproducible, built-in metrics |
| Orchestration | K3s | Lightweight Kubernetes |
| Session State | Redis 7 | Sub-ms latency for UE contexts |
| NRF Registry | etcd | Consistent KV for NF discovery |
| UPF | go-upf (free5GC) | Go-based, PFCP, GTP-U |
| SCTP Proxy | Custom Go binary | SCTP termination, NGAP-to-HTTP |
| RAN Simulator | UERANSIM | Industry-standard gNB/UE simulator |
| IaC | Docker Compose + Helm | Reproducible deployment |
| Monitoring | Prometheus + Grafana + cAdvisor | Resource metrics and visualization |
| Infrastructure | IONOS Cloud VMs (via ionosctl) | Reproducible cloud environment |

## 5. Evaluation Plan

### Baselines
- **Open5GS** (C, containerized) - traditional VM/container deployment
- **free5GC** (Go, containerized) - microservices-based, same language

### Test Environment (IONOS Cloud)

| VM | Role | Spec |
|----|------|------|
| VM-1 | Serverless 5GC (OpenFaaS + K3s + Redis + etcd + proxy + UPF) | 8 vCPU, 16GB RAM |
| VM-2 | Open5GS baseline (Docker Compose) | 8 vCPU, 16GB RAM |
| VM-3 | free5GC baseline (Docker Compose) | 8 vCPU, 16GB RAM |
| VM-4 | Load generator (UERANSIM + scripts) | 4 vCPU, 8GB RAM |
| VM-5 | Monitoring (Prometheus + Grafana + cAdvisor) | 4 vCPU, 8GB RAM |

### Traffic Scenarios

| Scenario | UEs | Registration Rate | PDU Sessions | Duration |
|----------|-----|-------------------|-------------|----------|
| Low (IoT/rural) | 100 | 1 reg/sec | 50 active | 30 min |
| Medium (suburban) | 1,000 | 10 reg/sec | 500 active | 30 min |
| High (stadium) | 10,000 | 100 reg/sec | 5,000 active | 30 min |
| Idle (scale-to-zero) | 0 | 0 | 0 | 30 min |
| Burst | 0 to 5,000 | 0 to 50 reg/sec spike | Ramp up | 30 min |

Each scenario runs 3 times per system for statistical significance.

### Metrics

| Category | Metrics |
|----------|---------|
| Cost | CPU-seconds, memory-MB-seconds, projected cloud cost |
| Latency | Registration e2e time, PDU setup time, per-function duration |
| Scalability | Max concurrent UEs, throughput, cold start time |
| Resource utilization | CPU%, memory%, network I/O, disk I/O |
| Reliability | Success rate, error counts, timeout counts |

### Cost Model

```
Serverless Cost = SUM(function_invocations x avg_duration_s x memory_mb) x price_per_gb_second
Traditional Cost = SUM(container_cpu_reserved x uptime_hours + container_mem_reserved x uptime_hours) x price_per_unit
```

Uses published AWS Lambda/Fargate pricing applied to measured utilization for projected cost comparison.

## 6. Expected Findings

1. **Cost crossover point**: UE count at which serverless becomes more/less expensive
2. **Idle cost advantage**: Serverless at zero load vs baselines with idle containers
3. **Burst handling**: Serverless auto-scaling vs pre-provisioned baselines
4. **UPF unsuitability**: Data plane incompatible with FaaS, backed by data
5. **Cold start impact**: Latency overhead on registration procedure

## 7. Target Paper Structure

1. Introduction & Motivation
2. Background (5G Core Architecture, Serverless Computing)
3. Related Work
4. System Design (Function-per-Procedure architecture)
5. Implementation
6. Evaluation Methodology
7. Results & Analysis
8. Discussion (limitations, UPF finding, future work)
9. Conclusion

## 8. Project Structure

```
serverless5gc/
├── cmd/
│   └── sctp-proxy/          # SCTP-HTTP proxy binary
├── functions/
│   ├── amf/                 # AMF function handlers
│   │   ├── registration/
│   │   ├── deregistration/
│   │   ├── service-request/
│   │   ├── pdu-session-relay/
│   │   ├── handover/
│   │   └── auth-initiate/
│   ├── smf/                 # SMF function handlers
│   │   ├── pdu-session-create/
│   │   ├── pdu-session-update/
│   │   ├── pdu-session-release/
│   │   └── n4-session-setup/
│   ├── udm/                 # UDM functions
│   ├── udr/                 # UDR functions
│   ├── ausf/                # AUSF functions
│   ├── nrf/                 # NRF functions
│   ├── pcf/                 # PCF functions
│   └── nssf/                # NSSF functions
├── pkg/
│   ├── nas/                 # NAS protocol encoding/decoding
│   ├── ngap/                # NGAP protocol handling
│   ├── pfcp/                # PFCP protocol (N4 interface)
│   ├── sbi/                 # 5G SBI HTTP/2 client
│   ├── state/               # Redis/etcd state management
│   └── models/              # 3GPP data models
├── deploy/
│   ├── openfaas/            # OpenFaaS function YAML
│   ├── k3s/                 # K3s manifests
│   ├── baselines/
│   │   ├── open5gs/         # Open5GS Docker Compose
│   │   └── free5gc/         # free5GC Docker Compose
│   ├── monitoring/          # Prometheus + Grafana configs
│   └── ionos/               # IONOS VM provisioning scripts
├── eval/
│   ├── scenarios/           # Traffic scenario configs
│   ├── scripts/             # Load generation and data collection
│   └── analysis/            # Data analysis and chart generation
├── docs/
│   └── plans/               # Design and planning docs
├── go.mod
├── go.sum
└── Makefile
```
