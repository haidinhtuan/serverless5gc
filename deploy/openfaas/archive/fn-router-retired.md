# Retired: fn-router

`fn-router` was a workaround for the OpenFaaS Community Edition 15-function
limit, used while all 31 functions were deployed simultaneously. It has been
**retired** in favour of deploying the 12-function evaluation subset on stock
OpenFaaS CE (see `../README-eval-deployment.md`). This file documents it for the
record.

## What it was

An nginx reverse proxy (`nginx:1.27-alpine`) in the `openfaas-fn` namespace,
exposed as a `NodePort` on `31120`. It routed `/function/<name>` straight to
each function's ClusterIP Service, bypassing the OpenFaaS gateway — which, under
CE, refuses to invoke any function once more than 15 are deployed
(`HTTP 405`). Functions' `OPENFAAS_GATEWAY` env was pointed at it so inter-function
SBI calls also bypassed the gateway.

## Why it was retired

The evaluation only exercises 12 functions, which is within the CE limit. Running
the subset on the unmodified OpenFaaS gateway is simpler and keeps the deployment
faithful to a stock OpenFaaS CE install — no custom routing component, and
functions run under the real of-watchdog runtime behind the real gateway.

## ConfigMap (`fn-router-conf`), for reference

```nginx
server {
  listen 8080;
  resolver 10.43.0.10 valid=10s;          # K3s CoreDNS
  location ~ ^/function/([a-zA-Z0-9_-]+)(/.*)?$ {
    set $fn $1;
    set $rest $2;
    proxy_pass http://$fn.openfaas-fn.svc.cluster.local:8080$rest$is_args$args;
    proxy_set_header Host $host;
  }
}
```

The Deployment was a single `nginx:1.27-alpine` replica mounting the ConfigMap at
`/etc/nginx/conf.d/default.conf`, fronted by a `NodePort` Service on `31120`.
