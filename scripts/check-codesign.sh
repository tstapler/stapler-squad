#!/bin/sh
# check-codesign.sh — Check whether the StaplerSquadDev signing cert exists
# and is trusted. Used by the Makefile _codesign-binary guard.
#
# Exit 0 if cert is found, exit 1 if not.

# macOS-only guard
[ "$(uname)" = "Darwin" ] || exit 0

if security find-identity -v -p codesigning | grep -q "StaplerSquadDev"; then
    exit 0
else
    exit 1
fi
