#!/bin/sh
# Worst-case terminal content for the MC-1 contrast check (notification-contrast-over-xterm.spec.ts).
# usage: mc1-terminal-content.sh <brightfill|whitebg|brightwhitebg|yellowbg|syntax>
mode="$1"
printf '\033[?25l'
line() { awk -v n="$1" -v c="$2" 'BEGIN { s = ""; for (i = 0; i < n; i++) s = s c; printf "%s", s }'; }
i=0
while [ "$i" -lt 160 ]; do
  case "$mode" in
    brightfill)    printf '\033[40;1;97m'; line 300 W ;;
    whitebg)       printf '\033[47;30m'; line 300 ' ' ;;
    brightwhitebg) printf '\033[107;97m'; line 300 ' ' ;;
    yellowbg)      printf '\033[103;93m'; line 300 '#' ;;
    syntax)
      printf '\033[35mfunc \033[33mrender\033[0m(\033[36mctx\033[0m \033[34mContext\033[0m) \033[32m"string literal"\033[0m \033[90m// comment\033[0m \033[41;97m ERROR \033[0m \033[42;30m OK \033[0m \033[7m reverse \033[0m \033[94mbright-blue\033[0m \033[91mbright-red\033[0m \033[93mbright-yellow\033[0m'
      ;;
  esac
  printf '\033[0m\n'
  i=$((i + 1))
done
exec sleep 86400
