#!/bin/bash
# Generates a self-signed TLS certificate for the webhook service.
# Run once before helm install. On helm upgrade use --reuse-values to keep the same cert.
#
# Usage: bash helm/generate-certs.sh [namespace]
# Default namespace: oracle-system

set -e

NAMESPACE="${1:-oracle-system}"
SERVICE_NAME="oracle-operator-webhook"
CERT_DIR="$(dirname "$0")/certs"

mkdir -p "$CERT_DIR"

echo "Generating CA..."
openssl genrsa -out "$CERT_DIR/ca.key" 2048 2>/dev/null
openssl req -x509 -new -nodes \
  -key "$CERT_DIR/ca.key" \
  -out "$CERT_DIR/ca.crt" \
  -days 3650 \
  -subj "/CN=oracle-operator-ca" 2>/dev/null

echo "Generating server certificate (SAN: ${SERVICE_NAME}.${NAMESPACE}.svc)..."
openssl genrsa -out "$CERT_DIR/tls.key" 2048 2>/dev/null

openssl req -new \
  -key "$CERT_DIR/tls.key" \
  -out "$CERT_DIR/tls.csr" \
  -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" 2>/dev/null

cat > "$CERT_DIR/san.cnf" << EOF
[v3_req]
subjectAltName = DNS:${SERVICE_NAME}.${NAMESPACE}.svc,DNS:${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

openssl x509 -req \
  -in "$CERT_DIR/tls.csr" \
  -CA "$CERT_DIR/ca.crt" \
  -CAkey "$CERT_DIR/ca.key" \
  -CAcreateserial \
  -out "$CERT_DIR/tls.crt" \
  -days 3650 \
  -extfile "$CERT_DIR/san.cnf" \
  -extensions v3_req 2>/dev/null

rm -f "$CERT_DIR/tls.csr" "$CERT_DIR/san.cnf" "$CERT_DIR/ca.srl"

TLS_CRT=$(openssl base64 -A -in "$CERT_DIR/tls.crt")
TLS_KEY=$(openssl base64 -A -in "$CERT_DIR/tls.key")
CA_CRT=$(openssl base64 -A -in "$CERT_DIR/ca.crt")

cat > "$(dirname "$0")/certs.yaml" << EOF
webhook:
  tls:
    crt: "${TLS_CRT}"
    key: "${TLS_KEY}"
    ca: "${CA_CRT}"
EOF

echo ""
echo "Done. Certificate written to helm/certs/"
echo "Values file written to helm/certs.yaml"
echo ""
echo "Next steps:"
echo "  1. Build and import images (see docs/HELM.md)"
echo "  2. helm install oracle-operator ~/oracle-operator-lab/helm/oracle-operator -f ~/oracle-operator-lab/helm/certs.yaml"
