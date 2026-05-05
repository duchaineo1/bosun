REGISTRY ?= ghcr.io/duchaineo1
TAG      ?= latest

# ── Local dev ────────────────────────────────────────────────────────────────
dev:
	cd deploy/dev && docker compose up --build

dev-down:
	cd deploy/dev && docker compose down

# ── Build images ─────────────────────────────────────────────────────────────
build-api:
	cd services/api && go mod tidy && docker build -t $(REGISTRY)/bosun-api:$(TAG) .

build-controller:
	cd services/controller && go mod tidy && docker build -t $(REGISTRY)/bosun-controller:$(TAG) .

build-runner:
	docker build -t $(REGISTRY)/bosun-runner:$(TAG) services/runner

build-ui:
	docker build -t $(REGISTRY)/bosun-ui:$(TAG) services/ui

build: build-api build-controller build-runner build-ui

# ── Push images ───────────────────────────────────────────────────────────────
push:
	docker push $(REGISTRY)/bosun-api:$(TAG)
	docker push $(REGISTRY)/bosun-controller:$(TAG)
	docker push $(REGISTRY)/bosun-runner:$(TAG)
	docker push $(REGISTRY)/bosun-ui:$(TAG)

# ── Kubernetes ────────────────────────────────────────────────────────────────
deploy-ns:
	kubectl apply -f deploy/manifests/namespace.yaml

create-pull-secret:
	kubectl create secret docker-registry ghcr-pull-secret \
		--docker-server=ghcr.io \
		--docker-username=$(GITHUB_USER) \
		--docker-password=$(GITHUB_PAT) \
		--namespace=bosun \
		--dry-run=client -o yaml | kubectl apply -f -

deploy: deploy-ns
	kubectl apply -f deploy/manifests/secrets.yaml
	kubectl apply -f deploy/manifests/rbac.yaml
	kubectl apply -f deploy/manifests/postgres.yaml
	kubectl apply -f deploy/manifests/api.yaml
	kubectl apply -f deploy/manifests/controller.yaml
	kubectl apply -f deploy/manifests/ui.yaml

status:
	kubectl get all -n bosun

logs-api:
	kubectl logs -n bosun -l app=api -f

logs-controller:
	kubectl logs -n bosun -l app=controller -f

# ── Go mod tidy (run before first build) ────────────────────────────────────
tidy:
	cd services/api && go mod tidy
	cd services/controller && go mod tidy

.PHONY: dev dev-down build build-api build-controller build-runner build-ui \
        push deploy deploy-ns create-pull-secret status logs-api logs-controller tidy
