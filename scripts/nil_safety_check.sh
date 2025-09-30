#!/bin/bash

# Nil Safety Analysis Script for Claude Squad
# This script performs automated static analysis to detect potential nil pointer vulnerabilities

set -e

echo "🔍 Claude Squad Nil Safety Analysis"
echo "=================================="

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

# Create results directory
mkdir -p analysis_results
RESULTS_FILE="analysis_results/nil_safety_$(date +%Y%m%d_%H%M%S).txt"

echo "Results will be saved to: $RESULTS_FILE"
echo "" | tee "$RESULTS_FILE"

# 1. Find direct pointer dereferences without nil checks
echo -e "${RED}🚨 CRITICAL: Direct pointer dereferences without nil checks${NC}" | tee -a "$RESULTS_FILE"
echo "Looking for patterns like: ptr.field or (*ptr) without preceding nil checks..." | tee -a "$RESULTS_FILE"

# Look for pointer dereferences that aren't preceded by nil checks within reasonable distance
find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | while read -r file; do
    # Find lines with pointer dereferences
    grep -n '\(\->\|\..*(\|.*\.\w\+\s*[^=!]\)' "$file" 2>/dev/null | while IFS=: read -r line_num content; do
        # Check if there's a nil check in the previous 5 lines
        start_line=$((line_num - 5))
        if [ $start_line -lt 1 ]; then start_line=1; fi
        
        nil_check=$(sed -n "${start_line},${line_num}p" "$file" | grep -E "(if.*!=.*nil|if.*==.*nil|if.*nil)" 2>/dev/null || true)
        
        if [ -z "$nil_check" ] && echo "$content" | grep -qE '\w+\.\w+\s*\(' 2>/dev/null; then
            echo "  $file:$line_num - $content" | tee -a "$RESULTS_FILE"
        fi
    done
done

echo "" | tee -a "$RESULTS_FILE"

# 2. Find channel operations without nil checks  
echo -e "${RED}🚨 CRITICAL: Channel operations without nil checks${NC}" | tee -a "$RESULTS_FILE"
echo "Looking for channel operations (close, send, receive) without nil checks..." | tee -a "$RESULTS_FILE"

find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | xargs grep -n "close(" | grep -v "if.*!= nil" | head -10 | tee -a "$RESULTS_FILE"

echo "" | tee -a "$RESULTS_FILE"

# 3. Find concurrent access to shared pointers
echo -e "${YELLOW}⚠️  HIGH RISK: Potential concurrent access issues${NC}" | tee -a "$RESULTS_FILE" 
echo "Looking for goroutines accessing shared state..." | tee -a "$RESULTS_FILE"

find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | xargs grep -n -A 3 -B 1 "go func\|goroutine" | grep -E "(\.|\->)" | head -10 | tee -a "$RESULTS_FILE"

echo "" | tee -a "$RESULTS_FILE"

# 4. Find error handling patterns that might leave nil pointers
echo -e "${YELLOW}⚠️  MEDIUM RISK: Error handling with potential nil states${NC}" | tee -a "$RESULTS_FILE"
echo "Looking for error returns that might leave objects partially initialized..." | tee -a "$RESULTS_FILE"

find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | xargs grep -n -A 2 "if err != nil" | grep "return.*nil" | head -10 | tee -a "$RESULTS_FILE"

echo "" | tee -a "$RESULTS_FILE"

# 5. Find good nil safety patterns (for reference)
echo -e "${GREEN}✅ GOOD PATTERNS: Existing nil safety practices${NC}" | tee -a "$RESULTS_FILE"
echo "Documenting good nil checking patterns found in codebase..." | tee -a "$RESULTS_FILE"

find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*" | xargs grep -n "if.*!= nil" | head -10 | tee -a "$RESULTS_FILE"

echo "" | tee -a "$RESULTS_FILE"

# 6. Use go vet for additional static analysis
echo -e "${BLUE}🔧 Running go vet for additional static analysis...${NC}" | tee -a "$RESULTS_FILE"
if go vet ./... 2>&1 | tee -a "$RESULTS_FILE"; then
    echo "go vet passed - no issues found" | tee -a "$RESULTS_FILE"
fi

echo "" | tee -a "$RESULTS_FILE"

# 7. Check for nilaway if available (Google's nil safety tool)
echo -e "${BLUE}🔧 Checking for nilaway (Google's nil safety analyzer)...${NC}" | tee -a "$RESULTS_FILE"
if command -v nilaway &> /dev/null; then
    echo "Running nilaway analysis..." | tee -a "$RESULTS_FILE"
    nilaway ./... 2>&1 | tee -a "$RESULTS_FILE" || true
else
    echo "nilaway not installed. Install with: go install go.uber.org/nilaway/cmd/nilaway@latest" | tee -a "$RESULTS_FILE"
fi

echo "" | tee -a "$RESULTS_FILE"
echo "==================================" | tee -a "$RESULTS_FILE"
echo -e "${GREEN}Analysis complete! Results saved to: $RESULTS_FILE${NC}"
echo ""
echo "📋 Summary of nil safety checks performed:"
echo "  1. Direct pointer dereferences without nil checks"
echo "  2. Channel operations without nil guards"  
echo "  3. Concurrent access to shared pointers"
echo "  4. Error handling that might leave nil pointers"
echo "  5. Documentation of existing good patterns"
echo "  6. Standard go vet static analysis"
echo "  7. Advanced nilaway analysis (if available)"
echo ""
echo -e "${YELLOW}💡 To install additional tools:${NC}"
echo "  go install go.uber.org/nilaway/cmd/nilaway@latest"
echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"

# Make the script executable
chmod +x "$0"