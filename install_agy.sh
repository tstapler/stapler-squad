#!/bin/bash
set -e
echo "Downloading Antigravity CLI installation script..."
curl -fsSL https://antigravity.google/cli/install.sh -o /tmp/agy_install.sh
echo "Executing installation script..."
bash /tmp/agy_install.sh
