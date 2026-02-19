#!/bin/bash
# Generates a self-signed TLS certificate for local HTTPS development.
# Usage: ./scripts/generate-cert.sh [output_dir]
#
# The certificate is valid for localhost, 127.0.0.1, and ::1.
# Validity: 365 days.

set -euo pipefail

CERT_DIR="${1:-./certs}"

mkdir -p "$CERT_DIR"

if [[ -f "$CERT_DIR/server.crt" && -f "$CERT_DIR/server.key" ]]; then
    echo "Certificates already exist in $CERT_DIR — skipping generation."
    echo "  To regenerate, delete $CERT_DIR/server.crt and $CERT_DIR/server.key first."
    exit 0
fi

echo "Generating self-signed TLS certificate in $CERT_DIR ..."

MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' openssl req -x509 -newkey rsa:2048 \
    -keyout "$CERT_DIR/server.key" \
    -out "$CERT_DIR/server.crt" \
    -days 365 -nodes \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1"

echo "Done. Files created:"
echo "  Certificate: $CERT_DIR/server.crt"
echo "  Private key: $CERT_DIR/server.key"
echo ""
echo "To trust the certificate on macOS (optional, removes browser warnings):"
echo "  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain $CERT_DIR/server.crt"
