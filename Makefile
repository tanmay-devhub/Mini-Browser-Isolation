# ============================================================================
#  Mini Browser Isolation – Makefile
#  One-command local dev: make dev
# ============================================================================

.PHONY: dev build-runner build-orchestrator build-frontend test test-unit test-e2e \
        k8s-apply k8s-delete lint clean help

COMPOSE_FILE := infra/docker-compose/docker-compose.yml
COMPOSE      := docker compose -f $(COMPOSE_FILE)

## ── Local dev ───────────────────────────────────────────────────────────────

# Start everything: build images then bring up all services.
dev: build-runner build-orchestrator build-frontend
	$(COMPOSE) up --remove-orphans

# Same as dev but run in background.
dev-detached: build-runner build-orchestrator build-frontend
	$(COMPOSE) up -d --remove-orphans

# Stop all services and clean up containers.
stop:
	$(COMPOSE) down --remove-orphans

# Full teardown including volumes.
clean:
	$(COMPOSE) down -v --remove-orphans
	docker rmi -f mini-browser-runner:latest mini-browser-orchestrator:latest mini-browser-frontend:latest 2>/dev/null || true

## ── Image builds ────────────────────────────────────────────────────────────

build-runner:
	docker build -t mini-browser-runner:latest ./runner

build-orchestrator:
	docker build -t mini-browser-orchestrator:latest ./orchestrator

build-frontend:
	docker build -t mini-browser-frontend:latest ./frontend

## ── Testing ─────────────────────────────────────────────────────────────────

# Run Go unit tests for the orchestrator session manager.
test-unit:
	cd orchestrator && go test ./session/... -v -count=1

# Run Playwright e2e smoke tests (requires `make dev-detached` first).
test-e2e:
	cd tests/e2e && npm install && npx playwright install --with-deps chromium && npm test

# Run all tests.
test: test-unit test-e2e

## ── Kubernetes ───────────────────────────────────────────────────────────────

# Apply all K8s manifests in dependency order.
k8s-apply:
	kubectl apply -f infra/k8s/namespace.yaml
	kubectl apply -f infra/k8s/runner-rbac.yaml
	kubectl apply -f infra/k8s/orchestrator-deployment.yaml
	kubectl apply -f infra/k8s/orchestrator-service.yaml
	kubectl apply -f infra/k8s/orchestrator-hpa.yaml
	kubectl apply -f infra/k8s/coturn-deployment.yaml
	kubectl apply -f infra/k8s/prometheus-scrape-cm.yaml

# Delete all K8s resources.
k8s-delete:
	kubectl delete -f infra/k8s/ --ignore-not-found

## ── Utilities ────────────────────────────────────────────────────────────────

logs:
	$(COMPOSE) logs -f orchestrator

lint:
	cd orchestrator && go vet ./...
	cd runner && go vet ./...

help:
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'
