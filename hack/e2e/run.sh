#!/usr/bin/env bash
# E2E test script for authentik-k8s-operator.
# Expects to run inside a k3d cluster with the operator image already loaded.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

AUTHENTIK_NAMESPACE="authentik"
OPERATOR_NAMESPACE="operator-system"
AUTHENTIK_TOKEN="e2e-test-token-for-operator"
PORT_FORWARD_PID=""

# --- Helpers ---

cleanup() {
    echo "=== Cleaning up ==="
    if [ -n "${PORT_FORWARD_PID}" ]; then
        kill "${PORT_FORWARD_PID}" 2>/dev/null || true
    fi
}
trap cleanup EXIT

dump_debug() {
    echo ""
    echo "=== DEBUG: Operator logs ==="
    kubectl logs -n "${OPERATOR_NAMESPACE}" deployment/authentik-operator --tail=200 2>/dev/null || true
    echo ""
    echo "=== DEBUG: Authentik server logs ==="
    kubectl logs -n "${AUTHENTIK_NAMESPACE}" -l app.kubernetes.io/name=authentik -c authentik --tail=100 2>/dev/null || true
    echo ""
    echo "=== DEBUG: CR status ==="
    kubectl get authentikoauth2application -A -o yaml 2>/dev/null || true
    kubectl get authentiksamlapplication -A -o yaml 2>/dev/null || true
    echo ""
    echo "=== DEBUG: Events ==="
    kubectl get events -A --sort-by=.lastTimestamp --no-headers 2>/dev/null | tail -50 || true
    echo ""
    echo "=== DEBUG: All pods ==="
    kubectl get pods -A 2>/dev/null || true
}
trap 'dump_debug; cleanup' ERR

wait_for() {
    local description="$1"
    local command="$2"
    local retries="${3:-60}"
    local interval="${4:-5}"

    echo "Waiting for ${description}..."
    for i in $(seq 1 "${retries}"); do
        if eval "${command}" >/dev/null 2>&1; then
            echo "OK: ${description}"
            return 0
        fi
        if [ "$i" -eq "${retries}" ]; then
            echo "FAIL: ${description} (timed out after $((retries * interval))s)"
            return 1
        fi
        sleep "${interval}"
    done
}

assert_not_empty() {
    local name="$1"
    local value="$2"
    if [ -z "${value}" ]; then
        echo "FAIL: ${name} is empty"
        exit 1
    fi
    echo "OK: ${name} = ${value}"
}

# --- Phase A: Deploy Authentik ---

echo "=== Phase A: Deploying Authentik ==="
helm repo add authentik https://charts.goauthentik.io/
helm repo update authentik

kubectl create namespace "${AUTHENTIK_NAMESPACE}"
kubectl create secret generic authentik-bootstrap-secret \
    --namespace "${AUTHENTIK_NAMESPACE}" \
    --from-literal=AUTHENTIK_SECRET_KEY="e2e-test-secret-key-not-for-production-use" \
    --from-literal=AUTHENTIK_BOOTSTRAP_PASSWORD="e2e-admin-password" \
    --from-literal=AUTHENTIK_BOOTSTRAP_TOKEN="${AUTHENTIK_TOKEN}"

helm install authentik authentik/authentik \
    --namespace "${AUTHENTIK_NAMESPACE}" \
    -f "${SCRIPT_DIR}/authentik-values.yaml" \
    --timeout 10m \
    --wait

echo "Authentik Helm install complete."

# --- Phase B: Wait for Authentik readiness ---

echo "=== Phase B: Waiting for Authentik readiness ==="

# Start port-forward
kubectl port-forward -n "${AUTHENTIK_NAMESPACE}" svc/authentik-server 9000:80 &
PORT_FORWARD_PID=$!
sleep 3

wait_for "Authentik health endpoint" \
    "curl -sf http://localhost:9000/-/health/ready/" \
    60 5

# Wait for default flows to be seeded (Authentik creates these asynchronously after startup)
wait_for "default authorization flow" \
    "curl -sf http://localhost:9000/api/v3/flows/instances/default-provider-authorization-implicit-consent/ -H 'Authorization: Bearer ${AUTHENTIK_TOKEN}'" \
    30 5

