package secretscan

import (
	"strings"
	"testing"
)

func TestScanForSecrets_NoFalsePositives(t *testing.T) {
	t.Parallel()
	// Shell variable references must NOT be flagged as plaintext secrets.
	safeCommands := []struct {
		name string
		cmd  string
	}{
		{
			name: "Gradle flyway with RDS token variable (quoted)",
			cmd:  `./gradlew flywayRepair -Pflyway.url="jdbc:postgresql://host:5432/db" -Pflyway.user="fbg_migrations" -Pflyway.password="$RDS_TOKEN" -Pflyway.cleanDisabled=true`,
		},
		{
			name: "Flyway password as shell variable (unquoted)",
			cmd:  `./gradlew flywayRepair -Pflyway.password=$RDS_TOKEN`,
		},
		{
			name: "PGPASSWORD as variable reference",
			cmd:  `PGPASSWORD=$DB_PASS psql -h host -U user -d db`,
		},
		{
			name: "password env var with quoted variable reference",
			cmd:  `PASSWORD="$SECRET_VALUE" some-command`,
		},
		{
			name: "DB_PASSWORD with brace variable",
			cmd:  `DB_PASSWORD="${SOME_VAR}" ./start.sh`,
		},
		{
			name: "CLI flag --password with variable",
			cmd:  `myapp --password "$DB_PASS"`,
		},
		{
			name: "aws rds generate-db-auth-token full command",
			cmd: `RDS_TOKEN=$(aws-vault exec profile -- aws rds generate-db-auth-token --hostname host.rds.amazonaws.com --port 5432 --username fbg_migrations --region us-east-2)
./gradlew flywayRepair -Pflyway.url="jdbc:postgresql://host:5432/scorecards" -Pflyway.user="fbg_migrations" -Pflyway.password="$RDS_TOKEN" -Pflyway.cleanDisabled=true`,
		},
		{
			name: "password with command substitution",
			cmd:  `some-cmd --password="$(vault kv get -field=password secret/db)"`,
		},
	}

	for _, tc := range safeCommands {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ScanForSecrets(tc.cmd)
			if result.Found {
				t.Errorf("false positive: %q flagged as %q", tc.name, result.PatternName)
			}
		})
	}
}

func TestScanForSecrets_TruePositives(t *testing.T) {
	t.Parallel()
	// Real plaintext secrets MUST still be caught.
	secretCommands := []struct {
		name        string
		cmd         string
		wantPattern string
	}{
		{
			name:        "GitHub PAT",
			cmd:         `git clone https://ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij@github.com/org/repo`,
			wantPattern: "GitHub personal access token",
		},
		{
			name:        "AWS access key ID",
			cmd:         `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE aws s3 ls`,
			wantPattern: "AWS access key ID",
		},
		{
			name:        "OpenAI API key",
			cmd:         `curl -H "Authorization: Bearer sk-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij" https://api.openai.com/v1/models`,
			wantPattern: "OpenAI API key",
		},
		{
			name:        "Inline plaintext password (unquoted)",
			cmd:         `./gradlew -Pflyway.password=mysupersecretpassword123`,
			wantPattern: "Inline secret env var",
		},
		{
			name:        "Inline plaintext password (double-quoted)",
			cmd:         `./gradlew -Pflyway.password="mysupersecretpassword123"`,
			wantPattern: "Inline secret env var",
		},
		{
			// "Inline secret env var" matches first because "password" is in its keyword list.
			name:        "PGPASSWORD with literal value",
			cmd:         `PGPASSWORD=mysecretpass psql -h host -U user`,
			wantPattern: "Inline secret env var",
		},
		{
			// "Inline secret env var" matches first because "password" is in its keyword list.
			name:        "CLI flag --password with literal",
			cmd:         `myapp --password=mysecretpassword123`,
			wantPattern: "Inline secret env var",
		},
		{
			name:        "Slack Incoming Webhook URL",
			cmd:         `curl -X POST -H 'Content-type: application/json' --data '{"text":"hi"}' https://hooks.slack.com/services/T0/B0/XXXXXXXXXXXXXXXXXXXXXXXX`,
			wantPattern: "Slack Incoming Webhook URL",
		},
	}

	for _, tc := range secretCommands {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ScanForSecrets(tc.cmd)
			if !result.Found {
				t.Errorf("missed secret: %q should have been flagged", tc.name)
			} else if result.PatternName != tc.wantPattern {
				t.Errorf("wrong pattern: got %q, want %q", result.PatternName, tc.wantPattern)
			}
		})
	}
}

func TestScanForSecrets_should_OnlyScanTheFirst4096Bytes(t *testing.T) {
	t.Parallel()
	token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"
	pad := func(n int) string { return strings.Repeat(" ", n) }

	if got := ScanForSecrets(pad(4097-len(token)) + token); got.Found {
		t.Errorf("secret ending at byte 4097 must not be scanned, got %+v", got)
	}
	if got := ScanForSecrets(pad(4096-len(token)) + token + "x"); !got.Found {
		t.Error("secret ending exactly at byte 4096 of a longer string must be found")
	}
}

func TestFormatSecretDenyMessage_should_NameThePattern(t *testing.T) {
	t.Parallel()
	got := FormatSecretDenyMessage("AWS access key ID")
	for _, want := range []string{"Blocked:", "(AWS access key ID)", "secrets manager"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q missing %q", got, want)
		}
	}
}

func TestRedactSecrets(t *testing.T) {
	t.Parallel()
	token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"
	tests := []struct {
		name        string
		in          string
		want        string
		wantChanged bool
	}{
		{name: "no secret is returned unchanged", in: "echo $HOME", want: "echo $HOME"},
		{name: "empty", in: "", want: ""},
		{name: "one secret", in: "token " + token + " end", want: "token [redacted] end", wantChanged: true},
		{name: "every occurrence", in: token + " " + token, want: "[redacted] [redacted]", wantChanged: true},
		{name: "different patterns in one string", in: "AKIAIOSFODNN7EXAMPLE and " + token, want: "[redacted] and [redacted]", wantChanged: true},
		{name: "scans past 4096 bytes", in: strings.Repeat("a ", 2048) + token, want: strings.Repeat("a ", 2048) + "[redacted]", wantChanged: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, changed := RedactSecrets(tc.in)
			if got != tc.want || changed != tc.wantChanged {
				t.Errorf("RedactSecrets(%q) = (%q, %v), want (%q, %v)", tc.in, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}
