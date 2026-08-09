COMPOSE := docker compose --env-file deploy/.env -f deploy/compose.yaml

TRIVY_IMAGE ?= aquasec/trivy:0.61.1

.PHONY: setup fmt fmt-check lint typecheck test build check dev frontend-dev api-dev compose-config up down logs scan-dependencies scan-filesystem scan-images sbom security-scan

setup:
	cd frontend && npm ci
	cp -n deploy/.env.example deploy/.env || true

fmt:
	cd backend && gofmt -w $$(find . -name '*.go' -type f)
	cd frontend && npm run format

fmt-check:
	test -z "$$(gofmt -l backend)"
	cd frontend && npm run format:check

lint:
	cd backend && go vet ./...
	cd frontend && npm run lint

typecheck:
	cd frontend && npm run typecheck

test:
	cd backend && go test -race ./...
	cd frontend && npm test

build:
	cd backend && go build -trimpath -o bin/api ./cmd/api
	cd frontend && npm run build

check: fmt-check lint typecheck test build compose-config

frontend-dev:
	cd frontend && npm run dev

api-dev:
	cd backend && go run ./cmd/api

compose-config:
	$(COMPOSE) config --quiet

up:
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

scan-dependencies:
	command -v govulncheck >/dev/null || { echo "Install govulncheck: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 2; }
	cd backend && govulncheck ./...
	cd frontend && npm audit --audit-level=high

scan-filesystem:
	docker run --rm -v "$(CURDIR):/workspace:ro" $(TRIVY_IMAGE) fs --exit-code 1 --severity HIGH,CRITICAL --scanners vuln,secret,misconfig /workspace

scan-images: up
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --exit-code 1 --severity HIGH,CRITICAL family-habit-api
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --exit-code 1 --severity HIGH,CRITICAL family-habit-frontend
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --exit-code 1 --severity HIGH,CRITICAL family-habit-caddy
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --exit-code 1 --severity HIGH,CRITICAL family-habit-postgres

sbom: up
	mkdir -p artifacts/phase-8
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --format cyclonedx family-habit-api > artifacts/phase-8/api-sbom.cdx.json
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock:ro $(TRIVY_IMAGE) image --format cyclonedx family-habit-frontend > artifacts/phase-8/frontend-sbom.cdx.json

security-scan: scan-dependencies scan-filesystem scan-images sbom