wait_for "default invalidation flow" \
    "curl -sf http://localhost:9000/api/v3/flows/instances/default-provider-invalidation-flow/ -H 'Authorization: Bearer ${AUTHENTIK_TOKEN}'" \
    30 5

echo "Authentik is fully ready."

# --- Phase C: Deploy operator ---

echo "=== Phase C: Deploying operator ==="

kubectl create namespace "${OPERATOR_NAMESPACE}"
kubectl create secret generic authentik-operator-token \
    --namespace "${OPERATOR_NAMESPACE}" \
    --from-literal=token="${AUTHENTIK_TOKEN}"

# Copy CRDs into Helm chart
make -C "${REPO_ROOT}" helm-crds

helm install authentik-operator "${REPO_ROOT}/charts/authentik-operator" \
    --namespace "${OPERATOR_NAMESPACE}" \
    --set image.repository=authentik-operator \
    --set image.tag=e2e \
    --set image.pullPolicy=Never \
    --set authentik.url="http://authentik-server.${AUTHENTIK_NAMESPACE}.svc.cluster.local" \
    --set authentik.existingSecret.name=authentik-operator-token \
    --set authentik.existingSecret.key=token \
    --set leaderElection.enabled=false \
    --set logging.development=true \
    --timeout 5m \
    --wait

kubectl rollout status deployment/authentik-operator \
    -n "${OPERATOR_NAMESPACE}" --timeout=120s

echo "Operator is running."

# --- Phase D: Apply CR and verify creation ---

echo "=== Phase D: Applying sample CR ==="

kubectl apply -f "${REPO_ROOT}/config/samples/authentik_v1alpha1_authentikoauth2application.yaml"

# Wait for Ready condition
wait_for "AuthentikOAuth2Application Ready" \
    "[ \"\$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}')\" = 'True' ]" \
    60 5

echo ""
echo "--- Verifying K8s Secret ---"

# Verify secret exists and has expected keys
SECRET_KEYS=("client-id" "client-secret" "issuer-url" "authorization-url" "token-url" "userinfo-url")
for KEY in "${SECRET_KEYS[@]}"; do
    VALUE=$(kubectl get secret sample-app-oauth -n default -o jsonpath="{.data.${KEY}}" | base64 -d)
    assert_not_empty "secret key '${KEY}'" "${VALUE}"
done

echo ""
echo "--- Verifying CR status ---"

APP_UID=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.applicationUid}')
assert_not_empty "applicationUid" "${APP_UID}"

PROVIDER_ID=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.providerId}')
assert_not_empty "providerId" "${PROVIDER_ID}"
if [ "${PROVIDER_ID}" -le 0 ] 2>/dev/null; then
    echo "FAIL: providerId should be > 0, got ${PROVIDER_ID}"
    exit 1
fi

SECRET_NAME=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.secretName}')
if [ "${SECRET_NAME}" != "sample-app-oauth" ]; then
    echo "FAIL: secretName expected 'sample-app-oauth', got '${SECRET_NAME}'"
    exit 1
fi
echo "OK: secretName = ${SECRET_NAME}"

CLIENT_ID=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.clientId}')
assert_not_empty "clientId" "${CLIENT_ID}"

echo ""
echo "--- Verifying Authentik API ---"

