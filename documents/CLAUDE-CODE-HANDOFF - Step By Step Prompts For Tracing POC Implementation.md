# Claude Code Handoff Playbook — Quarkus Tracing POC

Companion to: *PRD — JAVA QUARKUS - POC Concept For Tracing Log Pattern In Java Quarkus* (v1.1.1)

## Core working principles (read once, apply always)

1. **One phase per session.** Start each phase with a fresh context (`/clear` or a new session). Claude Code works best with focused scope; the PRD is the shared memory between sessions.
2. **Plan before code.** For every phase, first ask for a plan, review it, *then* say "proceed." Never let it implement unreviewed.
3. **The PRD is law.** Every prompt points Claude Code at `docs/prd.md` and names the specific sections/decisions that govern the work. If Claude Code proposes something that contradicts a decision (D1–D10), tell it to re-read the decision log.
4. **Exit criteria are the definition of done.** End every phase by making Claude Code verify against the phase's exit criteria in PRD section 7 and show evidence (command output, screenshots you take).
5. **Commit per phase.** Each phase ends with a clean commit (or PR) before moving on. ArgoCD deploys from Git, so uncommitted work literally doesn't exist for the cluster.
6. **Run Claude Code on the Dev VM** (where mvn, kubectl, podman, and the mounted repo live), so it can execute and verify — not just write files.

---

## Step 0 — Repository preparation (manual, ~15 minutes)

Before involving Claude Code:

1. Create the repo `quarkus-tracing-poc` (structure per PRD section 8 — Claude Code will build the skeleton, just init the repo).
2. Copy the PRD into `docs/prd.md`.
3. Create `CLAUDE.md` at the repo root (content below) — Claude Code reads this automatically every session.
4. `git init`, first commit, push to your remote.

### CLAUDE.md content (copy this in)

```markdown
# Project: Quarkus Distributed Tracing POC

## Before anything else
Read docs/prd.md. It contains the architecture, 7 success criteria (S1–S7),
phased plan (section 7), and a decision log (D1–D10) that is binding.
Do not contradict a logged decision without flagging it to me first.

## Hard constraints (from PRD decisions)
- Quarkus 3.33 LTS via io.quarkus.platform:quarkus-bom, Java 21 (D7)
- Container images: exact tags only, never :latest (D7)
- Image registry: 192.168.56.20:5000 (Dev VM, plain HTTP, insecure=true) (D9)
- Images built with quarkus-container-image-jib, tagged with git SHA (D9)
- All deploys via ArgoCD Applications + Kustomize overlays; never
  kubectl apply as the standard path (D8, S7)
- Kustomize layout: deploy/base + deploy/overlays/{local,eks,eks-demo} (section 8)
- Quarkus pods carry label logs.export/otlp: "true" (D5)
- Kafka: single-pod KRaft, plain Deployment, no operator (D6)
- Logs via OTLP export from Quarkus; JSON console logging stays on (D5)

## Environment
- Dev VM: 192.168.1.10 + 192.168.56.20 (build host, registry, this repo)
- Cluster: control 192.168.56.10, workers .11/.12 (Vagrant, K8s 1.33+, Calico)
- ArgoCD: pre-provisioned by cluster bootstrap, NodePort 30002
- MetalLB pool: 192.168.56.240-250; ingress hostnames via nip.io

## Workflow
- Work one PRD phase at a time; verify against the phase exit criteria
  (PRD section 7) before declaring done
- Update docs/runbook.md as you go: how to demo each success criterion
- Commit at phase boundaries with message "Phase N: <summary>"
```

---

## Step 1 — Phase 0 pre-work (infrastructure repos, not the app repo)

The D10 pre-work touches your **Vagrant repos**, not the POC repo. Run Claude Code in the K8s cluster repo directory and the Dev VM repo directory for this. Prompt:

