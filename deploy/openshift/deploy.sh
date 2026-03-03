#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${NAMESPACE:-}" ]]; then
    echo "Error: NAMESPACE environment variable is required."
    echo "Usage: NAMESPACE=my-namespace bash deploy/openshift/deploy.sh"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Database credentials (override via env vars if desired)
DB_USER="${DB_USER:-scorecard}"
DB_PASSWORD="${DB_PASSWORD:-$(openssl rand -hex 24)}"
DB_NAME="${DB_NAME:-sankey_scorecard}"

echo "==> Deploying sankey-scorecard to namespace: ${NAMESPACE}"

# Verify oc context and switch namespace if needed
CURRENT_NS=$(oc project -q 2>/dev/null || true)
if [[ "${CURRENT_NS}" != "${NAMESPACE}" ]]; then
    echo "==> Switching to namespace: ${NAMESPACE}"
    oc project "${NAMESPACE}"
fi

# Config file (override via CONFIG_FILE env var)
CONFIG_FILE="${CONFIG_FILE:-config/sankey-scorecard.yaml}"
if [[ ! -f "${CONFIG_FILE}" ]]; then
    echo "Error: Config file not found at '${CONFIG_FILE}'."
    echo "Set CONFIG_FILE to point to your sankey-scorecard.yaml, or place it at config/sankey-scorecard.yaml"
    exit 1
fi

# Check that the Jira secret exists
if ! oc get secret sankey-scorecard-jira -n "${NAMESPACE}" &>/dev/null; then
    echo "Error: Secret 'sankey-scorecard-jira' not found in namespace '${NAMESPACE}'."
    echo "Create it with:"
    echo "  oc create secret generic sankey-scorecard-jira \\"
    echo "    --from-literal=jira-url=https://your-jira-instance.com \\"
    echo "    --from-literal=api-token=your-api-token \\"
    echo "    -n ${NAMESPACE}"
    exit 1
fi

# Create the database secret if it doesn't already exist
if ! oc get secret sankey-scorecard-db -n "${NAMESPACE}" &>/dev/null; then
    echo "==> Creating database secret 'sankey-scorecard-db'..."
    DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@sankey-scorecard-postgres:5432/${DB_NAME}?sslmode=disable"
    oc create secret generic sankey-scorecard-db \
        --from-literal=database-user="${DB_USER}" \
        --from-literal=database-password="${DB_PASSWORD}" \
        --from-literal=database-name="${DB_NAME}" \
        --from-literal=database-url="${DATABASE_URL}" \
        -n "${NAMESPACE}"
else
    echo "==> Database secret 'sankey-scorecard-db' already exists, skipping creation."
fi

# Create/update the config ConfigMap from the local file
echo "==> Creating/updating config ConfigMap from '${CONFIG_FILE}'..."
oc create configmap sankey-scorecard-config \
    --from-file=sankey-scorecard.yaml="${CONFIG_FILE}" \
    -n "${NAMESPACE}" \
    --dry-run=client -o yaml | oc apply -n "${NAMESPACE}" -f -

# Build and push the container image
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
echo "==> Building container image..."
make -C "${PROJECT_ROOT}" build-image

REGISTRY=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}' 2>/dev/null \
    || oc registry info 2>/dev/null | tail -1)
LOCAL_IMAGE="localhost/sankey-scorecard:${IMAGE_TAG:-latest}"
echo "==> Pushing ${LOCAL_IMAGE} to ${REGISTRY}/${NAMESPACE}/sankey-scorecard:latest..."
podman login --tls-verify=false "${REGISTRY}" -u "$(oc whoami)" -p "$(oc whoami -t)"
podman push --tls-verify=false \
    "${LOCAL_IMAGE}" \
    "${REGISTRY}/${NAMESPACE}/sankey-scorecard:latest"

# Apply PostgreSQL manifests first (order matters: PVC -> Deployment -> Service)
echo "==> Deploying PostgreSQL..."
for manifest in postgres-pvc.yaml postgres-deployment.yaml postgres-service.yaml; do
    oc apply -n "${NAMESPACE}" -f "${SCRIPT_DIR}/${manifest}"
done

# Wait for PostgreSQL to be ready before deploying the app
echo "==> Waiting for PostgreSQL rollout..."
oc rollout status deployment/sankey-scorecard-postgres -n "${NAMESPACE}" --timeout=120s

# Apply application manifests (deployment.yaml uses ${NAMESPACE} for the image path)
echo "==> Applying application manifests..."
envsubst '${NAMESPACE}' < "${SCRIPT_DIR}/deployment.yaml" | oc apply -n "${NAMESPACE}" -f -
for manifest in service.yaml route.yaml; do
    oc apply -n "${NAMESPACE}" -f "${SCRIPT_DIR}/${manifest}"
done

# Restart the deployment to pick up any updated image
echo "==> Restarting deployment..."
oc rollout restart deployment/sankey-scorecard -n "${NAMESPACE}"

# Wait for rollout
echo "==> Waiting for rollout..."
oc rollout status deployment/sankey-scorecard -n "${NAMESPACE}" --timeout=120s

# Print the route URL
ROUTE_URL=$(oc get route sankey-scorecard -n "${NAMESPACE}" --template='https://{{ .spec.host }}')
echo ""
echo "==> Deployment complete!"
echo "    URL: ${ROUTE_URL}"
echo ""
echo "    To trigger initial data load:"
echo "    curl -X POST ${ROUTE_URL}/api/refresh_data"
