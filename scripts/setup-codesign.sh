#!/bin/sh
# setup-codesign.sh — Create a self-signed code-signing certificate for
# stable TCC identity. Run once per developer machine.
#
# Requires OpenSSL (not LibreSSL). If macOS's default openssl is LibreSSL,
# install real OpenSSL first:
#   brew install openssl
# Then either add it to PATH or run:
#   OPENSSL_BIN=$(brew --prefix openssl)/bin/openssl make setup-codesign

set -e

# 1. Check macOS
[ "$(uname)" = "Darwin" ] || { echo "macOS only"; exit 0; }

# 2. Skip if cert already exists (idempotent)
if security find-identity -v -p codesigning | grep -q "StaplerSquadDev"; then
    echo "StaplerSquadDev cert already present."
    exit 0
fi

# PREREQUISITE: Requires OpenSSL (not LibreSSL). macOS ships LibreSSL which
# lacks -addext support. Check and error early if LibreSSL is detected.
# Resolve OPENSSL first so OPENSSL_BIN override is respected in the check.
OPENSSL="${OPENSSL_BIN:-openssl}"
if "$OPENSSL" version | grep -q LibreSSL; then
    echo "ERROR: Requires OpenSSL (not LibreSSL). Run: brew install openssl"
    echo "Then either add Homebrew openssl to PATH or set OPENSSL_BIN:"
    echo "  OPENSSL_BIN=\$(brew --prefix openssl)/bin/openssl make setup-codesign"
    exit 1
fi

# 3. Generate cert and key with openssl
# No -extensions v3_req (avoids duplicate keyUsage from v3_req defaults).
# No keyEncipherment (RSA key transport, not needed for code signing).
"$OPENSSL" req -x509 -newkey rsa:2048 -keyout /tmp/staplerdev.key -out /tmp/staplerdev.crt \
    -days 7300 -nodes \
    -subj "/CN=StaplerSquadDev/O=StaplerSquadDev" \
    -addext "keyUsage=critical,digitalSignature" \
    -addext "extendedKeyUsage=codeSigning"

# Include -name so the private key label in keychain matches "StaplerSquadDev"
# (required for set-key-partition-list -l filter to work correctly)
"$OPENSSL" pkcs12 -export -out /tmp/StaplerSquadDev.p12 \
    -inkey /tmp/staplerdev.key -in /tmp/staplerdev.crt \
    -passout pass:"" -name "StaplerSquadDev"

# 4. Import into login keychain (-T grants codesign access at import time)
security import /tmp/StaplerSquadDev.p12 \
    -k ~/Library/Keychains/login.keychain-db \
    -P "" -T /usr/bin/codesign

# 5. Set trust: user domain only (no -d flag; -d = admin domain, requires sudo)
# User-domain trust is sufficient for codesign on behalf of the current user.
security add-trusted-cert -r trustRoot \
    -k ~/Library/Keychains/login.keychain-db \
    /tmp/staplerdev.crt

# 6. Set key partition list so codesign never prompts (no -k flag; omitting -k
# causes security to use the currently-unlocked keychain without a password arg,
# which is correct for the login keychain on a logged-in developer machine)
security set-key-partition-list \
    -S "apple-tool:,apple:,codesign:" \
    -s -l "StaplerSquadDev" \
    ~/Library/Keychains/login.keychain-db

# 7. Clean up temp files
rm -f /tmp/staplerdev.key /tmp/staplerdev.crt /tmp/StaplerSquadDev.p12

echo "StaplerSquadDev certificate created and trusted successfully."
echo "Run 'make verify-codesign' after 'make install-service' to confirm signing."
