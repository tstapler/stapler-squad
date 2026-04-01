#!/bin/bash
# Bazel dependency automation script
# Automatically handles indirect->direct dependency conversions for Bazel

set -euo pipefail

echo "=== Bazel Dependency Automation ==="
echo "This script automates updating Bazel dependencies from go.mod"
echo ""

MAX_ITERATIONS=20
ITERATION=0

while [ $ITERATION -lt $MAX_ITERATIONS ]; do
    ITERATION=$((ITERATION + 1))
    echo ""
    echo "--- Iteration $ITERATION ---"
    
    # Run bazel mod tidy first
    echo "Running bazel mod tidy..."
    bazel mod tidy 2>&1 | tail -5
    
    # Try to build
    echo "Attempting build..."
    BUILD_OUTPUT=$(bazel build //:stapler-squad 2>&1 || true)
    
    # Check if build succeeded
    if echo "$BUILD_OUTPUT" | grep -q "Build completed successfully"; then
        echo ""
        echo "✅ Build successful!"
        break
    fi
    
    # Extract missing packages (portable: avoid grep -P and use awk instead)
    MISSING=$(echo "$BUILD_OUTPUT" | awk '/no such package/ && /unknown repo/ {
        n = split($0, parts, "'\''")
        if (n >= 2) print parts[2]
    }' | sort -u)
    
    if [ -z "$MISSING" ]; then
        echo "Build failed but no missing packages detected. Showing error:"
        echo "$BUILD_OUTPUT" | grep -i error | head -5
        break
    fi
    
    echo "Found missing dependencies:"
    echo "$MISSING"
    echo ""
    
    # Convert Bazel repo names to Go import paths
    for REPO in $MISSING; do
        # Convert repo name to Go import path
        # Examples:
        # com_github_foo_bar -> github.com/foo/bar
        # io_opentelemetry_foo -> go.opentelemetry.io/foo
        # org_golang_x_net -> golang.org/x/net
        
        case "$REPO" in
            com_github_*)
                GOPATH=$(echo "$REPO" | sed 's/com_github_/github.com\//' | sed 's/_/\//g')
                ;;
            io_opentelemetry_*)
                GOPATH=$(echo "$REPO" | sed 's/io_opentelemetry_/go.opentelemetry.io\//')
                ;;
            org_golang_google_protobuf*)
                SUFFIX="${REPO#org_golang_google_protobuf}"
                if [ -n "$SUFFIX" ]; then
                    GOPATH="google.golang.org/protobuf$(echo "$SUFFIX" | sed 's/_/\//g')"
                else
                    GOPATH="google.golang.org/protobuf"
                fi
                ;;
            org_golang_google_grpc*)
                SUFFIX="${REPO#org_golang_google_grpc}"
                if [ -n "$SUFFIX" ]; then
                    GOPATH="google.golang.org/grpc$(echo "$SUFFIX" | sed 's/_/\//g')"
                else
                    GOPATH="google.golang.org/grpc"
                fi
                ;;
            org_golang_x_*)
                GOPATH=$(echo "$REPO" | sed 's/org_golang_x_/golang.org\/x\//')
                ;;
            in_gopkg_*)
                rest=${REPO#in_gopkg_}
                version_suffix=""
                base="$rest"
                if echo "$rest" | grep -q '_v[0-9]\+$'; then
                    version_suffix=".${rest##*_}"
                    base=${rest%_*}
                fi
                GOPATH="gopkg.in/$(echo "$base" | sed 's/_/\//g')$version_suffix"
                ;;
            go_uber_*)
                GOPATH=$(echo "$REPO" | sed 's/go_uber_/go.uber.io\//')
                ;;
            cc_*)
                GOPATH=$(echo "$REPO" | sed 's/cc_//')
                ;;
            com_connectrpc_*)
                GOPATH=$(echo "$REPO" | sed 's/com_connectrpc_/connectrpc.com\//')
                ;;
            *)
                GOPATH="$REPO"
                ;;
        esac
        
        echo "Adding to go.mod: $GOPATH"
        
        # Add to go.mod as direct dependency
        # Use go get to add it (this will find the latest version)
        go get "$GOPATH@latest" 2>/dev/null || true
    done
done

if [ $ITERATION -eq $MAX_ITERATIONS ]; then
    echo ""
    echo "⚠️  Reached maximum iterations ($MAX_ITERATIONS)"
fi

echo ""
echo "=== Running final bazel mod tidy ==="
bazel mod tidy 2>&1 | tail -3

echo ""
echo "=== Running Gazelle ==="
bazel run //:gazelle 2>&1 | tail -3

echo ""
echo "=== Dependency update complete ==="
echo "Run 'bazel build //:stapler-squad' to verify"
