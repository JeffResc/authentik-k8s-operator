# Contributing to Authentik Kubernetes Operator

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Development Prerequisites

- Go 1.26+
- Make
- Docker (for building container images)
- [golangci-lint](https://golangci-lint.run/) v2.11+
- A Kubernetes cluster (for testing)
- An Authentik instance with API access (for e2e testing)

## Setting Up Your Development Environment

1. Clone the repository:
   ```bash
   git clone https://github.com/JeffResc/authentik-k8s-operator.git
   cd authentik-k8s-operator
   ```

2. Generate code and manifests:
   ```bash
   make generate manifests
   ```

3. Set up environment variables for local runs:
   ```bash
   export AUTHENTIK_URL=https://authentik.example.com
   export AUTHENTIK_TOKEN=your-api-token
   ```

## Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
go test ./... -v

# Run tests for a specific package
go test ./internal/template/... -v
```

## Code Style

This project uses `golangci-lint` with the configuration in `.golangci.yaml`. Before submitting a PR, ensure your code passes all checks:

```bash
# Format code
make fmt

# Run static analysis
make vet

# Run the full linter suite
golangci-lint run --timeout=5m ./...
```

## Pull Request Requirements

- All CI checks must pass (lint, build, tests)
- Use [conventional commits](https://www.conventionalcommits.org/) for PR titles (e.g., `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`)
- Include tests for new functionality
- Keep PRs focused — one logical change per PR

## Building

```bash
# Build the binary
make build

# Build the Docker image
make docker-build IMG=my-registry/authentik-operator:latest

# Package the Helm chart
make helm-package
```

## Project Structure

```
├── api/v1alpha1/          # CRD type definitions
├── cmd/                   # Entry point
├── config/                # Kubernetes manifests (CRDs, RBAC)
├── charts/                # Helm chart
├── hack/                  # Scripts (e2e tests, boilerplate)
├── internal/
│   ├── authentik/         # Authentik API client
│   ├── controller/        # Reconciliation logic
│   └── template/          # Secret template rendering
```
