# Makefile for coredns-ingress-sync controller

# Variables
PROJECT_NAME := coredns-ingress-sync
DOCKER_REGISTRY := ghcr.io
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(shell echo $(GITHUB_REPOSITORY) | tr '[:upper:]' '[:lower:]')
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go variables
GO_VERSION := 1.24
GOARCH := amd64
GOOS := linux
CGO_ENABLED := 0

# Build flags
LDFLAGS := -w -s -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: help
help: ## Show this help message
	@echo "$(PROJECT_NAME) - Development Commands"
	@echo "======================================"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: deps
deps: ## Install dependencies
	go mod download
	go mod tidy

.PHONY: init
init: ## Initialize Go modules
	go mod tidy

.PHONY: build
build: ## Build the binary
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags "$(LDFLAGS)" \
		-o bin/$(PROJECT_NAME) \
		main.go

.PHONY: run
run: ## Run the controller locally
	go run main.go

.PHONY: manifests
manifests: ## Generate Kubernetes manifests
	@echo "Kubernetes manifests are in deploy/ directory"

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf test_results/
	rm -f coverage.out coverage.html
	docker system prune -f
	docker volume prune -f

##@ Testing
.PHONY: test
test: ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./...

.PHONY: test-unit
test-unit: ## Run unit tests (alias for test)
	./tests/run_tests.sh --unit

.PHONY: test-coverage
test-coverage: test ## Run tests and generate coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-integration
test-integration: ## Run integration tests
	./tests/run_tests.sh --integration

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	./tests/run_tests.sh --e2e

.PHONY: test-performance
test-performance: ## Run performance benchmarks
	./tests/run_tests.sh --performance

.PHONY: test-all
test-all: ## Run all tests
	./tests/run_tests.sh --all

.PHONY: test-safety
test-safety: ## Run kubecontext safety test
	./tests/test_safety_check.sh

.PHONY: benchmark
benchmark: ## Run benchmarks
	./tests/run_tests.sh --performance

.PHONY: go-test
go-test: ## Run Go tests directly
	go test -v ./...

##@ Git & Development Setup
.PHONY: setup-git-hooks
setup-git-hooks: ## Setup git hooks for conventional commits
	./scripts/setup-git-hooks.sh

.PHONY: setup-dev
setup-dev: ## Setup complete development environment (recommended)
	@echo "Setting up complete development environment..."
	@if command -v asdf >/dev/null 2>&1; then \
		echo "Installing required tool versions..."; \
		asdf install; \
		echo "✓ Tool versions installed via asdf"; \
	else \
		echo "⚠ asdf not found - ensure Go and Node.js are installed manually"; \
	fi
	go mod download
	go mod tidy
	@if command -v npm >/dev/null 2>&1; then \
		echo "Installing commit tools..."; \
		npm install; \
		echo "✓ Git hooks and commit tools installed"; \
	else \
		echo "⚠ npm not found - commit validation will use basic fallback"; \
		echo "Install Node.js for enhanced commit validation"; \
	fi
	@echo "Installing development tools..."
	@$(MAKE) dev-tools
	@echo "✓ Complete development environment ready!"

.PHONY: dev-setup
dev-setup: setup-dev ## Alias for setup-dev (deprecated - use setup-dev)

.PHONY: install-commitlint
install-commitlint: ## Install commitlint for conventional commits validation
	npm install

.PHONY: validate-commit
validate-commit: ## Validate commit message format (usage: make validate-commit MSG="your message")
	@if [ -z "$(MSG)" ]; then \
		echo "Usage: make validate-commit MSG=\"your commit message\""; \
		echo "Example: make validate-commit MSG=\"feat: add new feature\""; \
		exit 1; \
	fi
	@if command -v npx >/dev/null 2>&1 && [ -f package.json ]; then \
		echo "$(MSG)" | npx commitlint; \
	else \
		./scripts/validate-commit-msg.sh "$(MSG)"; \
	fi

.PHONY: commit-interactive
commit-interactive: ## Interactive commit message builder (requires commitlint)
	@if command -v npm >/dev/null 2>&1 && [ -f package.json ]; then \
		npm run commit; \
	else \
		echo "Interactive commits require Node.js and commitlint"; \
		echo "Install with: make install-commitlint"; \
		exit 1; \
	fi

