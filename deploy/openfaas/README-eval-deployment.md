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

## Historical note: fn-router

An earlier iteration deployed **all 31** functions and bypassed the CE
invocation limit with `fn-router`, an nginx reverse proxy. That approach was
retired in favour of the 12-function subset above, which runs on unmodified
OpenFaaS CE. See `archive/fn-router-retired.md`.
