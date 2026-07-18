# Runbook — Quarkus Distributed Tracing POC

Companion to [prd.md](prd.md). Records pinned versions (D7), bootstrap steps, and — as phases complete — how to demo each success criterion S1–S7.

## Pinned versions (D7 — exact tags only, never `latest`)

Pinned 2026-07-18 (Phase 0). Java-side OTel versions are managed by the Quarkus BOM and never pinned separately (D7).

| Component | Image / Chart | Version |
|---|---|---|
| Container registry (Dev VM, Podman) | `docker.io/library/registry` | `2.8.3` |
| OTel Collector | `otel/opentelemetry-collector-contrib` | `0.156.0` |
| Grafana LGTM all-in-one | `grafana/otel-lgtm` | `0.29.0` |
| MetalLB | Helm chart `metallb` @ `https://metallb.github.io/metallb` | `0.16.1` |
| ingress-nginx | Helm chart `ingress-nginx` @ `https://kubernetes.github.io/ingress-nginx` | `4.15.1` |
| Quarkus platform BOM | `io.quarkus.platform:quarkus-bom` | `3.33.1` (pinned 2026-07-18, Phase 1) |
| Java | — | `21` |
| OTel JS SDK | `@opentelemetry/*` (single 2.x train) | pin at Phase 3 start |
| PostgreSQL | pin at Phase 2 | — |
| Kafka | pin at Phase 4 | — |

## Phase 0 — bootstrap & verification

All commands run from the Dev VM (`192.168.56.20`) unless noted.

### 1. Registry on the Dev VM (D9)

Follow [infra/devvm-registry/README.md](../infra/devvm-registry/README.md) — installs the Podman Quadlet unit, enables linger, starts the registry on port 5000.

### 2. containerd trust on cluster nodes (D9)

```bash
cd infra/ansible
cp inventory.example.ini inventory.ini   # adjust if needed
ansible-playbook -i inventory.ini registry-trust.yaml
```

### 3. ArgoCD bootstrap (D8)

The one permitted imperative step — everything after it is GitOps:

```bash
kubectl apply -f deploy/argocd/local-root.yaml
```

`local-root` syncs `deploy/argocd/local/`, which installs (in sync-wave order): MetalLB (wave 0) → ingress-nginx pinned to `192.168.56.240` (wave 1) → the `deploy/overlays/local` Kustomize overlay (wave 2: observability namespace, OTel Collector, LGTM, ingresses, MetalLB address pool).

### 4. Exit criteria (PRD §7, Phase 0)

| Check | Command / action | Expect |
|---|---|---|
| Registry reachable from cluster | on a cluster node: `curl http://192.168.56.20:5000/v2/_catalog` | JSON (`{"repositories":[...]}`) |
| ArgoCD apps synced | `kubectl get applications -n argocd` (or UI at `https://192.168.56.10:30002`) | all Synced/Healthy |
| Grafana via ingress | browse `http://grafana.192.168.56.240.nip.io` | Grafana UI loads |
| Collector healthy | `kubectl -n observability get pods` ; logs show no exporter errors | Running, ready 1/1 |
| Image push/pull round trip | `podman pull docker.io/library/alpine:3.20 && podman tag alpine:3.20 192.168.56.20:5000/test:1 && podman push --tls-verify=false 192.168.56.20:5000/test:1` then on a node: `crictl pull 192.168.56.20:5000/test:1` | both succeed |

### Endpoints (local overlay)

| What | URL |
|---|---|
| Grafana | `http://grafana.192.168.56.240.nip.io` |
| order-service API | `http://api.192.168.56.240.nip.io` |
| OTLP/HTTP (browser export, Phase 3) | `http://otlp.192.168.56.240.nip.io` |
| ArgoCD | `https://192.168.56.10:30002` (NodePort) |
| Registry | `http://192.168.56.20:5000` |

## Demo scripts per success criterion

Filled in as each phase completes.

- **S1 — one trace across order → stock** (browser root span extends this in Phase 3):

  ```bash
  # on the Dev VM, repo committed and pushed
  make build-push          # Jib-builds both services, pushes :$(git SHA) to 192.168.56.20:5000
  make deploy              # bumps overlays/local image tags, commits, pushes -> ArgoCD syncs
  kubectl get pods -n tracing-poc          # wait for order-service + stock-service Ready

  # place an order (SKU-1..SKU-3 have stock; SKU-4 is empty; unknown sku -> 400)
  curl -s -X POST http://api.192.168.56.240.nip.io/orders \
    -H 'content-type: application/json' \
    -d '{"sku":"SKU-1","quantity":2}'
  # -> 201 {"orderId":"...","sku":"SKU-1","quantity":2,"status":"PLACED"}
  ```

  Then in Grafana (`http://grafana.192.168.56.240.nip.io`): **Explore → Tempo datasource →
  Search**, service `order-service`, most recent trace. The waterfall must show
  `POST /orders` (order-service) with a child REST-client span and under it
  `GET /stock/{sku}` (stock-service) — all sharing **one trace ID**. That is S1
  (two services; the browser root span is added in Phase 3).
- **S2 — SQL child span with `db.statement`:** _Phase 2_
- **S3 — trace-to-logs jump:** _Phase 2_
- **S4 — custom `@WithSpan`:** _Phase 2_
- **S5 — async Kafka hop in same trace:** _Phase 4_
- **S6 — exemplar → trace:** _Phase 5_
- **S7 — declarative reproducibility:** continuous — `kubectl apply -f deploy/argocd/local-root.yaml` on a clean cluster reproduces everything

## Propagation debug (orphan / split traces)

Per PRD risk table — check `traceparent` at each hop of the §4.3 sequence:
1. Browser devtools → request headers on `POST /orders` → `traceparent` present?
2. CORS preflight response allows `traceparent`?
3. order-service logs: same traceId as browser span?
4. stock-service request headers (enable access log or tcpdump)
5. Kafka message headers carry `traceparent`?
