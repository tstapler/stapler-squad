#!/usr/bin/env bash
# Fixture 9-cache-auth-rejected.sh: seeded failure for errors.yaml row E11.
# Bazel mode starts a local stub HTTP cache that answers every request 403.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E11
REMEDY_RE=''
MAKE_APPLICABLE=false
seed() { :; }
run_make() { :; }
run_bazel() {
  local portfile; portfile="$(mktemp)"
  python3 -I -c '
import http.server, sys
class H(http.server.BaseHTTPRequestHandler):
    def _deny(self): self.send_response(403); self.end_headers()
    do_GET = do_PUT = do_HEAD = _deny
    def log_message(self, *a): pass
s = http.server.HTTPServer(("127.0.0.1", 0), H)
open(sys.argv[1], "w").write(str(s.server_address[1]))
s.serve_forever()' "$portfile" &
  local srv=$!
  trap 'kill $srv 2>/dev/null' RETURN
  until [[ -s "$portfile" ]]; do sleep 0.1; done
  tools/bazel build //config:all --remote_cache="http://127.0.0.1:$(cat "$portfile")"
}
fixture_main "$@"
