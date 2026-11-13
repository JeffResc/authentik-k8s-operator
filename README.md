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

Create an `AuthentikOAuth2Application` resource:

```yaml
apiVersion: goauthentik.io/v1alpha1
kind: AuthentikOAuth2Application
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

The operator does not adopt an OAuth2 provider or application that it did not create for this CR. If a provider named `my-app-provider` or an application with slug `my-app` already exists in Authentik, reconciliation fails. On deletion, the operator deletes only the application and provider recorded in the CR status.

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
kind: AuthentikOAuth2Application
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

### Groups

Create an `AuthentikGroup` resource to manage an Authentik user group:

```yaml
apiVersion: goauthentik.io/v1alpha1
kind: AuthentikGroup
metadata:
  name: platform-team
spec:
  name: "Platform Team"
  attributes:
    team: platform
  deletionPolicy: Retain
```

The operator does not manage group membership. Users added to the group in Authentik stay in the group.

#### Group ownership

The operator writes the attribute `goauthentik.io/owner: <namespace>/<name>` on each group that it creates. The operator changes or deletes only a group that has the owner value of the CR:

- If a group with the same name exists and does not have this owner value, the operator does not adopt it. The CR becomes `Ready=False` and the operator does not change the group.
- The operator does not trust `status.groupUid` alone. The group with that UID must also have the owner value.
- On deletion with `deletionPolicy: Delete`, the operator does not delete a group that does not have the owner value. It records a `NotOwned` event and removes the finalizer.

To bring an existing group under management (for example, a group that an earlier operator version created or adopted), an Authentik administrator must add the attribute `goauthentik.io/owner: <namespace>/<name>` to the group in Authentik. Do this only for groups that the CR is permitted to control.

#### Privileged fields

`isSuperuser`, `parents` and `roles` can grant Authentik privileges to group members. By default the operator rejects a CR that sets `isSuperuser: true` or a non-empty `parents` or `roles`. The CR becomes `Ready=False` with reason `ValidationFailed`. To permit these fields, set the Helm value `controller.allowGroupPrivilegedFields: true` (flag `--allow-group-privileged-fields`). Enable it only if every subject that can write `AuthentikGroup` resources is trusted as an Authentik administrator.

## Configuration Reference

### AuthentikOAuth2ApplicationSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Display name in Authentik |
| `slug` | string | No | `metadata.name` | URL-friendly identifier. Immutable: the API server rejects an update that adds, changes, or removes `slug`. To use a different slug, delete the resource and create it again. This rule also applies to `AuthentikSAMLApplication`. |
| `group` | string | No | - | Application group in Authentik |
| `policyEngineMode` | string | No | `any` | Policy evaluation mode (`all` or `any`) |
| `metaLaunchUrl` | string | No | - | Launch URL for the application |
| `metaDescription` | string | No | - | Application description |
| `metaPublisher` | string | No | - | Application publisher |
| `openInNewTab` | bool | No | `true` | Open in new browser tab |
| `groupBindings` | []string | No | - | Authentik group names allowed to access the app. When set, the operator owns the app's group bindings: it adds missing ones and removes others. Policy and user bindings are not changed. |
| `provider` | OAuth2ProviderSpec | Yes | - | OAuth2 provider configuration |
| `secret` | SecretSpec | No | - | Output secret configuration |

### OAuth2ProviderSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `authorizationFlow` | string | Yes | - | Slug of the authorization flow |
| `invalidationFlow` | string | Yes | - | Slug of the invalidation/logout flow |
| `redirectUris` | []string | Yes | - | Allowed redirect URIs |
| `scopes` | []string | No | `["openid","profile","email"]` | Scope names to enable (looked up by name) |
| `clientType` | string | No | `confidential` | `confidential` or `public` |
| `accessCodeValidity` | string | No | `minutes=1` | Access code lifetime |
| `accessTokenValidity` | string | No | `minutes=5` | Access token lifetime |
| `refreshTokenValidity` | string | No | `days=30` | Refresh token lifetime |
| `subMode` | string | No | `hashed_user_id` | Subject claim mode |
| `includeClaimsInIdToken` | bool | No | `true` | Include claims in ID token |
| `issuerMode` | string | No | `per_provider` | Issuer URL mode |
| `grantTypes` | []string | No | `["authorization_code","refresh_token"]` | Allowed OAuth2 grant types (authentik 2026.8+ rejects unlisted ones) |
| `propertyMappings` | []string | No | - | Additional property mapping UUIDs |
| `signingKey` | string | No | `authentik Self-signed Certificate` | Certificate name for signing tokens |

### SecretSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | `{metadata.name}-oauth` | Secret name |
| `labels` | map[string]string | No | - | Additional labels |
| `annotations` | map[string]string | No | - | Additional annotations |
| `template` | string | No | See above | Go template for secret data |

### AuthentikGroupSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | No | `metadata.name` | Group name in Authentik |
| `isSuperuser` | bool | No | `false` | Users in this group are superusers. Requires `--allow-group-privileged-fields` |
| `parents` | []string | No | - | Parent group names or UUIDs. Requires `--allow-group-privileged-fields` |
| `roles` | []string | No | - | RBAC role names or UUIDs. Requires `--allow-group-privileged-fields` |
| `attributes` | object | No | - | Free-form JSON attributes. The operator sets `goauthentik.io/owner` |
| `deletionPolicy` | string | No | `Delete` | `Delete` or `Retain` the Authentik group when the CR is deleted |

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
| `controller.allowGroupPrivilegedFields` | `false` | Allow `AuthentikGroup` resources to set `isSuperuser`, `parents` and `roles` |

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

## Security Considerations

### RBAC Scope

The operator's ClusterRole grants full CRUD permissions on Secrets cluster-wide. This is required because `AuthentikOAuth2Application` CRs can exist in any namespace and the operator must create/update secrets in those namespaces.

This means a compromised operator pod could read or modify any Secret in the cluster. To reduce blast radius:

- **Deploy in a dedicated namespace** (e.g., `authentik-operator-system`) with restricted access
- **Apply NetworkPolicies** to limit the operator's network egress to the Kubernetes API server and your Authentik instance only
- **Use a dedicated service account** (the Helm chart does this by default)

All secrets managed by the operator are labeled with `app.kubernetes.io/managed-by: authentik-operator` for easy identification and auditing.

## Secret Rotation

### Operator API Token

To rotate the Authentik API token used by the operator:

1. Create a new API token in Authentik
2. Update the Kubernetes secret referenced by `authentik.existingSecret.name`:
   ```bash
   kubectl create secret generic authentik-api-token \
     --namespace=authentik-operator-system \
     --from-literal=token=NEW_API_TOKEN \
     --dry-run=client -o yaml | kubectl apply -f -
   ```
3. Restart the operator pod to pick up the new token:
   ```bash
   kubectl rollout restart deployment -n authentik-operator-system
   ```

### OAuth2 Client Credentials

The OAuth2 client credentials (client ID and secret) in managed secrets are generated by Authentik when the provider is created. Currently, rotating these credentials requires:

1. Delete the `AuthentikOAuth2Application` CR
2. Re-create the CR — this provisions a new provider with fresh credentials
3. Update any applications consuming the secret

The operator will automatically update the Kubernetes secret with the new credentials.

## License

GNU General Public License v3.0
