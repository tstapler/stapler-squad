#!/usr/bin/env bash
# Shared driver for the 9 seeded failure fixtures (Task 0.1h). A fixture script
# defines: KEY (errors.yaml row it must trigger under Bazel), REMEDY_RE (regex
# a correct remedy line must match; empty when not applicable), MAKE_APPLICABLE
# (true|false), and functions seed(), run_make(), run_bazel(); then calls
# `fixture_main "$@"`.
#
#   <fixture>.sh --key            print KEY
#   <fixture>.sh --remedy-re      print REMEDY_RE
#   <fixture>.sh --make-applicable  print true|false
#   <fixture>.sh --run make|bazel <worktree>   seed in <worktree>, run the
#       command an agent would run, print its combined output, exit with its
#       status. The worktree must be a throwaway; this never touches the repo
#       checkout it is invoked from.
# bazel mode needs tools/bazel (Task 0.2f); without it exits 77 (skipped).

fixture_main() {
  case "${1:-}" in
    --key) echo "$KEY" ;;
    --remedy-re) echo "$REMEDY_RE" ;;
    --make-applicable) echo "$MAKE_APPLICABLE" ;;
    --run)
      local sys="$2" wt="$3"
      [[ -d "$wt" ]] || { echo "worktree $wt missing" >&2; exit 2; }
      cd "$wt" || exit 2
      seed
      if [[ "$sys" == make ]]; then
        if [[ "$MAKE_APPLICABLE" != true ]]; then echo "N/A-make"; exit 0; fi
        run_make 2>&1
      else
        [[ -x tools/bazel ]] || { echo "SKIP: tools/bazel wrapper not present (Task 0.2f)"; exit 77; }
        run_bazel 2>&1
      fi ;;
    *) echo "usage: $0 --key|--remedy-re|--make-applicable|--run make|bazel <worktree>" >&2; exit 2 ;;
  esac
}
