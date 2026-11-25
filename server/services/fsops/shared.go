package fsops

import (
	"errors"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TruncateSize is the soft limit; text files larger than this are served
// truncated with is_truncated=true.
const TruncateSize = 1 * 1024 * 1024 // 1 MB

// WorkspaceProvider resolves workspace path information for a session.
// It is the consumer-side view of services.WorkspaceProvider.
type WorkspaceProvider interface {
	GetWorkspace(sessionID string) (domain.Workspace, error)
}

// InternalIOError logs the I/O cause server-side and returns a generic Internal error, so the wire never carries host detail.
func InternalIOError(op string, path ResolvedPath, cause error, clientMsg string) *connect.Error {
	log.Error("file I/O failed", "op", op, "path", string(path), "err", cause)
	return connect.NewError(connect.CodeInternal, errors.New(clientMsg))
}

// ReadFull reads up to len(buf) bytes from r. Returns bytes read and any non-EOF error.
func ReadFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

// KnownTextExtensions is the allowlist for extensions we know are always text.
// Files with these extensions skip the MIME and null-byte binary checks.
var KnownTextExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true, ".csv": true,
	".md": true, ".markdown": true, ".rst": true, ".txt": true, ".text": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".bat": true, ".cmd": true,
	".c": true, ".cpp": true, ".cc": true, ".cxx": true, ".h": true, ".hpp": true,
	".java": true, ".kt": true, ".kts": true, ".scala": true, ".groovy": true,
	".rb": true, ".rs": true, ".swift": true, ".m": true, ".mm": true,
	".php": true, ".lua": true, ".r": true, ".R": true, ".pl": true, ".pm": true,
	".sql": true, ".graphql": true, ".gql": true, ".proto": true,
	".tf": true, ".tfvars": true, ".hcl": true, ".Dockerfile": true, ".dockerfile": true,
	".makefile": true, ".mk": true, ".env": true, ".envrc": true,
	".gitignore": true, ".gitattributes": true, ".editorconfig": true,
	".mod": true, ".sum": true, ".lock": true,
	".log": true, ".diff": true, ".patch": true,
}
