#!/bin/bash
#
# Flow Control High-Throughput Stress Test
#
# This script generates large volumes of terminal output to test:
# 1. Watermark-based flow control (HIGH/LOW thresholds)
# 2. ANSI escape sequence integrity (no split control codes)
# 3. Browser stability under load
# 4. Terminal responsiveness during backpressure
#
# Prerequisites:
# - Debug mode enabled: localStorage.setItem('debug-terminal', 'true')
# - Browser console open (F12) to monitor flow control logs
#
# Reference: https://xtermjs.org/docs/guides/flowcontrol/

set -e

echo "==================================="
echo "Flow Control Stress Test Suite"
echo "==================================="
echo ""
echo "Prerequisites:"
echo "  1. Enable debug mode: localStorage.setItem('debug-terminal', 'true')"
echo "  2. Open browser console (F12)"
echo "  3. Watch for [FlowControl] logs"
echo ""
echo "Press Enter to start..."
read

# Test 1: Plain Text Volume (500KB)
echo ""
echo "Test 1: Plain Text Volume (500KB)"
echo "Expected: HIGH_WATERMARK warnings, then LOW_WATERMARK resumes"
echo "---"
for i in {1..5000}; do
    # Each line ~100 bytes = 500KB total
    echo "Line $i: Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor."
done
echo "✓ Test 1 complete"
sleep 2

# Test 2: Colored Output (ANSI SGR codes)
echo ""
echo "Test 2: Colored Output with ANSI Codes"
echo "Expected: Colors render correctly, no garbled escape sequences"
echo "---"
for i in {1..1000}; do
    case $((i % 5)) in
        0) echo -e "\x1b[31mRed line $i\x1b[0m" ;;
        1) echo -e "\x1b[32mGreen line $i\x1b[0m" ;;
        2) echo -e "\x1b[33mYellow line $i\x1b[0m" ;;
        3) echo -e "\x1b[34mBlue line $i\x1b[0m" ;;
        4) echo -e "\x1b[35mMagenta line $i\x1b[0m" ;;
    esac
done
echo "✓ Test 2 complete"
sleep 2

# Test 3: Progress Bar Animation (Line Rewriting)
echo ""
echo "Test 3: Progress Bar Animation"
echo "Expected: Smooth animation, no visual glitches"
echo "---"
for i in {0..100}; do
    # Calculate progress bar
    filled=$((i / 2))
    empty=$((50 - filled))
    bar=$(printf '=%.0s' $(seq 1 $filled))$(printf ' %.0s' $(seq 1 $empty))

    # Clear line, cursor home, colored output
    printf "\x1b[2K\r\x1b[34m[%3d%%]\x1b[0m %s" "$i" "$bar"

    # 60fps = ~16ms per frame
    sleep 0.016
done
printf "\n"
echo "✓ Test 3 complete"
sleep 2

# Test 4: Cursor Positioning (CSI sequences)
echo ""
echo "Test 4: Cursor Positioning"
echo "Expected: Grid pattern appears correctly"
echo "---"
for row in {1..20}; do
    for col in {1..60}; do
        if [ $((col % 10)) -eq 0 ]; then
            printf "\x1b[%d;%dH*" "$row" "$col"
        fi
    done
done
printf "\x1b[22;1H"  # Move cursor to safe position
echo "✓ Test 4 complete"
sleep 2

# Test 5: Mixed Content (Text + Colors + Cursor)
echo ""
echo "Test 5: Mixed Content"
echo "Expected: All formatting preserved, smooth rendering"
echo "---"
for i in {1..500}; do
    case $((i % 4)) in
        0)
            # Colored text
            echo -e "\x1b[3$((RANDOM % 8))m$(printf 'x%.0s' {1..80})\x1b[0m"
            ;;
        1)
            # Cursor save/restore
            echo -e "Before\x1b[sSAVED\x1b[uAfter"
            ;;
        2)
            # Plain text
            echo "Plain text line $i with some content for volume testing"
            ;;
        3)
            # Bold + underline
            echo -e "\x1b[1m\x1b[4mBold Underline $i\x1b[0m"
            ;;
    esac
