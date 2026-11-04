#!/usr/bin/env python3
"""Deterministic PTY peer: real readline editing plus opt-in raw/alternate-screen modes."""
import hashlib
import os
import readline
import sys
import termios
import tty


def emit(message):
    print(message, flush=True)


if "libedit" in (readline.__doc__ or ""):
    readline.parse_and_bind("bind -e")
else:
    for binding in ["set editing-mode emacs", "set enable-bracketed-paste on", '"\\e[H": beginning-of-line', '"\\e[F": end-of-line', '"\\eOH": beginning-of-line', '"\\eOF": end-of-line']:
        readline.parse_and_bind(binding)

sequence = 0
emit(f"FIXTURE_READY pid={os.getpid()}")
while True:
    try:
        line = input("edit> ")
    except KeyboardInterrupt:
        emit(f"\nINTERRUPTED pid={os.getpid()}")
        continue
    except EOFError:
        # EOF is acknowledged without exiting, so Ctrl+D cannot revive a replacement process.
        emit(f"\nEOF_ACK pid={os.getpid()}")
        continue
    if line == ":ready":
        emit(f"FIXTURE_READY pid={os.getpid()}")
    elif line == ":history":
        for number in range(1, 401):
            emit(f"HISTORY-{number:04d}")
        emit("HISTORY_END")
    elif line == ":dims":
        size = os.get_terminal_size()
        emit(f"DIMS {size.columns}x{size.lines}")
    elif line == ":raw":
        saved = termios.tcgetattr(sys.stdin)
        tty.setraw(sys.stdin)
        sys.stdout.write("\x1b[?1049h\x1b[2J\x1b[H\x1b[?2004hRAW_READY\r\n")
        sys.stdout.flush()
        received = bytearray()
        try:
            while True:
                byte = os.read(sys.stdin.fileno(), 1)
                if byte == b"\x1d":
                    break
                received.extend(byte)
                # Print each byte separately so output never depends on read chunking.
                sys.stdout.write(f"{byte.hex()} ")
                sys.stdout.flush()
        finally:
            termios.tcsetattr(sys.stdin, termios.TCSANOW, saved)
            sys.stdout.write("\x1b[?2004l\x1b[?1049l")
            sys.stdout.flush()
        emit("RAW_ACK " + received.hex())
    else:
        sequence += 1
        digest = hashlib.sha256(line.encode()).hexdigest()[:16]
        emit(f"ACK {sequence} pid={os.getpid()} chars={len(line)} sha={digest}")
