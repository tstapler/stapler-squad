#!/bin/sh
# Stand-in terminal program for the notification tray specs: fills the screen,
# prints one 140-column line (TY-11) and a scrollback marker, then leaves a shell.
# usage: tray-terminal-fixture.sh <marker>
seq 1 300
printf '%0140d\n' 0
echo "$1"
exec bash
