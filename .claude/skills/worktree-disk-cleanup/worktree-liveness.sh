#!/usr/bin/env bash
# Usage: worktree-liveness.sh <worktree-path>
# Prints one line: "<VERDICT> <path> <reasons...>". Exit 0 always.
#   LIVE        a process runs inside it, or an Active/Creating/Hibernated/Restoring
#               stapler-squad session points at it. Never remove.
#   CONFIGURED  a non-archived Paused/Stopped session still references it. Keep until the user OKs.
#   RECENT      Claude transcript or HEAD reflog touched within RECENT_HOURS (default 2). Keep.
#   IDLE        no signal found. Eligible for the normal Step 2 classification.
# Cloud/remote Claude sessions run in their own checkouts and never write to local worktrees,
# so they are not detectable (or relevant) here.
set -u
wt=$(readlink -f "${1:?worktree path}")
recent_secs=$(( ${RECENT_HOURS:-2} * 3600 ))
live=() configured=() recent=()

# 1. Processes whose cwd is inside the worktree (Claude Code, shells, dev servers, tmux panes' children).
for p in /proc/[0-9]*; do
  cwd=$(readlink "$p/cwd" 2>/dev/null) || continue
  case "$cwd" in "$wt"|"$wt"/*) live+=("pid $(basename "$p") $(tr '\0\n' '  ' <"$p/cmdline" 2>/dev/null | cut -c1-60)");; esac
done

# 2. stapler-squad sessions (every state DB: shared, instances/*, workspaces/*).
# Status ints: 0 Creating, 1 Active, 2 Paused, 3 Stopped, 4 Hibernated, 5+ see session/instance.go.
esc=${wt//\'/\'\'}
while IFS= read -r db; do
  [ -s "$db" ] || continue
  rows=$(sqlite3 -readonly "$db" "
    SELECT s.title||':'||s.status||':'||(s.archived_at IS NOT NULL) FROM sessions s
    LEFT JOIN worktrees w ON w.session_worktree = s.id
    WHERE w.worktree_path = '$esc' OR s.existing_worktree = '$esc' OR s.path = '$esc'" 2>/dev/null) || continue
  while IFS=: read -r title status archived; do
    [ -n "$title" ] || continue
    [ "$archived" = 1 ] && continue
    case "$status" in
      0|1|4|5) live+=("ssq session '$title' status=$status ($db)");;
      *) configured+=("ssq session '$title' status=$status");;
    esac
  done <<<"$rows"
done < <(find ~/.stapler-squad -maxdepth 3 -name sessions.db 2>/dev/null)

# 3. Recent activity: Claude Code transcripts for this cwd, and the worktree's HEAD reflog.
now=$(date +%s)
proj=~/.claude/projects/$(printf %s "$wt" | sed 's/[^A-Za-z0-9]/-/g')
if [ -d "$proj" ]; then
  newest=$(find "$proj" -maxdepth 1 -name '*.jsonl' -printf '%T@\n' 2>/dev/null | sort -n | tail -1 | cut -d. -f1)
  [ -n "${newest:-}" ] && [ $(( now - newest )) -lt "$recent_secs" ] && recent+=("claude transcript touched $(( (now-newest)/60 ))m ago")
fi
gd=$(git -C "$wt" rev-parse --git-dir 2>/dev/null) && [ -f "$gd/logs/HEAD" ] && {
  m=$(stat -c %Y "$gd/logs/HEAD"); [ $(( now - m )) -lt "$recent_secs" ] && recent+=("HEAD reflog $(( (now-m)/60 ))m ago")
}

if   [ ${#live[@]} -gt 0 ];       then echo "LIVE $wt ${live[*]}"
elif [ ${#configured[@]} -gt 0 ]; then echo "CONFIGURED $wt ${configured[*]}"
elif [ ${#recent[@]} -gt 0 ];     then echo "RECENT $wt ${recent[*]}"
else echo "IDLE $wt"; fi