# Verify application exists in Authentik
APP_NAME=$(curl -sf "http://localhost:9000/api/v3/core/applications/sample-app/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.name')
if [ "${APP_NAME}" != "Sample Application" ]; then
    echo "FAIL: Authentik application name expected 'Sample Application', got '${APP_NAME}'"
    exit 1
fi
echo "OK: Authentik application name = ${APP_NAME}"

# Verify provider exists in Authentik
PROVIDER_NAME=$(curl -sf "http://localhost:9000/api/v3/providers/oauth2/${PROVIDER_ID}/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.name')
if [ "${PROVIDER_NAME}" != "sample-app-provider" ]; then
    echo "FAIL: Authentik provider name expected 'sample-app-provider', got '${PROVIDER_NAME}'"
    exit 1
fi
echo "OK: Authentik provider name = ${PROVIDER_NAME}"

echo ""
echo "=== Phase D: Creation verification PASSED ==="

# --- Phase D2: Update CR and verify reconciliation ---

echo ""
echo "=== Phase D2: Updating CR and verifying update ==="

# Capture the current observedGeneration
OLD_GEN=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.observedGeneration}')

# Patch the CR: add metaDescription and a new redirect URI
kubectl patch authentikoauth2application sample-app -n default --type='merge' -p '{
  "spec": {
    "metaDescription": "Updated by e2e test",
    "provider": {
      "redirectUris": [
        "https://sample-app.example.com/callback",
        "https://sample-app.example.com/oauth/callback",
        "https://sample-app.example.com/new-callback"
      ]
    }
  }
}'

# Wait for observedGeneration to increment (controller processed the update)
wait_for "observedGeneration to increment" \
    "[ \"\$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.observedGeneration}')\" -gt \"${OLD_GEN}\" ]" \
    60 5

