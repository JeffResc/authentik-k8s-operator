# Repository Audit Report: authentik-k8s-operator

**Date:** 2026-04-11
**Scope:** Full repository audit for best practices
**Repository:** github.com/JeffResc/authentik-k8s-operator

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Project Overview](#project-overview)
3. [Code Quality](#code-quality)
4. [Testing](#testing)
5. [Security](#security)
6. [Kubernetes Manifests & CRDs](#kubernetes-manifests--crds)
7. [CI/CD Pipeline](#cicd-pipeline)
8. [Documentation & Project Health](#documentation--project-health)
9. [Findings Summary](#findings-summary)
10. [Recommendations](#recommendations)

---

## Executive Summary

The **authentik-k8s-operator** is a well-structured Kubernetes operator for declaratively managing Authentik OAuth2/OIDC applications. The project demonstrates strong fundamentals in security hardening, Helm chart design, and CI/CD automation. However, there are critical gaps in test coverage and several code quality improvements that should be addressed before wider adoption.

### Overall Assessment

| Category | Rating | Summary |
|----------|--------|---------|
| Code Quality | **Good** | Clean structure, some error handling inconsistencies |
| Testing | **Critical** | Zero test files — highest priority gap |
| Security | **Excellent** | Non-root, read-only FS, dropped capabilities, distroless image |
| Kubernetes Manifests | **Excellent** | Well-validated CRDs, proper RBAC, good Helm chart |
| CI/CD | **Good** | Comprehensive pipeline, release automation |
| Documentation | **Good** | Strong README, missing contributing/security docs |

---

## Project Overview

- **Language:** Go 1.25.0
- **Framework:** controller-runtime v0.23.3 (Kubebuilder)
- **API Version:** v1alpha1
- **Deployment:** Helm 3 chart, multi-arch Docker images (amd64/arm64)
- **Lines of Go Code:** ~1,163 (excluding generated)
- **Direct Dependencies:** 5

---

## Code Quality

### Strengths

- **Clean project layout** following standard Kubebuilder conventions (`api/`, `internal/`, `cmd/`, `config/`)
- **Proper error wrapping** using `fmt.Errorf` with `%w` throughout the codebase
- **Custom API error extraction** (`extractAPIError` in `internal/authentik/client.go`) provides user-friendly messages
- **Structured logging** via `go-logr/zap`
- **Comprehensive linter configuration** (`.golangci.yaml`) with 10 linters and 16 revive rules enabled

### Issues Found

#### HIGH: Code Duplication in Provider CRUD

**Location:** `internal/authentik/provider.go:115-364`

`CreateOAuth2Provider` (123 lines) and `UpdateOAuth2Provider` (124 lines) share nearly identical logic for building request objects, looking up flows, resolving scopes, and handling property mappings. This violates DRY and increases maintenance burden — a bug fix in one path could easily be missed in the other.

**Recommendation:** Extract shared request-building logic into a private helper method.

#### HIGH: Template Validation Returns Nil Error

**Location:** `internal/controller/authentikapplication_controller.go:85-90`

```go
if err := template.ValidateTemplate(app.Spec.Secret.Template); err != nil {
    logger.Error(err, "invalid secret template")
    r.setCondition(ctx, app, metav1.ConditionFalse,
        authentikv1alpha1.ReasonTemplateError, fmt.Sprintf("Invalid secret template: %v", err))
    return ctrl.Result{}, nil  // Returns nil — masks the failure from controller-runtime
}
```

While the intent (don't requeue until CR is updated) is documented in a comment, returning `nil` means controller-runtime metrics won't track this as a failure. Consider returning a terminal error or using a dedicated "user error" pattern.

#### MEDIUM: Inconsistent Requeue/Error Return Patterns

**Location:** `internal/controller/authentikapplication_controller.go` — lines 66, 98, 107, 115, 139

Some error paths return `ctrl.Result{RequeueAfter: RequeueDelay}, nil` while others return `ctrl.Result{}, err`. The `nil` error with `RequeueAfter` hides failures from controller-runtime's error metrics and rate limiting. Returning the error alongside `RequeueAfter` is the recommended pattern.

#### MEDIUM: Status Update Errors Silently Dropped

**Location:** `internal/controller/authentikapplication_controller.go:356-358`

```go
func (r *AuthentikApplicationReconciler) setCondition(...) {
    // ...
    if err := r.Status().Update(ctx, app); err != nil {
        logger.Error(err, "failed to update status condition", ...)
        // Error is logged but not returned — caller cannot react
    }
}
```

If a status update fails, the caller has no way to know. This can lead to stale status conditions visible to users.

#### MEDIUM: Missing URL Validation at Startup

**Location:** `cmd/main.go:62-72`

`AUTHENTIK_URL` is checked for emptiness but not validated as a proper URL. A malformed URL would only fail later during API calls, making debugging harder.

```go
// Suggested improvement:
if _, err := url.Parse(authentikURL); err != nil {
    setupLog.Error(err, "AUTHENTIK_URL is not a valid URL")
    os.Exit(1)
}
```

#### MEDIUM: Readiness Check Creates New Client Per Probe

**Location:** `cmd/main.go:105-114`

Every readiness probe request creates a new `authentik.Client`. While functional, this is inefficient. Consider caching the client instance or the last health check result with a TTL.

#### LOW: Nil Pointer Risk in Enum Conversions

**Location:** `internal/authentik/provider.go:177-179, 303-305, 319-321`

Enum conversion results are nil-checked but the pattern is inconsistent. If `NewSubModeEnumFromValue` returns `nil` without error, the configuration is silently dropped.

#### LOW: No `missingkey=error` Option on Templates

**Location:** `internal/template/secret.go:46`

Templates are parsed without `Option("missingkey=error")`. Typos in template variable names (e.g., `{{ .CleintID }}` instead of `{{ .ClientID }}`) would silently render as empty strings rather than surfacing as errors.

---

## Testing

### Status: CRITICAL — Zero Test Coverage

**No test files (`*_test.go`) exist anywhere in the repository.** This is the single highest-priority finding in this audit.

While the test infrastructure is configured (Makefile `test` target, CI pipeline runs `go test ./...`, codecov integration, linter exclusions for test files), no actual tests have been written. CI always passes because there are no tests to fail.

### Impact

| Untested Area | Lines | Risk |
|---------------|-------|------|
| Controller reconciliation logic | 367 | HIGH — core business logic |
| Authentik API client (CRUD) | 689 | HIGH — external API interaction |
| Secret template rendering | 107 | HIGH — credential generation |
| API type helpers | 241 | LOW — simple helper methods |
| Main entrypoint | 126 | LOW — startup logic |

### Key Gaps

- **No unit tests** for any package
- **No integration tests** using envtest (controller-runtime test framework)
- **No mock interfaces** — the `Client` struct directly embeds `*api.APIClient`, making unit testing nearly impossible without HTTP mocking
- **No testdata directory** or test fixtures
- **No e2e tests** or acceptance test framework

### Recommendations

1. **Define interfaces** for the Authentik API client to enable dependency injection and mocking
2. **Start with `internal/template/`** — pure functions, easiest to test, highest value
3. **Add envtest-based controller tests** for reconciliation logic
4. **Use `httptest` package** to mock Authentik API responses for client tests
5. **Set a coverage target** (suggest 70%+ for initial milestone)

---

## Security

### Strengths

- **Distroless base image** (`gcr.io/distroless/static:nonroot`) — minimal attack surface
- **Non-root execution** (UID/GID 65532)
- **Read-only root filesystem** in container security context
- **All capabilities dropped** (`capabilities: { drop: [ALL] }`)
- **No privilege escalation** (`allowPrivilegeEscalation: false`)
- **Seccomp profile** set to `RuntimeDefault`
- **Secret references** — Authentik token loaded from Kubernetes Secret, never hardcoded
- **CGO disabled** in build — no C library dependencies
- **Multi-stage Docker build** — build tools not present in runtime image
- **`.gitignore`** properly excludes binaries and coverage files

### Areas for Improvement

#### MEDIUM: RBAC Scope — Cluster-Wide Secret Access

**Location:** `config/rbac/role.yaml`, `charts/authentik-operator/templates/clusterrole.yaml`

The operator's ClusterRole grants full CRUD on **all Secrets cluster-wide**:

```yaml
- apiGroups: [""]
  resources: [secrets]
  verbs: [create, delete, get, list, patch, update, watch]
```

If the operator is compromised, an attacker could read or modify any Secret in the cluster.

**Recommendation:**
- Document the security implications clearly
- Consider adding label-based filtering so the operator only manages Secrets it created (e.g., `app.kubernetes.io/managed-by: authentik-operator`)
- Evaluate whether namespace-scoped Roles could work for some deployment models

#### LOW: No Network Policies

No NetworkPolicy templates are included. In security-sensitive environments, restricting the operator's network access to only the Authentik API endpoint and the Kubernetes API server would reduce blast radius.

#### LOW: Missing SECURITY.md

No security policy or vulnerability disclosure process is documented. Adding a `SECURITY.md` with disclosure instructions is a GitHub best practice.

#### LOW: No Secret Rotation Documentation

While the operator creates secrets, there's no documented process for rotating the Authentik API token or the generated OAuth2 client credentials.

---

## Kubernetes Manifests & CRDs

### Strengths

- **Comprehensive CRD schema** with validation rules (`minLength`, `pattern`, `enum`, `minItems`)
- **Proper status subresource** with Kubernetes-standard conditions (type, status, reason, message, lastTransitionTime, observedGeneration)
- **Additional printer columns** for `kubectl get` output (Status, Application UID, Provider ID, Age)
- **Sensible defaults** — `clientType: confidential`, `subMode: hashed_user_id` (privacy-preserving), `scopes: [openid, profile, email]`
- **Two well-documented sample CRs** covering basic and advanced (custom template) use cases
- **Health probes** — liveness (`/healthz`) and readiness (`/readyz`) with Authentik connectivity verification
- **Leader election** enabled by default for HA
- **Prometheus integration** via optional ServiceMonitor

### Helm Chart Assessment

| Area | Status | Notes |
|------|--------|-------|
| Chart structure | Excellent | Proper Helm 3 layout |
| Security context | Excellent | Hardened defaults |
| Resource limits | Good | `cpu: 500m/10m`, `memory: 128Mi/64Mi` |
| Labels | Good | Kubernetes recommended labels |
| Configurability | Good | Key values exposed |
| CRD management | Acceptable | CRDs in `config/crd/bases/`, empty `crds/` in chart |

### Areas for Improvement

#### LOW: Missing PodDisruptionBudget

No PDB template is included. For production HA deployments, a PDB should ensure at least one replica remains available during node drains.

#### LOW: No Kustomize Support

The `config/` directory lacks `kustomization.yaml` files. Users who prefer Kustomize over Helm have no supported deployment path.

#### LOW: Missing MaxLength on Several Fields

CRD fields like `metaDescription`, `metaPublisher`, and `metaLaunchURL` have no length limits. While not critical, adding `maxLength` constraints prevents oversized values from reaching the Authentik API.

#### LOW: No Startup Probe

Only liveness and readiness probes are configured. A startup probe would prevent liveness failures during slow initial startup.

---

## CI/CD Pipeline

### Strengths

- **Two workflows:** `ci.yaml` (PR/push validation) and `release.yaml` (automated releases)
- **Comprehensive CI checks:** `gofmt`, `go vet`, `golangci-lint`, Helm lint, build, test, Docker build
- **Multi-arch builds** (amd64/arm64) via Docker buildx
- **Automated releases** via release-please with semantic versioning
- **OCI artifact publishing** for both Docker images and Helm charts to GHCR
- **Coverage reporting** via Codecov integration
- **Docker layer caching** via GitHub Actions cache

### Areas for Improvement

#### MEDIUM: CI Tests Are Effectively a No-Op

Since no test files exist, `go test ./...` always passes. The CI pipeline gives a false sense of validation. Consider adding a minimum coverage threshold check that fails CI if coverage drops below a target.

#### LOW: No Dependency Scanning

No automated vulnerability scanning (e.g., `govulncheck`, Dependabot security advisories, Trivy container scanning) is configured in CI. While Renovate bot keeps dependencies updated, proactive vulnerability detection would improve security posture.

#### LOW: No Integration Test Stage

CI only runs unit tests. A dedicated integration test stage (even with mocked dependencies) would catch issues that unit tests miss.

---

## Documentation & Project Health

### Strengths

- **Comprehensive README** covering installation (Helm), configuration, custom templates, CRD reference, local development, and contributing guidelines
- **Apache License 2.0** — appropriate for open-source operator
- **Proper `.gitignore`** excluding binaries, coverage files, and IDE artifacts
- **Release automation** with changelog generation via release-please
- **Good commit message quality** — descriptive, follows conventional commits style

### Areas for Improvement

#### MEDIUM: Missing LICENSE File

Apache License 2.0 is referenced in the README and in `hack/boilerplate.go.txt` (embedded in all Go source files), but no `LICENSE` file exists at the repository root. Most open-source tooling (GitHub license detection, `go-licenses`, FOSSA) expects a root-level LICENSE file.

#### MEDIUM: Missing CONTRIBUTING.md

Contributing guidelines are briefly mentioned in the README but not in a dedicated file. A `CONTRIBUTING.md` should cover:
- Development environment setup
- Code style expectations
- Test requirements for PRs
- PR review process

#### MEDIUM: Missing SECURITY.md

No vulnerability disclosure policy exists. GitHub recommends a `SECURITY.md` for all repositories.

#### LOW: No CODE_OF_CONDUCT.md

Standard for open-source projects to set community expectations.

#### LOW: No Architecture Documentation

The codebase is small enough to understand by reading, but a brief architecture doc explaining the reconciliation flow, API interactions, and secret lifecycle would help new contributors.

#### LOW: Missing GoDoc Comments on Exported Types

While the code has inline comments, exported functions and types in `internal/authentik/` lack comprehensive GoDoc-style comments.

#### LOW: No GitHub Issue/PR Templates

No `.github/ISSUE_TEMPLATE/` or `.github/pull_request_template.md` files exist. Templates help maintain consistency in bug reports, feature requests, and pull request descriptions.

---

## Findings Summary

### By Severity

| Severity | Count | Key Findings |
|----------|-------|-------------|
| **Critical** | 1 | Zero test coverage across entire codebase |
| **High** | 2 | Provider CRUD code duplication; template validation masks failures |
| **Medium** | 8 | Inconsistent error returns; status update errors dropped; RBAC scope; CI tests no-op; missing LICENSE file; missing CONTRIBUTING.md; missing SECURITY.md; readiness check inefficiency |
| **Low** | 11 | Missing network policies; no PDB; no kustomize support; no startup probe; no dependency scanning; no architecture docs; enum nil risks; template missingkey option; field length limits; no GoDoc; no issue/PR templates |

### By Category

| Category | Critical | High | Medium | Low |
|----------|----------|------|--------|-----|
| Code Quality | — | 2 | 3 | 2 |
| Testing | 1 | — | — | — |
| Security | — | — | 1 | 2 |
| Kubernetes/Helm | — | — | — | 3 |
| CI/CD | — | — | 1 | 2 |
| Documentation | — | — | 3 | 4 |

---

## Recommendations

### Priority 1 — Address Immediately

1. **Add unit tests** — Start with `internal/template/` (pure functions, highest ROI), then `internal/authentik/` client with HTTP mocking, then controller tests with envtest. Target 70%+ coverage.
2. **Define interfaces for the Authentik client** — Required to enable testability. Extract an interface from the concrete `Client` struct for dependency injection.
3. **Fix provider code duplication** — Extract shared request-building logic between `CreateOAuth2Provider` and `UpdateOAuth2Provider` to reduce maintenance risk.

### Priority 2 — Address Before v1.0

4. **Standardize error return patterns** in the controller — return errors alongside `RequeueAfter` so controller-runtime metrics accurately reflect failures.
5. **Add `SECURITY.md`** with vulnerability disclosure policy.
6. **Add `CONTRIBUTING.md`** with development setup, test requirements, and PR process.
7. **Add CI coverage threshold** — fail the build if coverage drops below a minimum.
8. **Add dependency/container scanning** to CI (e.g., `govulncheck`, Trivy).

### Priority 3 — Nice to Have

9. Add `template.Option("missingkey=error")` to catch template variable typos.
10. Validate `AUTHENTIK_URL` format at startup.
11. Cache the Authentik client in the readiness probe instead of creating a new one per request.
12. Add PodDisruptionBudget and NetworkPolicy Helm templates.
13. Add Kustomize support alongside Helm.
14. Add a startup probe to the deployment.
15. Document RBAC security implications and secret rotation strategy.

---

*This report was generated by an automated audit. All findings should be reviewed by a maintainer for accuracy and prioritization in the context of the project's roadmap.*
