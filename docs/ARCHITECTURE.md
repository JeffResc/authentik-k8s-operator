# Architecture

## Component Overview

```
┌──────────────────────────────────────────────────┐
│                  Kubernetes                       │
│                                                   │
│  AuthentikOAuth2Application CR ──► Controller ──► Secret│
│                                   │               │
└───────────────────────────────────┼───────────────┘
                                    │
                                    ▼
                            ┌──────────────┐
                            │  Authentik    │
                            │  API (v3)    │
                            └──────────────┘
```

The operator watches `AuthentikOAuth2Application` custom resources and reconciles them against both the Authentik API and the Kubernetes API.

## Reconciliation Loop

Each reconciliation cycle follows this sequence:

1. **Fetch CR** — Read the `AuthentikOAuth2Application` from the Kubernetes API
2. **Check deletion** — If the CR is being deleted, run finalizer cleanup
3. **Add finalizer** — Ensure the finalizer is present for cleanup on delete
4. **Validate template** — Parse the secret template (if custom); reject with a condition if invalid
5. **Reconcile provider** — Create or update the OAuth2 provider in Authentik
6. **Reconcile application** — Create or update the application in Authentik
7. **Reconcile secret** — Fetch OIDC URLs, render the template, create or update the Kubernetes secret
8. **Update status** — Set the Ready condition and populate status fields

## Finalizer Lifecycle

1. **Add**: On first reconcile, the finalizer `goauthentik.io/finalizer` is added to the CR
2. **Cleanup**: When the CR is deleted, the finalizer triggers deletion of the Authentik application and provider
3. **Remove**: After cleanup succeeds, the finalizer is removed, allowing Kubernetes to complete deletion

The Kubernetes secret is cleaned up automatically via owner references (garbage collection).

## Secret Template Rendering

1. The controller collects OAuth2 credentials (client ID/secret) and OIDC endpoint URLs from Authentik
2. These are passed to a Go template (either the default or a user-provided one)
3. The template output is parsed as `key: value` lines
4. The resulting map becomes the secret's `data` field

## Requeue Strategy

- **Periodic**: Every 5 minutes for drift detection (ensures Authentik stays in sync)
- **Immediate on error**: Transient errors (API failures) return both `RequeueAfter` and the error
- **No requeue on user error**: Invalid templates don't requeue — the user must fix the CR

## Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/` | Entry point, manager setup, readiness probe |
| `api/v1alpha1/` | CRD type definitions |
| `internal/controller/` | Reconciliation logic |
| `internal/authentik/` | Authentik API client (providers, applications, health) |
| `internal/template/` | Secret template rendering |
