# Authentik Kubernetes Operator

[![CI](https://github.com/JeffResc/authentik-k8s-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/JeffResc/authentik-k8s-operator/actions/workflows/ci.yaml)
[![Release](https://github.com/JeffResc/authentik-k8s-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/JeffResc/authentik-k8s-operator/actions/workflows/release.yaml)

A Kubernetes operator for managing [Authentik](https://goauthentik.io/) applications. This operator allows you to declaratively create OAuth2/OIDC applications in Authentik and automatically generates Kubernetes secrets with the client credentials.

## Features

- **Declarative Application Management**: Define Authentik applications as Kubernetes Custom Resources
- **Automatic Secret Generation**: OAuth2 client credentials are automatically synced to Kubernetes secrets
- **Custom Secret Templates**: Use Go templates to customize the format of generated secrets
- **Drift Detection**: Periodic reconciliation ensures Authentik stays in sync with your CRs
- **Proper Cleanup**: Finalizers ensure Authentik resources are cleaned up when CRs are deleted

## Prerequisites

- Kubernetes cluster (1.26+)
- Authentik instance with API access
- Helm 3.x (for installation)

## Installation

### 1. Create Authentik API Token

First, create an API token in Authentik:

1. Log into Authentik as an admin
2. Go to **Directory** → **Tokens and App passwords**
3. Create a new token with appropriate permissions
4. Save the token value

### 2. Create Kubernetes Secret

Create a secret containing your Authentik API token:

```bash
kubectl create secret generic authentik-api-token \
  --namespace=authentik-operator-system \
  --from-literal=token=YOUR_API_TOKEN
```

### 3. Install the Operator

#### From OCI Registry (Recommended)

Install directly from the GitHub Container Registry:

```bash
helm install authentik-operator \
  oci://ghcr.io/jeffresc/charts/authentik-operator \
  --version 0.1.0 \
  --namespace authentik-operator-system \
  --create-namespace \
  --set authentik.url=https://authentik.example.com \
  --set authentik.existingSecret.name=authentik-api-token
```

#### From Source

If you're developing or want to install from source:

```bash
helm install authentik-operator ./charts/authentik-operator \
  --namespace authentik-operator-system \
  --create-namespace \
  --set authentik.url=https://authentik.example.com \
  --set authentik.existingSecret.name=authentik-api-token
```

## Usage

### Basic Example

Create an `AuthentikApplication` resource:

```yaml
apiVersion: goauthentik.io/v1alpha1
kind: AuthentikApplication
metadata:
  name: my-app
  namespace: default
spec:
  name: "My Application"
  provider:
    authorizationFlow: "default-provider-authorization-implicit-consent"
    invalidationFlow: "default-provider-invalidation-flow"
    redirectUris:
      - "https://my-app.example.com/callback"
```

This will:
1. Create an OAuth2 provider in Authentik named `my-app-provider`
2. Create an application in Authentik named "My Application" with slug `my-app`
3. Create a Kubernetes secret named `my-app-oauth` with the client credentials

### Retrieving Credentials

The generated secret contains:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-app-oauth
type: Opaque
data:
  client-id: <base64-encoded>
  client-secret: <base64-encoded>
  issuer-url: <base64-encoded>
  authorization-url: <base64-encoded>
  token-url: <base64-encoded>
  userinfo-url: <base64-encoded>
```

### Custom Secret Template

You can customize the secret format using Go templates:

```yaml
apiVersion: goauthentik.io/v1alpha1
kind: AuthentikApplication
metadata:
  name: my-app
spec:
  name: "My Application"
  provider:
    authorizationFlow: "default-provider-authorization-implicit-consent"
    invalidationFlow: "default-provider-invalidation-flow"
    redirectUris:
      - "https://my-app.example.com/callback"
  secret:
    template: |
      OIDC_CLIENT_ID: {{ .ClientID }}
      OIDC_CLIENT_SECRET: {{ .ClientSecret }}
      OIDC_ISSUER: {{ .IssuerURL }}
```

Available template variables:
- `.ClientID` - OAuth2 client ID
- `.ClientSecret` - OAuth2 client secret
- `.IssuerURL` - OIDC issuer URL
- `.AuthURL` - Authorization endpoint
- `.TokenURL` - Token endpoint
- `.UserInfoURL` - UserInfo endpoint
- `.LogoutURL` - End session/logout endpoint
- `.JWKSURL` - JWKS endpoint
- `.ProviderInfoURL` - OpenID Connect discovery URL (`.well-known/openid-configuration`)
- `.Slug` - Application slug
- `.Name` - Application display name

## Configuration Reference

### AuthentikApplicationSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Display name in Authentik |
| `slug` | string | No | `metadata.name` | URL-friendly identifier |
| `group` | string | No | - | Application group in Authentik |
| `policyEngineMode` | string | No | `any` | Policy evaluation mode (`all` or `any`) |
| `metaLaunchUrl` | string | No | - | Launch URL for the application |
| `metaDescription` | string | No | - | Application description |
| `metaPublisher` | string | No | - | Application publisher |
| `openInNewTab` | bool | No | `true` | Open in new browser tab |
| `provider` | OAuth2ProviderSpec | Yes | - | OAuth2 provider configuration |
| `secret` | SecretSpec | No | - | Output secret configuration |

### OAuth2ProviderSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `authorizationFlow` | string | Yes | - | Slug of the authorization flow |
| `invalidationFlow` | string | Yes | - | Slug of the invalidation/logout flow |
| `redirectUris` | []string | Yes | - | Allowed redirect URIs |
| `scopes` | []string | No | `["openid","profile","email"]` | OAuth2 scopes |
| `clientType` | string | No | `confidential` | `confidential` or `public` |
| `accessCodeValidity` | string | No | `minutes=1` | Access code lifetime |
| `accessTokenValidity` | string | No | `minutes=5` | Access token lifetime |
| `refreshTokenValidity` | string | No | `days=30` | Refresh token lifetime |
| `subMode` | string | No | `hashed_user_id` | Subject claim mode |
| `includeClaimsInIdToken` | bool | No | `true` | Include claims in ID token |
| `issuerMode` | string | No | `per_provider` | Issuer URL mode |
| `propertyMappings` | []string | No | - | Property mapping UUIDs |

### SecretSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | `{metadata.name}-oauth` | Secret name |
| `labels` | map[string]string | No | - | Additional labels |
| `annotations` | map[string]string | No | - | Additional annotations |
| `template` | string | No | See above | Go template for secret data |

## Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `authentik.url` | `""` | Authentik instance URL (required) |
| `authentik.existingSecret.name` | `""` | Secret containing API token (required) |
| `authentik.existingSecret.key` | `token` | Key in secret containing token |
| `replicaCount` | `1` | Number of operator replicas |
| `image.repository` | `ghcr.io/JeffResc/authentik-k8s-operator` | Image repository |
| `image.tag` | Chart appVersion | Image tag |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `leaderElection.enabled` | `true` | Enable leader election |

## Development

### Prerequisites

- Go 1.23+
- Make
- Docker (for building images)

### Building

```bash
# Generate code and manifests
make generate manifests

# Build binary
make build

# Build Docker image
make docker-build IMG=my-registry/authentik-operator:latest
```

### Running Locally

```bash
export AUTHENTIK_URL=https://authentik.example.com
export AUTHENTIK_TOKEN=your-api-token
make run
```

### Testing

```bash
make test
```

## CI/CD

This project uses GitHub Actions for continuous integration and releases.

### Pull Request Checks

On every PR, the following checks run:
- **Go Lint**: Runs `gofmt`, `go vet`, and `golangci-lint`
- **Helm Lint**: Validates the Helm chart
- **Build**: Compiles the binary and runs tests
- **Docker Build**: Verifies the container image builds successfully

### Releases

When a GitHub Release is published:
1. **Container Image**: Multi-arch image (amd64/arm64) is built and pushed to `ghcr.io/<owner>/authentik-operator`
2. **Helm Chart**: Packaged and pushed as OCI artifact to `ghcr.io/<owner>/charts/authentik-operator`

To create a release:
1. Create and push a tag: `git tag v0.1.0 && git push origin v0.1.0`
2. Create a GitHub Release from the tag
3. The release workflow will automatically publish artifacts

## License

Apache License 2.0
