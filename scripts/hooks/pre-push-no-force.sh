#!/usr/bin/env bash
# git pre-push hook: reject non-fast-forward updates and deletion of main.
# stdin lines: <local ref> <local sha> <remote ref> <remote sha>
zero=0000000000000000000000000000000000000000
while read -r local_ref local_sha remote_ref remote_sha; do
  if [ "$local_sha" = "$zero" ]; then
    case "$remote_ref" in
      refs/heads/main|refs/heads/master)
        echo "pre-push: refusing to delete $remote_ref" >&2; exit 1 ;;
    esac
    continue
  fi
  [ "$remote_sha" = "$zero" ] && continue   # new branch
  if ! git merge-base --is-ancestor "$remote_sha" "$local_sha" 2>/dev/null; then
    echo "pre-push: refusing non-fast-forward update of $remote_ref ($local_ref); pull/merge instead" >&2
    exit 1
  fi
done
exit 0
