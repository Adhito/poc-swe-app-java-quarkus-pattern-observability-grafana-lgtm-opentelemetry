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
