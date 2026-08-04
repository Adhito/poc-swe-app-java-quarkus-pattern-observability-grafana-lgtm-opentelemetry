# GitOps build & deploy loop per PRD D9 / section 6.4. Run on the Dev VM
# (needs: mvn, git, kustomize, docker — note `kubectl kustomize` cannot do `edit`).
#
#   make build-push   # build all images, push to the Dev VM registry, tag = git SHA
#   make deploy       # point overlays/local at that SHA, commit, push -> ArgoCD syncs
#
# Commit code changes BEFORE build-push so the SHA on the image matches the code.

GIT_SHA  := $(shell git rev-parse --short HEAD)
REGISTRY := 192.168.56.20:5000
SERVICES    := order-service stock-service notification-service       # Quarkus/Jib services
GO_SERVICES := shipping-service carrier-service                       # Go/docker services
IMAGES      := order-service stock-service notification-service shipping-service carrier-service frontend
OVERLAY     := deploy/overlays/local

.PHONY: build-push build-services build-go build-frontend deploy

build-push: build-services build-go build-frontend

# Quarkus services via Jib (daemonless — no Docker needed for these)
build-services:
	@if ! git diff --quiet HEAD -- src-backend/; then \
		echo "WARNING: uncommitted changes under src-backend/ — image tag $(GIT_SHA) will not match the code"; \
	fi
	@for svc in $(SERVICES); do \
		echo "==> $$svc ($(GIT_SHA))"; \
		( cd src-backend/$$svc && mvn -q package \
			-Dquarkus.container-image.build=true \
			-Dquarkus.container-image.push=true \
			-Dquarkus.container-image.tag=$(GIT_SHA) ) || exit 1; \
	done

# Go services — multi-stage docker build (Jib is Java-only). The Docker build
# is also the compile+vet gate, since the authoring machine has no Go toolchain.
build-go:
	@if ! git diff --quiet HEAD -- src-backend/; then \
		echo "WARNING: uncommitted changes under src-backend/ — image tag $(GIT_SHA) will not match the code"; \
	fi
	@for svc in $(GO_SERVICES); do \
		echo "==> $$svc ($(GIT_SHA))"; \
		docker build -t $(REGISTRY)/$$svc:$(GIT_SHA) src-backend/$$svc || exit 1; \
		docker push $(REGISTRY)/$$svc:$(GIT_SHA) || exit 1; \
	done

# Frontend is static JS (not Java) — plain multi-stage docker build, not Jib.
# `--tls-verify` isn't a thing for docker; the insecure registry is trusted via
# /etc/docker/daemon.json on the Dev VM.
build-frontend:
	@if ! git diff --quiet HEAD -- src-frontend/; then \
		echo "WARNING: uncommitted changes under src-frontend/ — image tag $(GIT_SHA) will not match the code"; \
	fi
	@echo "==> frontend ($(GIT_SHA))"
	docker build -t $(REGISTRY)/frontend:$(GIT_SHA) src-frontend/
	docker push $(REGISTRY)/frontend:$(GIT_SHA)

deploy:
	cd $(OVERLAY) && \
	for img in $(IMAGES); do \
		kustomize edit set image $(REGISTRY)/$$img=$(REGISTRY)/$$img:$(GIT_SHA) || exit 1; \
	done
	git add $(OVERLAY)/kustomization.yaml
	git commit -m "deploy(local): images -> $(GIT_SHA)"
	git push
