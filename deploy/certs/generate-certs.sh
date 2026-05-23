#!/bin/bash
# What this does:
# Generates self-signed TLS certificates for the webhook server.
#
# The certificate chain:
#   1. CA (Certificate Authority) — signs the server cert
#   2. Server cert — used by the webhook HTTPS server
#
# The server cert's SAN (Subject Alternative Name) MUST match:
#   topology-webhook.topology-system.svc
#
# This is because the API server resolves the webhook service to this DNS name
# and validates it against the cert's SAN during the TLS handshake.

set -euo pipefail

CERT_DIR="deploy/certs"
SERVICE="topology-webhook"
NAMESPACE="topology-system"
# The full DNS name that the API server will use to reach our webhook
FQDN="${SERVICE}.${NAMESPACE}.svc"

mkdir -p "${CERT_DIR}"
cd "${CERT_DIR}"

echo "=== Generating CA ==="
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes \
    -key ca.key \
    -subj "/CN=Topology Webhook CA" \
    -days 365 \
    -out ca.crt

echo "=== Generating Server Key ==="
openssl genrsa -out tls.key 2048

echo "=== Creating CSR with SAN ==="
# The SAN (Subject Alternative Name) is critical.
# Without it, the API server rejects the TLS handshake.
cat > csr.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
DNS.4 = ${SERVICE}.${NAMESPACE}.svc.cluster.local
EOF

openssl req -new \
    -key tls.key \
    -subj "/CN=${FQDN}" \
    -out server.csr \
    -config csr.conf

echo "=== Signing Server Cert with CA ==="
openssl x509 -req \
    -in server.csr \
    -CA ca.crt \
    -CAkey ca.key \
    -CAcreateserial \
    -out tls.crt \
    -days 365 \
    -extensions v3_req \
    -extfile csr.conf

echo "=== Verifying Certificate ==="
openssl verify -CAfile ca.crt tls.crt
openssl x509 -in tls.crt -noout -text | grep -A1 "Subject Alternative Name"

echo ""
echo "✅ Certificates generated successfully in ${CERT_DIR}/"
echo "   ca.crt  — CA certificate (inject into MutatingWebhookConfiguration caBundle)"
echo "   tls.crt — Server certificate"
echo "   tls.key — Server private key"

# Clean up intermediate files
rm -f server.csr csr.conf ca.key ca.srl
