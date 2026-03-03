#!/usr/bin/env bash
set -euo pipefail

PIPELINE_NS="rosa-dashboard--pipeline"
RUNTIME_NS="rosa-dashboard--runtime-int"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Granting image-puller access to ${RUNTIME_NS}..."
oc policy add-role-to-group system:image-puller \
    "system:serviceaccounts:${RUNTIME_NS}" \
    -n "${PIPELINE_NS}"

echo "==> Applying pipeline ServiceAccount..."
oc apply -f "${SCRIPT_DIR}/serviceaccount.yaml"

echo "==> Linking builder secret to pipeline ServiceAccount..."
BUILDER_SECRET=$(oc get sa builder -n "${PIPELINE_NS}" -o jsonpath='{.secrets[0].name}')
oc secrets link pipeline "${BUILDER_SECRET}" -n "${PIPELINE_NS}" --for=pull,mount 2>/dev/null || true

echo "==> Applying Pipeline..."
oc apply -f "${SCRIPT_DIR}/pipeline.yaml"

echo ""
echo "==> Setup complete. To trigger a build:"
echo "    oc create -f deploy/openshift/pipeline/pipelinerun.yaml"