```
Read the file docs/prd.md in ~/workspace-app/quarkus-tracing-poc (or paste path),
specifically decision D10 and the Phase 0 pre-work list in section 7.

In THIS repo, make the following changes only — show me the diff before writing:
1. [Dev VM repo] Add a second private_network NIC with IP 192.168.56.20
   to the Vagrantfile, keeping the existing 192.168.1.10 network intact.
2. [K8s repo] In settings.yaml, bump kubernetes from 1.29.0-* to 1.33
   (verify the exact package pattern the setup scripts expect), and check
   whether calico 3.28.0 supports K8s 1.33 — if not, propose the version bump.
3. [K8s repo] Fix the bug on line ~130 of the Vagrantfile where the ArgoCD
   provisioner condition checks software.dashboard instead of software.argocd.

Do not change anything else. Explain any side effects of the K8s version bump
on the setup scripts in scripts-setup/.
```

Then rebuild: `vagrant reload` (Dev VM) and rebuild the cluster. Verify manually:

```
From devnodemaster01: ping 192.168.56.20   → must succeed
kubectl get nodes                           → all Ready, VERSION v1.33.x
kubectl get pods -n argocd                  → ArgoCD running
```

## Step 2 — Phase 0 (platform foundation, in the POC repo)

Fresh session in `quarkus-tracing-poc`. Prompt:

```
Read docs/prd.md. We are implementing Phase 0 (section 7). Pre-work from D10
is already done: the cluster runs K8s 1.33, ArgoCD is up on NodePort 30002,
and the Dev VM has IP 192.168.56.20.

Plan first, don't write files yet. The plan must cover:
1. Podman systemd/Quadlet unit for registry:2 on this VM per D9
   (data in /home/vagrant/registry-data, port 5000)
2. Ansible task (or standalone playbook) that drops containerd hosts.toml
   trust config for 192.168.56.20:5000 on all worker nodes
3. deploy/base + deploy/overlays/local Kustomize skeleton per section 8
4. MetalLB install + IPAddressPool 192.168.56.240-250
5. ingress-nginx via Service: LoadBalancer
6. observability namespace: OTel Collector + Grafana LGTM all-in-one,
   exact image tags (record them in docs/runbook.md per D7)
7. ArgoCD Application manifests in deploy/argocd/ pointing at overlays/local
8. Grafana ingress at grafana.<metallb-ip>.nip.io

Show me the plan with the exact files you'll create. Wait for my approval.
```

Review the plan, then: `Proceed. After creating the files, walk me through applying the ArgoCD Applications and verifying each Phase 0 exit criterion from section 7, one by one, with the commands to run.`

**Exit check prompt (use this pattern at the end of EVERY phase):**

```
Verify Phase 0 against its exit criteria in docs/prd.md section 7.
Run the verification commands yourself where possible and show the output:
- curl http://192.168.56.20:5000/v2/_catalog from a cluster node (via ssh/vagrant)
- Grafana loads through ingress
- Collector healthy
- ArgoCD apps all Synced/Healthy
- Test image push + pull round trip
List anything failing and fix it before we close the phase.
Then commit with message "Phase 0: platform foundation".
```

## Step 3 — Phase 1 (two services, one trace)

Fresh session. Prompt:

```
Read docs/prd.md. Phase 0 is complete and committed. Implement Phase 1:
order-service and stock-service per sections 5.1 and 5.2 — but stock-service
returns STUB data this phase (no DB yet, per the phase plan).

Constraints reminder: Quarkus 3.33 LTS BOM, Java 21, quarkus-opentelemetry,
Jib push to 192.168.56.20:5000 with insecure=true, pods labeled
logs.export/otlp: "true", manifests in deploy/base/apps with the local overlay,
deployed via a new ArgoCD Application.

Also create the Makefile with targets: build-push (both services, git SHA tag)
and deploy (kustomize edit set image + commit) per D9/section 6.4.

Plan first with the file list; wait for my approval.
```

Exit check: one trace ID spanning both services in Tempo. Ask Claude Code to generate a test request (`curl` the order endpoint through the ingress) and tell you exactly where to look in Grafana.

## Step 4 — Phase 2 (database, logs, custom spans)

```
Read docs/prd.md. Implement Phase 2: PostgreSQL (base/platform manifests,
exact image tag), stock-service switched from stubs to instrumented JDBC
(db.statement visible per S2), OTLP log export per D5 with JSON console
logging retained, Grafana Tempo trace-to-logs datasource linking, and one
@WithSpan on order validation per section 5.1.

Plan first. Include the Grafana datasource provisioning change needed for
the trace→logs jump, and the schema + seed data for the stock table.
```