done
echo "✓ Test 5 complete"
sleep 2

# Test 6: Rapid Small Writes
echo ""
echo "Test 6: Rapid Small Writes (10000 lines)"
echo "Expected: Smooth streaming, no visual lag"
echo "---"
for i in {1..10000}; do
    echo "Rapid line $i"
done
echo "✓ Test 6 complete"
sleep 2

# Test 7: Large Single Write (Dictionary)
echo ""
echo "Test 7: Large Single Write (Dictionary Dump)"
echo "Expected: Multiple pause/resume cycles"
echo "---"
if [ -f /usr/share/dict/words ]; then
    cat /usr/share/dict/words
    echo "✓ Test 7 complete"
else
    # Fallback: generate large text
    for i in {1..5000}; do
        echo "Fallback line $i: $(printf 'x%.0s' {1..80})"
    done
    echo "✓ Test 7 complete (fallback)"
fi
sleep 2

# Test 8: OSC Sequences (Title Changes)
echo ""
echo "Test 8: OSC Sequences"
echo "Expected: No split OSC sequences in logs"
echo "---"
for i in {1..100}; do
    # Set terminal title (OSC sequence)
    printf "\x1b]0;Terminal Title %d\x07" "$i"
    echo "Setting title $i"
    sleep 0.05
done
echo "✓ Test 8 complete"
sleep 2

# Test 9: Complex ANSI SGR (RGB Colors)
echo ""
echo "Test 9: 24-bit RGB Colors"
echo "Expected: True color rendering, no sequence corruption"
echo "---"
for i in {0..255}; do
    # RGB gradient
    r=$i
    g=$((255 - i))
    b=$((i / 2))
    printf "\x1b[38;2;%d;%d;%dmRGB($r,$g,$b)\x1b[0m " "$r" "$g" "$b"

    if [ $((i % 8)) -eq 7 ]; then
        echo ""
    fi
done
echo ""
echo "✓ Test 9 complete"
sleep 2

# Test 10: Stress - Combined Load
echo ""
echo "Test 10: Combined Stress Test"
echo "Expected: System remains stable, all metrics normal"
echo "---"
for round in {1..5}; do
    echo "Round $round/5..."

    # Mix of all previous tests
    for i in {1..200}; do
        case $((i % 6)) in
            0) echo -e "\x1b[3$((RANDOM % 8))mColored $i\x1b[0m" ;;
            1) printf "\x1b[2K\r\x1b[34mProgress: %d%%\x1b[0m" "$((i / 2))" ;;
            2) echo "Plain text line $i: $(printf 'x%.0s' {1..50})" ;;
            3) printf "\x1b[%d;10H*\n" "$((i % 20 + 1))" ;;
            4) echo -e "\x1b[1m\x1b[4mBold $i\x1b[0m" ;;
            5) printf "\x1b]0;Title $i\x07"; echo "Title update" ;;
        esac
    done

    echo "Round $round complete"
done
echo "✓ Test 10 complete"

echo ""
echo "==================================="
echo "All Tests Complete!"
echo "==================================="
echo ""
echo "Review browser console for:"
echo "  • [FlowControl] HIGH WATERMARK EXCEEDED messages"
echo "  • [FlowControl] LOW WATERMARK REACHED messages"
echo "  • Watermark values cycling between 0-100KB"
echo "  • paused: true/false state changes"
echo ""
echo "Check for:"
echo "  ✓ No browser tab crash"
echo "  ✓ All colors rendered correctly"
echo "  ✓ No visible escape sequences (e.g., '[31m')"
echo "  ✓ Terminal remains responsive"
echo "  ✓ Smooth rendering throughout"
echo ""