.PHONY: commit-help
commit-help: ## Show conventional commit examples
	@echo "Conventional Commit Examples:"
	@echo "============================="
	@echo "feat: add new feature"
	@echo "fix: resolve bug in ConfigMap handling"
	@echo "docs: update installation guide"
	@echo "style: fix code formatting"
	@echo "refactor: restructure controller logic"
	@echo "test: add integration tests"
	@echo "chore: update dependencies"
	@echo "ci: update GitHub Actions workflow"
	@echo "feat(helm): add production values"
	@echo "fix!: breaking change in API"
	@echo ""
	@echo "Setup: make setup-git-hooks"
	@echo "Install commitlint: make install-commitlint"
	@echo "Interactive commit: make commit-interactive"
	@echo "Validate message: make validate-commit MSG=\"your message\""

.PHONY: go-test-coverage
go-test-coverage: ## Run Go tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: go-bench
go-bench: ## Run Go benchmarks
	go test -bench=. -benchmem ./...

##@ Code Quality
.PHONY: lint
lint: ## Run linters
	golangci-lint run --timeout=5m

.PHONY: fmt
fmt: ## Format code
	gofmt -s -w .
	goimports -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: sec
sec: ## Run security scanner
	gosec ./...

.PHONY: quality
quality: fmt vet lint sec ## Run all code quality checks

##@ Docker
.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(PROJECT_NAME):latest .
	docker tag $(PROJECT_NAME):latest $(PROJECT_NAME):$(VERSION)

.PHONY: docker-build-multi
docker-build-multi: ## Build multi-architecture Docker image
	docker buildx build --platform linux/amd64,linux/arm64 -t $(PROJECT_NAME):latest .

.PHONY: docker-scan
docker-scan: docker-build ## Scan Docker image for vulnerabilities
	trivy image --exit-code 1 --severity HIGH,CRITICAL $(PROJECT_NAME):latest

.PHONY: docker-run
docker-run: docker-build ## Run Docker container locally
	docker run --rm -it $(PROJECT_NAME):latest

.PHONY: docker-push
docker-push: docker-build ## Push Docker image to registry
	docker tag $(PROJECT_NAME):latest $(DOCKER_IMAGE):latest
	docker tag $(PROJECT_NAME):latest $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest
	docker push $(DOCKER_IMAGE):$(VERSION)

##@ Kubernetes
.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes
	kubectl apply -f deploy/rbac.yaml

.PHONY: k8s-undeploy
k8s-undeploy: ## Remove from Kubernetes
	kubectl delete -f deploy/rbac.yaml --ignore-not-found=true

.PHONY: k8s-logs
k8s-logs: ## Show controller logs
	kubectl logs -n kube-system deployment/$(PROJECT_NAME) -f

.PHONY: k8s-status
k8s-status: ## Show controller status
	kubectl get pods -n kube-system -l app=$(PROJECT_NAME)
	kubectl get configmap -n kube-system coredns-custom

##@ Kind/Local Development
.PHONY: kind-create
kind-create: ## Create Kind cluster for testing
	kind create cluster --name $(PROJECT_NAME)-test --config=- <<< 'kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n  kubeadmConfigPatches:\n  - |\n    kind: InitConfiguration\n    nodeRegistration:\n      kubeletExtraArgs:\n        node-labels: "ingress-ready=true"\n  extraPortMappings:\n  - containerPort: 80\n    hostPort: 80\n    protocol: TCP\n  - containerPort: 443\n    hostPort: 443\n    protocol: TCP'

.PHONY: kind-delete
kind-delete: ## Delete Kind cluster
	kind delete cluster --name $(PROJECT_NAME)-test

.PHONY: kind-load
kind-load: docker-build ## Load Docker image into Kind cluster
	kind load docker-image $(PROJECT_NAME):latest --name $(PROJECT_NAME)-test

.PHONY: kind-setup
kind-setup: kind-create kind-load ## Set up complete Kind environment
	kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
	kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s

##@ Kind Multi-Version Testing (Local Only)
.PHONY: kind-test-all-versions
kind-test-all-versions: ## Test controller against all supported Kubernetes versions
	./tests/kind/test-k8s-versions.sh

.PHONY: kind-test-version
kind-test-version: ## Test specific Kubernetes version (usage: make kind-test-version K8S_VERSION=1.29.4)
	@if [ -z "$(K8S_VERSION)" ]; then \
		echo "Usage: make kind-test-version K8S_VERSION=1.29.4"; \
		./tests/kind/test-k8s-versions.sh --list; \
		exit 1; \
	fi
	./tests/kind/test-k8s-versions.sh --version $(K8S_VERSION)

.PHONY: kind-test-latest
kind-test-latest: ## Test latest supported Kubernetes version
	./tests/kind/test-k8s-versions.sh --version 1.30.0

.PHONY: kind-test-oldest
kind-test-oldest: ## Test oldest supported Kubernetes version
	./tests/kind/test-k8s-versions.sh --version 1.25.16

