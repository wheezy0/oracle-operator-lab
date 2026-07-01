#!/bin/bash
# Generates client certificates and kubeconfig files for team users.
# Must be run with sudo (needs access to k3s CA key).
# Usage: sudo bash create-users.sh

set -e

K3S_CA_CERT="/var/lib/rancher/k3s/server/tls/client-ca.crt"
K3S_CA_KEY="/var/lib/rancher/k3s/server/tls/client-ca.key"
K3S_SERVER="https://127.0.0.1:6443"
OUT_DIR="$(dirname "$0")"

# Get the k3s server CA for the kubeconfig (embedded from existing kubeconfig)
SERVER_CA=$(KUBECONFIG=/home/stefanb/.kube/config kubectl config view --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

generate_user() {
    local USERNAME=$1
    local NAMESPACE=$2

    echo "--- Generating user: $USERNAME (namespace: $NAMESPACE)"

    # Generate private key
    openssl genrsa -out "${OUT_DIR}/${USERNAME}.key" 2048 2>/dev/null

    # Generate CSR — CN=username, O=team (O is used as group in k8s)
    openssl req -new \
        -key "${OUT_DIR}/${USERNAME}.key" \
        -out "${OUT_DIR}/${USERNAME}.csr" \
        -subj "/CN=${USERNAME}/O=${NAMESPACE}" 2>/dev/null

    # Sign with k3s client CA
    openssl x509 -req \
        -in "${OUT_DIR}/${USERNAME}.csr" \
        -CA "${K3S_CA_CERT}" \
        -CAkey "${K3S_CA_KEY}" \
        -CAcreateserial \
        -out "${OUT_DIR}/${USERNAME}.crt" \
        -days 3650 2>/dev/null

    # Embed cert and key as base64
    local CLIENT_CERT=$(openssl base64 -A -in "${OUT_DIR}/${USERNAME}.crt")
    local CLIENT_KEY=$(openssl base64 -A -in "${OUT_DIR}/${USERNAME}.key")

    # Write kubeconfig
    cat > "${OUT_DIR}/${USERNAME}.kubeconfig" << EOF
apiVersion: v1
kind: Config
clusters:
  - name: k3s
    cluster:
      server: ${K3S_SERVER}
      certificate-authority-data: ${SERVER_CA}
contexts:
  - name: ${USERNAME}@k3s
    context:
      cluster: k3s
      user: ${USERNAME}
      namespace: ${NAMESPACE}
current-context: ${USERNAME}@k3s
users:
  - name: ${USERNAME}
    user:
      client-certificate-data: ${CLIENT_CERT}
      client-key-data: ${CLIENT_KEY}
EOF

    # Fix ownership so the regular user can read the files
    chown stefanb:stefanb "${OUT_DIR}/${USERNAME}.key" \
        "${OUT_DIR}/${USERNAME}.crt" \
        "${OUT_DIR}/${USERNAME}.csr" \
        "${OUT_DIR}/${USERNAME}.kubeconfig"

    # Remove CSR — not needed after signing
    rm -f "${OUT_DIR}/${USERNAME}.csr"

    echo "    kubeconfig: rbac/${USERNAME}.kubeconfig"
}

generate_user "finance-dba"  "team-finance"
generate_user "devops-dba"   "team-devops"

echo ""
echo "Done. Test with:"
echo "  kubectl --kubeconfig rbac/finance-dba.kubeconfig get oracledatabases -n team-finance"
echo "  kubectl --kubeconfig rbac/devops-dba.kubeconfig  get oracledatabases -n team-devops"