# Verify Ready condition is still True after update
READY=$(kubectl get authentikoauth2application sample-app -n default -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
if [ "${READY}" != "True" ]; then
    echo "FAIL: Ready condition expected 'True' after update, got '${READY}'"
    exit 1
fi
echo "OK: Ready condition still True after update"

echo ""
echo "--- Verifying Authentik application updated ---"

# Verify metaDescription was updated in Authentik
APP_DESC=$(curl -sf "http://localhost:9000/api/v3/core/applications/sample-app/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.meta_description')
if [ "${APP_DESC}" != "Updated by e2e test" ]; then
    echo "FAIL: Authentik meta_description expected 'Updated by e2e test', got '${APP_DESC}'"
    exit 1
fi
echo "OK: Authentik meta_description = ${APP_DESC}"

echo ""
echo "--- Verifying Authentik provider updated ---"

# Verify redirect URIs were updated (should now have 3)
REDIRECT_COUNT=$(curl -sf "http://localhost:9000/api/v3/providers/oauth2/${PROVIDER_ID}/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq '.redirect_uris | length')
if [ "${REDIRECT_COUNT}" -ne 3 ]; then
    echo "FAIL: Expected 3 redirect URIs, got ${REDIRECT_COUNT}"
    exit 1
fi
echo "OK: Provider has ${REDIRECT_COUNT} redirect URIs"

echo ""
echo "--- Verifying secret still valid after update ---"

for KEY in "${SECRET_KEYS[@]}"; do
    VALUE=$(kubectl get secret sample-app-oauth -n default -o jsonpath="{.data.${KEY}}" | base64 -d)
    assert_not_empty "secret key '${KEY}' after update" "${VALUE}"
done

echo ""
echo "=== Phase D2: Update verification PASSED ==="

# --- Phase E: Delete CR and verify cleanup ---

echo ""
echo "=== Phase E: Deleting CR and verifying cleanup ==="

kubectl delete authentikoauth2application sample-app -n default

# Wait for CR to be fully deleted (finalizer must complete)
wait_for "CR deletion" \
    "! kubectl get authentikoauth2application sample-app -n default 2>/dev/null" \
    30 5

# Wait for K8s garbage collection of the secret (owner reference)
wait_for "secret garbage collection" \
    "! kubectl get secret sample-app-oauth -n default 2>/dev/null" \
    30 5
echo "OK: Secret was garbage collected"

# Verify application deleted from Authentik
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "http://localhost:9000/api/v3/core/applications/sample-app/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}")
if [ "${HTTP_CODE}" != "404" ]; then
    echo "FAIL: Authentik application still exists (HTTP ${HTTP_CODE})"
    exit 1
fi
echo "OK: Authentik application deleted (404)"

# Verify provider deleted from Authentik
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "http://localhost:9000/api/v3/providers/oauth2/${PROVIDER_ID}/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}")
if [ "${HTTP_CODE}" != "404" ]; then
    echo "FAIL: Authentik provider still exists (HTTP ${HTTP_CODE})"
    exit 1
fi
echo "OK: Authentik provider deleted (404)"

echo ""
echo "=== Phase E: Deletion verification PASSED ==="

# --- Phase G: SAML application lifecycle ---

echo ""
echo "=== Phase G: SAML application lifecycle ==="

kubectl apply -f "${REPO_ROOT}/config/samples/authentik_v1alpha1_authentiksamlapplication.yaml"

# Wait for Ready condition
wait_for "AuthentikSAMLApplication Ready" \
    "[ \"\$(kubectl get authentiksamlapplication sample-saml-app -n default -o jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}')\" = 'True' ]" \
    60 5

echo ""
echo "--- Verifying SAML K8s Secret ---"

# Verify secret exists and has a metadata key
METADATA_VALUE=$(kubectl get secret sample-saml-app-saml -n default -o jsonpath='{.data.metadata}' | base64 -d)
assert_not_empty "secret key 'metadata'" "${METADATA_VALUE}"

echo ""
echo "--- Verifying SAML CR status ---"

SAML_APP_UID=$(kubectl get authentiksamlapplication sample-saml-app -n default -o jsonpath='{.status.applicationUid}')
assert_not_empty "SAML applicationUid" "${SAML_APP_UID}"

SAML_PROVIDER_ID=$(kubectl get authentiksamlapplication sample-saml-app -n default -o jsonpath='{.status.providerId}')
assert_not_empty "SAML providerId" "${SAML_PROVIDER_ID}"
if [ "${SAML_PROVIDER_ID}" -le 0 ] 2>/dev/null; then
    echo "FAIL: SAML providerId should be > 0, got ${SAML_PROVIDER_ID}"
    exit 1
fi

SAML_SECRET_NAME=$(kubectl get authentiksamlapplication sample-saml-app -n default -o jsonpath='{.status.secretName}')
if [ "${SAML_SECRET_NAME}" != "sample-saml-app-saml" ]; then
    echo "FAIL: SAML secretName expected 'sample-saml-app-saml', got '${SAML_SECRET_NAME}'"
    exit 1
fi
echo "OK: SAML secretName = ${SAML_SECRET_NAME}"

echo ""
echo "--- Verifying SAML Authentik API ---"

# Verify application exists in Authentik
SAML_APP_NAME=$(curl -sf "http://localhost:9000/api/v3/core/applications/sample-saml-app/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.name')
if [ "${SAML_APP_NAME}" != "Sample SAML Application" ]; then
    echo "FAIL: Authentik SAML application name expected 'Sample SAML Application', got '${SAML_APP_NAME}'"
    exit 1
fi
echo "OK: Authentik SAML application name = ${SAML_APP_NAME}"

# Verify SAML provider exists in Authentik
SAML_PROVIDER_NAME=$(curl -sf "http://localhost:9000/api/v3/providers/saml/${SAML_PROVIDER_ID}/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.name')
if [ "${SAML_PROVIDER_NAME}" != "sample-saml-app-provider" ]; then
    echo "FAIL: Authentik SAML provider name expected 'sample-saml-app-provider', got '${SAML_PROVIDER_NAME}'"
    exit 1
fi
echo "OK: Authentik SAML provider name = ${SAML_PROVIDER_NAME}"

echo ""
echo "--- Deleting SAML CR and verifying cleanup ---"

kubectl delete authentiksamlapplication sample-saml-app -n default

wait_for "SAML CR deletion" \
    "! kubectl get authentiksamlapplication sample-saml-app -n default 2>/dev/null" \
    30 5

wait_for "SAML secret garbage collection" \
    "! kubectl get secret sample-saml-app-saml -n default 2>/dev/null" \
    30 5
echo "OK: SAML secret was garbage collected"

# Verify SAML application deleted from Authentik
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "http://localhost:9000/api/v3/core/applications/sample-saml-app/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}")
if [ "${HTTP_CODE}" != "404" ]; then
    echo "FAIL: Authentik SAML application still exists (HTTP ${HTTP_CODE})"
    exit 1
fi
echo "OK: Authentik SAML application deleted (404)"

# Verify SAML provider deleted from Authentik
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "http://localhost:9000/api/v3/providers/saml/${SAML_PROVIDER_ID}/" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}")
if [ "${HTTP_CODE}" != "404" ]; then
    echo "FAIL: Authentik SAML provider still exists (HTTP ${HTTP_CODE})"
    exit 1
fi
echo "OK: Authentik SAML provider deleted (404)"

echo ""
echo "=== Phase G: SAML lifecycle verification PASSED ==="

# --- Phase F: Event webhook integration ---

echo ""
echo "=== Phase F: Event webhook integration ==="

# Upgrade operator with event webhook enabled
helm upgrade authentik-operator "${REPO_ROOT}/charts/authentik-operator" \
    --namespace "${OPERATOR_NAMESPACE}" \
    --set image.repository=authentik-operator \
    --set image.tag=e2e \
    --set image.pullPolicy=Never \
    --set authentik.url="http://authentik-server.${AUTHENTIK_NAMESPACE}.svc.cluster.local" \
    --set authentik.existingSecret.name=authentik-operator-token \
    --set authentik.existingSecret.key=token \
    --set leaderElection.enabled=false \
    --set logging.development=true \
    --set eventWebhook.enabled=true \
    --set eventWebhook.port=9444 \
    --timeout 5m \
    --wait

kubectl rollout status deployment/authentik-operator \
    -n "${OPERATOR_NAMESPACE}" --timeout=120s

echo "Operator upgraded with event webhook enabled."

# Wait for the operator to register the webhook in Authentik
sleep 10

echo "--- Verifying Authentik notification transport ---"

TRANSPORT_COUNT=$(curl -sf "http://localhost:9000/api/v3/events/transports/?name=authentik-k8s-operator-webhook" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq '.results | length')
if [ "${TRANSPORT_COUNT}" -ne 1 ]; then
    echo "FAIL: Expected 1 notification transport, got ${TRANSPORT_COUNT}"
    exit 1
fi
echo "OK: Notification transport registered"

TRANSPORT_URL=$(curl -sf "http://localhost:9000/api/v3/events/transports/?name=authentik-k8s-operator-webhook" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq -r '.results[0].webhook_url')
if [ -z "${TRANSPORT_URL}" ] || [ "${TRANSPORT_URL}" = "null" ]; then
    echo "FAIL: Transport webhook_url is empty"
    exit 1
fi
echo "OK: Transport webhook URL = ${TRANSPORT_URL}"

echo "--- Verifying Authentik notification rule ---"

RULE_COUNT=$(curl -sf "http://localhost:9000/api/v3/events/rules/?name=authentik-k8s-operator-model-events" \
    -H "Authorization: Bearer ${AUTHENTIK_TOKEN}" | jq '.results | length')
if [ "${RULE_COUNT}" -ne 1 ]; then
    echo "FAIL: Expected 1 notification rule, got ${RULE_COUNT}"
    exit 1
fi
echo "OK: Notification rule registered"

echo "--- Verifying event webhook receiver responds ---"

# Port-forward to the operator's event webhook
kubectl port-forward -n "${OPERATOR_NAMESPACE}" svc/authentik-operator-events 9444:9444 &
EVENT_PF_PID=$!
sleep 3

HTTP_CODE=$(curl -sf -o /dev/null -w "%{http_code}" \
    -X POST http://localhost:9444/webhook \
    -H "Content-Type: application/json" \
    -d '{"body":"e2e test event","severity":"notice"}')
if [ "${HTTP_CODE}" != "200" ]; then
    echo "FAIL: Event webhook returned HTTP ${HTTP_CODE}, expected 200"
    kill "${EVENT_PF_PID}" 2>/dev/null || true
    exit 1
fi
echo "OK: Event webhook receiver responded 200"

kill "${EVENT_PF_PID}" 2>/dev/null || true

echo ""
echo "=== Phase F: Event webhook verification PASSED ==="

echo ""
echo "========================================="
echo "  E2E TEST PASSED"
echo "========================================="