.PHONY: kind-test-cleanup
kind-test-cleanup: ## Clean up all KIND test clusters
	./tests/kind/test-k8s-versions.sh --cleanup

.PHONY: kind-test-list
kind-test-list: ## List supported Kubernetes versions for testing
	./tests/kind/test-k8s-versions.sh --list

##@ Development Workflow
.PHONY: dev-tools
dev-tools: ## Install development tools (Go linters, security scanners)
	@echo "Installing development tools..."
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/aquasecurity/trivy/cmd/trivy@latest
	@echo "Development tools installed!"

.PHONY: dev-test
dev-test: quality test-coverage ## Run development tests
	@echo "All development tests passed!"

.PHONY: dev-build
dev-build: clean build docker-build ## Clean build and Docker image
	@echo "Build completed successfully!"

.PHONY: dev-deploy
dev-deploy: kind-setup k8s-deploy ## Deploy to local Kind cluster
	@echo "Deployed to local Kind cluster!"

.PHONY: ci-test
ci-test: quality test docker-scan ## Run CI tests locally
	@echo "CI tests completed successfully!"

##@ Release
.PHONY: release-dry-run
release-dry-run: ## Simulate release process
	@echo "Simulating release process..."
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Docker Image: $(DOCKER_IMAGE):$(VERSION)"

.PHONY: release
release: ci-test docker-push ## Create release
	@echo "Release $(VERSION) created successfully!"

##@ Utilities
.PHONY: version
version: ## Show version information
	@echo "Project: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(GO_VERSION)"

.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	go doc ./...

.PHONY: update-deps
update-deps: ## Update dependencies
	go get -u ./...
	go mod tidy

##@ Helm
.PHONY: helm-deps
helm-deps: ## Install Helm dependencies
	helm dependency update helm/$(PROJECT_NAME)

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	helm lint helm/$(PROJECT_NAME)

.PHONY: helm-template
helm-template: ## Template Helm chart
	helm template $(PROJECT_NAME) helm/$(PROJECT_NAME)

.PHONY: helm-install
helm-install: ## Install Helm chart
	helm install $(PROJECT_NAME) helm/$(PROJECT_NAME) \
		--namespace $(PROJECT_NAME) \
		--create-namespace

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade Helm chart
	helm upgrade $(PROJECT_NAME) helm/$(PROJECT_NAME) \
		--namespace $(PROJECT_NAME)

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm chart
	helm uninstall $(PROJECT_NAME) --namespace $(PROJECT_NAME)

.PHONY: helm-package
helm-package: ## Package Helm chart
	helm package helm/$(PROJECT_NAME)

##@ Development Tools
.PHONY: dev
dev: build ## Build and run locally for development
	DEVELOPMENT_MODE=true ZONE_FILE_PATH=/tmp/coredns-internal.db ./bin/$(PROJECT_NAME)

.PHONY: debug-config
debug-config: ## Show current CoreDNS configuration
	kubectl get configmap coredns-custom -n kube-system -o yaml

.PHONY: debug-logs
debug-logs: ## Show detailed controller logs
	kubectl logs deployment/$(PROJECT_NAME) -n $(PROJECT_NAME) --tail=100

.PHONY: debug-ingresses
debug-ingresses: ## Show nginx ingresses
	kubectl get ingress -A -o wide | grep nginx || echo "No nginx ingresses found"

.PHONY: clean-tests
clean-tests: ## Clean test artifacts
	rm -rf test_results
	rm -f coverage.out coverage.html

##@ Deployment
.PHONY: deploy-safe
deploy-safe: ## Deploy with safety checks
	./deploy/safe-deploy.sh

.PHONY: validate-import
validate-import: ## Validate CoreDNS import directive placement
	./deploy/validate-import.sh

.PHONY: safe-deploy
safe-deploy: validate-import deploy-safe ## Safe deployment with import validation

##@ Release Management
.PHONY: conventional-commit
conventional-commit: ## Interactive conventional commit helper
	./scripts/conventional-commit.sh

.PHONY: test-release-config
test-release-config: ## Test Release Please configuration
	./scripts/test-release-please.sh

.PHONY: test-release-cli
test-release-cli: ## Test Release Please CLI with dry-run (requires GITHUB_TOKEN)
	./scripts/test-release-please.sh --cli-only

.PHONY: version-check
version-check: test-release-config ## Check version consistency across files (alias for test-release-config)

.PHONY: release-docs
release-docs: ## Open release management documentation
	@echo "📖 Release Management Documentation:"
	@echo "   Local: docs/RELEASE_MANAGEMENT.md"
	@echo "   Online: https://github.com/rl-io/coredns-ingress-sync/blob/main/docs/RELEASE_MANAGEMENT.md"