Exit check: SQL span visible; click a span → Loki log lines with matching traceId.

## Step 5 — Phase 3 (browser root span) — the finicky one

```
Read docs/prd.md. Implement Phase 3: the frontend per section 5.4.
OTel JS: pin the latest 2.x versions now and record them in docs/runbook.md (D7).

Critical details from the PRD:
- Collector OTLP/HTTP (4318) exposed via its own ingress hostname
- Collector CORS allowed_origins must include the frontend's nip.io origin
- order-service CORS must allow the traceparent header from the frontend origin
- Browser fetch instrumentation with W3C trace context propagator

Plan first. Then, before wiring the JS SDK, give me a curl command that
proves the collector's OTLP/HTTP endpoint is reachable through the ingress
from this VM — per the PRD risk table, we verify the path before the SDK.
```

If traces split into two (browser orphan + backend trace), that's CORS or the propagator — ask: `The browser trace and backend trace have different trace IDs. Debug the propagation: check the actual request headers in the browser devtools instructions, the CORS preflight response, and the propagator config.`

## Step 6 — Phase 4 (Kafka async hop)

```
Read docs/prd.md. Implement Phase 4: single-pod KRaft Kafka per D6/section 5.6
(plain Deployment + Service, exact image tag, orders topic), order-service
publishing OrderPlaced via SmallRye Reactive Messaging (section 5.1), and
notification-service (section 5.3) consuming it.

Plan first. Confirm how trace context flows through Kafka message headers
with SmallRye and what config, if any, is needed for the consumer span to
join the same trace.
```

Exit check: the waterfall in PRD section 4.4 — async consumer bar, same trace ID, starting after the HTTP response.

## Step 7 — Phase 5 (exemplars)

```
Read docs/prd.md. Implement Phase 5: Micrometer + Prometheus registry with
exemplars enabled on HTTP server histograms (section 6.3), Prometheus
scraping/receiving via the collector as designed, Grafana provisioned with
an exemplar-enabled latency panel linked to the Tempo datasource.

Plan first. Note any collector pipeline changes needed for metrics.
```

Exit check: click an exemplar dot → land on the trace. **This completes Stage A: run the full S1–S7 verification** — prompt: `Run a complete acceptance pass: verify every success criterion S1–S7 from docs/prd.md section 2, with evidence for each. Update docs/runbook.md with the demo script for showing all seven. Then commit "Stage A complete".`

## Stage B (Phase 6 — EKS) — when you're ready

Same pattern: fresh session, `Read docs/prd.md, implement Phase 6` — but do the deferred decisions first (ingress controller choice, storage) and the backlogged EKS cost analysis as a *conversation* before an implementation session. Terraform for the cluster, `overlays/eks` + `overlays/eks-demo`, SSM tunnel scripts, and re-run the S1–S6 acceptance pass.

---

## Troubleshooting prompts (keep handy)

- **Orphan/split traces:** `Trace X and trace Y should be one trace. Check traceparent propagation at each hop per the PRD 4.3 sequence diagram and tell me which hop drops it.`
- **Nothing in Tempo:** `No spans arriving. Check in order: app OTLP endpoint config, collector receiver logs, collector exporter logs, Tempo ingest. Show the evidence at each step.`
- **ArgoCD drift:** `argocd app diff shows drift on <app>. Explain the diff and fix it in Git, not in the cluster.`
- **It contradicted the PRD:** `Stop — this contradicts decision D<n> in docs/prd.md. Re-read the decision log and redo this consistent with it, or tell me why the decision should change.`

## What NOT to do

- Don't paste the whole PRD into the prompt — it's in the repo; point at sections.
- Don't let one session run Phases 1–5 back-to-back; quality drops as context fills.
- Don't accept "it should work" — every phase ends with executed verification.
- Don't let it `kubectl apply` app manifests directly "just to test" — that breaks S7 discipline; dev-mode (`mvn quarkus:dev`) on the Dev VM is the fast loop, ArgoCD is the deploy path.
