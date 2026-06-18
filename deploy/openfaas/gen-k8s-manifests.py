#!/usr/bin/env python3
"""Generate raw K8s Deployment+Service manifests for every function in a stack file.

This is the offline / private-registry fallback deploy path. OpenFaaS CE rejects
deploying non-public images via faas-cli ("only allows public images"); when the
images cannot be published, we render them as plain Deployments+Services labelled
`faas_function: <name>` (imagePullPolicy: Never) so the gateway still resolves and
routes /function/<name> to them. The preferred path publishes images to a public
registry and uses `faas-cli deploy` (see deploy-eval-subset.sh / README).

The function images carry the of-watchdog runtime and their process config
(mode/fprocess/upstream_url) as image ENV, so no fprocess env is injected here.

Usage:  python3 gen-k8s-manifests.py deploy/openfaas/stack-eval.yml > /tmp/functions.yaml
"""
import sys

NAMESPACE = "openfaas-fn"


def parse_stack(path):
    """Minimal parser for the regular stack.yml structure (no PyYAML dependency)."""
    funcs = {}
    cur = None
    in_functions = False
    in_env = False
    with open(path) as fh:
        for raw in fh:
            line = raw.rstrip("\n")
            if not line.strip() or line.strip().startswith("#"):
                continue
            indent = len(line) - len(line.lstrip(" "))
            stripped = line.strip()

            if indent == 0:
                in_functions = stripped == "functions:"
                cur = None
                in_env = False
                continue
            if not in_functions:
                continue

            if indent == 2 and stripped.endswith(":"):
                cur = stripped[:-1]
                funcs[cur] = {"image": None, "env": {}}
                in_env = False
                continue

            if cur is None:
                continue

            if indent == 4:
                in_env = False
                if stripped.startswith("image:"):
                    funcs[cur]["image"] = stripped.split(":", 1)[1].strip()
                elif stripped == "environment:":
                    in_env = True
                continue

            if indent >= 6 and in_env:
                key, _, val = stripped.partition(":")
                val = val.strip().strip('"').strip("'")
                funcs[cur]["env"][key.strip()] = val
    return funcs


def emit(name, spec):
    env_lines = "\n".join(
        f'        - {{ name: "{k}", value: "{v}" }}'
        for k, v in spec["env"].items()
    )
    return f"""apiVersion: apps/v1
kind: Deployment
metadata:
  name: {name}
  namespace: {NAMESPACE}
  labels: {{ faas_function: {name} }}
spec:
  replicas: 1
  selector: {{ matchLabels: {{ faas_function: {name} }} }}
  template:
    metadata:
      labels: {{ faas_function: {name} }}
    spec:
      containers:
      - name: {name}
        image: {spec['image']}
        imagePullPolicy: Never
        ports: [ {{ containerPort: 8080 }} ]
        env:
{env_lines}
        readinessProbe:
          tcpSocket: {{ port: 8080 }}
          initialDelaySeconds: 2
          periodSeconds: 5
        resources:
          requests: {{ cpu: "25m", memory: "32Mi" }}
---
apiVersion: v1
kind: Service
metadata:
  name: {name}
  namespace: {NAMESPACE}
  labels: {{ faas_function: {name} }}
spec:
  selector: {{ faas_function: {name} }}
  ports: [ {{ name: http, port: 8080, targetPort: 8080 }} ]
---
"""


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "deploy/openfaas/stack.yml"
    funcs = parse_stack(path)
    if not funcs:
        sys.exit("no functions parsed from " + path)
    out = []
    for name, spec in funcs.items():
        if not spec["image"]:
            sys.exit(f"function {name} has no image")
        out.append(emit(name, spec))
    sys.stderr.write(f"generated manifests for {len(funcs)} functions\n")
    sys.stdout.write("".join(out))


if __name__ == "__main__":
    main()
