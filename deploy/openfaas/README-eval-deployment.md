# Evaluation deployment on OpenFaaS Community Edition

This document explains how the serverless 5GC is deployed for the evaluation
campaign and why it deploys a 12-function subset rather than all functions.

## The OpenFaaS CE 15-function limit

OpenFaaS Community Edition enforces a hard limit of **15 functions**. The limit
is checked in two places:

1. **Deploy time** — `faas-cli deploy` of a 16th function is rejected.
2. **Invocation time** — if more than 15 functions exist, the gateway refuses
   *all* invocations with `HTTP 405: "the OpenFaaS CE EULA only permits: 15
   functions"`. This is global, not per-function.

The full core implements 31 functions across 12 NFs, which exceeds the limit.

CE additionally **rejects deploying non-public images** via `faas-cli`
(`HTTP 400: "the Community Edition license agreement only allows public
images"`). We therefore publish the function images to a public Docker Hub
namespace (`haidinhtuan/<name>`) and deploy them with `faas-cli` the normal way.
(For an offline/private deployment, the images can instead be imported into
containerd and applied as raw K8s Deployments+Services via `gen-k8s-manifests.py`
+ `kubectl apply` — see "Offline / private fallback" below.)

## Why we deploy only 12 functions

The evaluation campaign (`deploy/ionos/run-eval.sh` → `eval/scripts/run-scenario.sh`)
drives exactly **two procedures** across all load levels (`idle`, `low`,
`medium`, `high`, `burst` are intensities, not different procedures):

- **UE registration** — `amf-initial-registration`
- **PDU session establishment** — `smf-pdu-session-create`

Tracing both call graphs, the union of functions actually invoked is **12**:

| Procedure | Functions |
|-----------|-----------|
| Registration | `amf-initial-registration`, `amf-auth-initiate`, `udm-generate-auth-data`, `ausf-authenticate`, `udm-get-subscriber-data` |
| PDU session | `smf-pdu-session-create`, `pcf-policy-create`, `bsf-binding-register`, `chf-charging-create` |
| Shared (slice admission) | `nsacf-slice-availability-check`, `nsacf-update-counters` |
| Provisioning (one-time) | `udr-data-write` |

12 ≤ 15, so the campaign runs on **stock OpenFaaS CE with no workaround**. The
other 19 functions remain implemented and unit/integration-tested; they are
simply not deployed for the load test.

### Call graphs

Functions invoke each other over SBI through the gateway (`pkg/sbi/client.go`,
`OPENFAAS_GATEWAY=http://gateway.openfaas:8080/function`). There is no NRF
discovery on the runtime path — calls address `/function/<name>` directly.

```
amf-initial-registration
├─ amf-auth-initiate
│  └─ udm-generate-auth-data        # 5G-AKA vectors; reads subscribers/<SUPI> from Redis
├─ ausf-authenticate                # verifies RES*
├─ udm-get-subscriber-data          # Nudm_SDM_Get (reads Redis)
├─ nsacf-slice-availability-check    # conditional on requested S-NSSAI
└─ nsacf-update-counters

smf-pdu-session-create
├─ pcf-policy-create                # SM policy
├─ nsacf-slice-availability-check
├─ bsf-binding-register
├─ chf-charging-create
└─ nsacf-update-counters
```

Notes:
- UDM reads subscriber data **directly from Redis** (`subscribers/<SUPI>`), not
  via the UDR functions. `udr-data-write` is used once before a run to provision
  subscribers.
- `amf-initial-registration` also fire-and-forgets a `udm-registration` call,
  but no such function exists in the stack; the ignored result makes it a no-op.

## Verification

`smoke-eval-subset.sh` provisions the test subscriber
(`imsi-001010000000001`, the vector from `test/integration/helpers_test.go`) and
exercises both procedures through the gateway. Expected:

```
provision subscriber : HTTP 201
registration         : HTTP 200   {"status":"registered", ...}
pdu session create   : HTTP 201   {"state":"ACTIVE", ...}
```

## Function runtime

Function images run under the genuine OpenFaaS **of-watchdog** runtime
(`deploy/openfaas/Dockerfile.template`). The watchdog (HTTP mode) is the
container entrypoint on `:8080`; it forks the compiled handler binary once and
reverse-proxies requests to it — the same execution model as the official
`go-http` template. We build via our own Dockerfile (rather than `faas-cli
build` with the stock template) because the handlers import shared packages from
this Go module; the custom build compiles the whole module while keeping the
of-watchdog runtime. Images are published to `haidinhtuan/<name>` on Docker Hub.

## Deploying

```bash
source deploy/ionos/vm-ips-meridian.env

# Build + push the 12 images, faas-cli deploy the subset, verify.
bash deploy/openfaas/deploy-eval-subset.sh "$SERVERLESS_IP" --build
bash deploy/openfaas/smoke-eval-subset.sh  "$SERVERLESS_IP"
```

`deploy-eval-subset.sh` (with `--build`) builds and pushes the 12 images to the
public registry, then `faas-cli deploy`s `stack-eval.yml` and restarts the
gateway so it re-counts functions and clears any stale over-limit (405) state.
Without `--build` it just (re)deploys the already-published images. The registry
defaults to `haidinhtuan`; override with `REGISTRY=<namespace>`.

### Offline / private fallback (no public registry)

If you cannot publish images, deploy them as raw K8s manifests instead — the
gateway still routes to and counts them:

```bash
bash deploy/openfaas/build-functions.sh <12 names...>          # local images
docker save ... | ssh root@VM 'k3s ctr images import -'        # into containerd
python3 deploy/openfaas/gen-k8s-manifests.py deploy/openfaas/stack-eval.yml \
    | ssh root@VM 'kubectl apply -f -'
ssh root@VM 'kubectl rollout restart deploy/gateway -n openfaas'
```

`gen-k8s-manifests.py` emits `imagePullPolicy: Never` Deployments+Services
labelled `faas_function: <name>`.

## Building a different subset

`build-functions.sh` accepts an optional list of function names:

```bash
bash deploy/openfaas/build-functions.sh amf-initial-registration ausf-authenticate
```

With no arguments it builds all functions (see `stack.yml`).

## Design history

The deployment approach evolved as the OpenFaaS CE restrictions surfaced:

1. **All 31 via raw manifests.** To get past the CE *deploy-time* 15-function
   cap and the public-image rule, all 31 functions were applied as plain K8s
   Deployments+Services (`gen-k8s-manifests.py` + `kubectl apply`). This worked
   for deployment but the gateway then blocked *every* invocation with HTTP 405
   (the invocation-time cap).
2. **fn-router.** An nginx reverse proxy (`archive/fn-router-retired.md`) was
   added to route `/function/<name>` straight to each Service, bypassing the
   gateway's invocation cap. Combined with a bare `net/http` container (no
   watchdog), this effectively stopped using OpenFaaS on the request path —
   only the handler SDK types remained.
3. **12-function subset (current).** Recognising that the campaign exercises
   only registration + PDU-session establishment (12 functions ≤ 15), we dropped
   the subset to within the CE limit, retired fn-router, and returned to the
   stock OpenFaaS gateway.
4. **of-watchdog runtime + public images (current).** The function images were
   rebuilt to run under the genuine of-watchdog runtime, published to public
   Docker Hub, and deployed with `faas-cli` — making the deployment stock
   OpenFaaS CE end to end (deploy, runtime, and gateway), with no workaround.

The net effect: the system implements the full 31-function core, but the
*evaluation* runs the 12-function workload subset on an unmodified OpenFaaS CE.
