package classifier

import (
	"testing"
)

// TestExtractInnerCommand verifies that wrapper flags are correctly skipped to
// locate the inner command for every program in recursiveEvalPrograms.
func TestExtractInnerCommand(t *testing.T) {
	tests := []struct {
		name string
		prog string
		args []string
		want string
	}{
		// ── xargs ──────────────────────────────────────────────────────────────
		{
			name: "xargs bare command",
			prog: "xargs", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "xargs -n short flag inline value",
			prog: "xargs", args: []string{"-n1", "git", "status"},
			want: "git status",
		},
		{
			name: "xargs -n two-token flag",
			prog: "xargs", args: []string{"-n", "1", "git", "status"},
			want: "git status",
		},
		{
			name: "xargs -I{} inline replacement",
			prog: "xargs", args: []string{"-I{}", "git", "status"},
			want: "git status",
		},
		{
			name: "xargs -I {} two-token replacement",
			prog: "xargs", args: []string{"-I", "{}", "git", "status"},
			want: "git status",
		},
		{
			name: "xargs -P parallel processes",
			prog: "xargs", args: []string{"-P", "4", "npm", "test"},
			want: "npm test",
		},
		{
			name: "xargs --null boolean flag",
			prog: "xargs", args: []string{"--null", "grep", "pattern"},
			want: "grep pattern",
		},
		{
			name: "xargs -0 boolean short flag",
			prog: "xargs", args: []string{"-0", "grep", "pattern"},
			want: "grep pattern",
		},
		{
			name: "xargs multiple flags",
			prog: "xargs", args: []string{"-n1", "-P4", "go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "xargs -- end of flags",
			prog: "xargs", args: []string{"--", "rm", "-rf", "/tmp/work"},
			want: "rm -rf /tmp/work",
		},
		{
			name: "xargs no inner command",
			prog: "xargs", args: []string{"-n1"},
			want: "",
		},
		{
			name: "xargs empty args",
			prog: "xargs", args: []string{},
			want: "",
		},
		{
			name: "xargs --max-args long flag with =",
			prog: "xargs", args: []string{"--max-args=5", "grep", "TODO"},
			want: "grep TODO",
		},
		{
			name: "xargs -a file flag",
			prog: "xargs", args: []string{"-a", "filelist.txt", "git", "add"},
			want: "git add",
		},
		// ── parallel ───────────────────────────────────────────────────────────
		{
			name: "parallel bare command",
			prog: "parallel", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "parallel -j jobs flag",
			prog: "parallel", args: []string{"-j", "4", "go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "parallel with ::: separator",
			prog: "parallel", args: []string{"git", "status", ":::", "repo1", "repo2"},
			want: "git status",
		},
		{
			name: "parallel ::: before command — no inner cmd",
			prog: "parallel", args: []string{":::", "git", "status"},
			want: "",
		},
		{
			name: "parallel no inner command",
			prog: "parallel", args: []string{"-j4"},
			want: "",
		},
		// ── timeout ────────────────────────────────────────────────────────────
		{
			name: "timeout bare duration + command",
			prog: "timeout", args: []string{"30", "git", "status"},
			want: "git status",
		},
		{
			name: "timeout with unit suffix",
			prog: "timeout", args: []string{"5m", "make", "build"},
			want: "make build",
		},
		{
			name: "timeout -k flag then duration then command",
			prog: "timeout", args: []string{"-k", "5s", "30", "git", "push"},
			want: "git push",
		},
		{
			name: "timeout --signal flag inline value",
			prog: "timeout", args: []string{"--signal=SIGINT", "10", "go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "timeout no command after duration",
			prog: "timeout", args: []string{"30"},
			want: "",
		},
		// ── nice ───────────────────────────────────────────────────────────────
		{
			name: "nice bare command",
			prog: "nice", args: []string{"make", "build"},
			want: "make build",
		},
		{
			name: "nice -n adjustment",
			prog: "nice", args: []string{"-n", "5", "go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "nice --adjustment= inline",
			prog: "nice", args: []string{"--adjustment=10", "make", "install"},
			want: "make install",
		},
		// ── stdbuf ────────────────────────────────────────────────────────────
		{
			name: "stdbuf -oL grep",
			prog: "stdbuf", args: []string{"-oL", "grep", "pattern"},
			want: "grep pattern",
		},
		{
			name: "stdbuf -o 0 grep",
			prog: "stdbuf", args: []string{"-o", "0", "grep", "pattern"},
			want: "grep pattern",
		},
		{
			name: "stdbuf --output=L go test",
			prog: "stdbuf", args: []string{"--output=L", "go", "test", "./..."},
			want: "go test ./...",
		},
		// ── xvfb-run ──────────────────────────────────────────────────────────
		{
			name: "xvfb-run bare command",
			prog: "xvfb-run", args: []string{"go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "xvfb-run -a boolean flag",
			prog: "xvfb-run", args: []string{"-a", "npm", "test"},
			want: "npm test",
		},
		{
			name: "xvfb-run -n display flag",
			prog: "xvfb-run", args: []string{"-n", "99", "make", "test"},
			want: "make test",
		},
		// ── ionice ────────────────────────────────────────────────────────────
		{
			name: "ionice -c class -n level command",
			prog: "ionice", args: []string{"-c", "2", "-n", "5", "git", "status"},
			want: "git status",
		},
		{
			name: "ionice bare command",
			prog: "ionice", args: []string{"make", "build"},
			want: "make build",
		},
		// ── setsid ────────────────────────────────────────────────────────────
		{
			name: "setsid bare command",
			prog: "setsid", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "setsid -w boolean flag",
			prog: "setsid", args: []string{"-w", "make", "test"},
			want: "make test",
		},
		// ── catchsegv ─────────────────────────────────────────────────────────
		{
			name: "catchsegv command",
			prog: "catchsegv", args: []string{"./myprogram", "--arg"},
			want: "./myprogram --arg",
		},
		// ── nohup ─────────────────────────────────────────────────────────────
		{
			name: "nohup bare command",
			prog: "nohup", args: []string{"make", "build"},
			want: "make build",
		},
		{
			name: "nohup command with args",
			prog: "nohup", args: []string{"go", "test", "./..."},
			want: "go test ./...",
		},
		// ── env ───────────────────────────────────────────────────────────────
		{
			name: "env bare command",
			prog: "env", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "env with VAR=val assignments",
			prog: "env", args: []string{"GIT_DIR=/tmp", "git", "status"},
			want: "git status",
		},
		{
			name: "env -u flag then command",
			prog: "env", args: []string{"-u", "HOME", "git", "status"},
			want: "git status",
		},
		{
			name: "env -i boolean then VAR=val then command",
			prog: "env", args: []string{"-i", "PATH=/usr/bin", "make", "build"},
			want: "make build",
		},
		// ── watch ─────────────────────────────────────────────────────────────
		{
			name: "watch bare command",
			prog: "watch", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "watch -n interval flag",
			prog: "watch", args: []string{"-n", "5", "go", "test", "./..."},
			want: "go test ./...",
		},
		{
			name: "watch -d boolean flag",
			prog: "watch", args: []string{"-d", "ls", "-la"},
			want: "ls -la",
		},
		// ── time ──────────────────────────────────────────────────────────────
		{
			name: "time bare command",
			prog: "time", args: []string{"make", "build"},
			want: "make build",
		},
		{
			name: "time -f format flag",
			prog: "time", args: []string{"-f", "%e", "go", "test", "./..."},
			want: "go test ./...",
		},
		// ── exec ──────────────────────────────────────────────────────────────
		{
			name: "exec bare command",
			prog: "exec", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "exec -a name flag",
			prog: "exec", args: []string{"-a", "myname", "git", "push"},
			want: "git push",
		},
		{
			name: "exec -c boolean flag",
			prog: "exec", args: []string{"-c", "make", "install"},
			want: "make install",
		},
		// ── command ───────────────────────────────────────────────────────────
		{
			name: "command bare command",
			prog: "command", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "command -p flag",
			prog: "command", args: []string{"-p", "ls", "-la"},
			want: "ls -la",
		},
		// ── sudo ──────────────────────────────────────────────────────────────
		{
			name: "sudo bare command",
			prog: "sudo", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "sudo -u user flag",
			prog: "sudo", args: []string{"-u", "root", "git", "status"},
			want: "git status",
		},
		{
			name: "sudo -n boolean flag",
			prog: "sudo", args: []string{"-n", "make", "install"},
			want: "make install",
		},
		// ── doas ──────────────────────────────────────────────────────────────
		{
			name: "doas bare command",
			prog: "doas", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "doas -u user command",
			prog: "doas", args: []string{"-u", "root", "git", "status"},
			want: "git status",
		},
		// ── run0 ──────────────────────────────────────────────────────────────
		{
			name: "run0 --user=root command",
			prog: "run0", args: []string{"--user=root", "git", "status"},
			want: "git status",
		},
		{
			name: "run0 bare command",
			prog: "run0", args: []string{"make", "install"},
			want: "make install",
		},
		// ── rtk ───────────────────────────────────────────────────────────────
		{
			name: "rtk bare command",
			prog: "rtk", args: []string{"git", "status"},
			want: "git status",
		},
		{
			name: "rtk proxy subcommand is skipped",
			prog: "rtk", args: []string{"proxy", "git", "status"},
			want: "git status",
		},
		{
			name: "rtk proxy risky command",
			prog: "rtk", args: []string{"proxy", "git", "push"},
			want: "git push",
		},
		// ── unknown program ────────────────────────────────────────────────────
		{
			name: "unknown program returns empty",
			prog: "sudo_unknown", args: []string{"git", "status"},
			want: "",
		},
		{
			name: "empty program",
			prog: "", args: []string{"git", "status"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractInnerCommand(tt.prog, tt.args)
			if got != tt.want {
				t.Errorf("ExtractInnerCommand(%q, %v) = %q, want %q", tt.prog, tt.args, got, tt.want)
			}
		})
	}
}
