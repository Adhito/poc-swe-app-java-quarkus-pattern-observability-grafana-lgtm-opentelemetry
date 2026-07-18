# GitOps build & deploy loop per PRD D9 / section 6.4. Run on the Dev VM
# (needs: mvn, git, kustomize — note `kubectl kustomize` cannot do `edit`).
#
#   make build-push   # Jib-build both services, push to the Dev VM registry, tag = git SHA
#   make deploy       # point overlays/local at that SHA, commit, push -> ArgoCD syncs
#
# Commit code changes BEFORE build-push so the SHA on the image matches the code.

GIT_SHA  := $(shell git rev-parse --short HEAD)
REGISTRY := 192.168.56.20:5000
SERVICES := order-service stock-service
OVERLAY  := deploy/overlays/local

.PHONY: build-push deploy

build-push:
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

deploy:
	cd $(OVERLAY) && \
	for svc in $(SERVICES); do \
		kustomize edit set image $(REGISTRY)/$$svc=$(REGISTRY)/$$svc:$(GIT_SHA) || exit 1; \
	done
	git add $(OVERLAY)/kustomization.yaml
	git commit -m "deploy(local): images -> $(GIT_SHA)"
	git push
