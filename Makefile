# GitOps build & deploy loop per PRD D9 / section 6.4. Run on the Dev VM
# (needs: mvn, git, kustomize, docker — note `kubectl kustomize` cannot do `edit`).
#
#   make build-push   # build all images, push to the Dev VM registry, tag = git SHA
#   make deploy       # point overlays/local at that SHA, commit, push -> ArgoCD syncs
#
# Commit code changes BEFORE build-push so the SHA on the image matches the code.

GIT_SHA  := $(shell git rev-parse --short HEAD)
REGISTRY := 192.168.56.20:5000
SERVICES := order-service stock-service       # Quarkus/Jib services
IMAGES   := order-service stock-service frontend  # everything the overlay tags
OVERLAY  := deploy/overlays/local

.PHONY: build-push build-services build-frontend deploy

build-push: build-services build-frontend

# Quarkus services via Jib (daemonless — no Docker needed for these)
build-services:
	@if ! git diff --quiet HEAD -- services/; then \
		echo "WARNING: uncommitted changes under services/ — image tag $(GIT_SHA) will not match the code"; \
	fi
	@for svc in $(SERVICES); do \
		echo "==> $$svc ($(GIT_SHA))"; \
		( cd services/$$svc && mvn -q package \
			-Dquarkus.container-image.build=true \
			-Dquarkus.container-image.push=true \
			-Dquarkus.container-image.tag=$(GIT_SHA) ) || exit 1; \
	done

# Frontend is static JS (not Java) — plain multi-stage docker build, not Jib.
# `--tls-verify` isn't a thing for docker; the insecure registry is trusted via
# /etc/docker/daemon.json on the Dev VM.
build-frontend:
	@if ! git diff --quiet HEAD -- frontend/; then \
		echo "WARNING: uncommitted changes under frontend/ — image tag $(GIT_SHA) will not match the code"; \
	fi
	@echo "==> frontend ($(GIT_SHA))"
	docker build -t $(REGISTRY)/frontend:$(GIT_SHA) frontend/
	docker push $(REGISTRY)/frontend:$(GIT_SHA)

deploy:
	cd $(OVERLAY) && \
	for img in $(IMAGES); do \
		kustomize edit set image $(REGISTRY)/$$img=$(REGISTRY)/$$img:$(GIT_SHA) || exit 1; \
	done
	git add $(OVERLAY)/kustomization.yaml
	git commit -m "deploy(local): images -> $(GIT_SHA)"
	git push
