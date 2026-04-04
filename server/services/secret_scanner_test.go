package services

import "testing"

func TestScanForSecrets_NoFalsePositives(t *testing.T) {
	// Shell variable references must NOT be flagged as plaintext secrets.
	safeCommands := []struct {
		name string
		cmd  string
	}{
		{
			name: "Gradle flyway with RDS token variable (quoted)",
			cmd: `./gradlew flywayRepair -Pflyway.url="jdbc:postgresql://host:5432/db" -Pflyway.user="fbg_migrations" -Pflyway.password="$RDS_TOKEN" -Pflyway.cleanDisabled=true`,
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
			result := ScanForSecrets(tc.cmd)
			if result.Found {
				t.Errorf("false positive: %q flagged as %q", tc.name, result.PatternName)
			}
		})
	}
}

func TestScanForSecrets_TruePositives(t *testing.T) {
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
	}

	for _, tc := range secretCommands {
		t.Run(tc.name, func(t *testing.T) {
			result := ScanForSecrets(tc.cmd)
			if !result.Found {
				t.Errorf("missed secret: %q should have been flagged", tc.name)
			} else if result.PatternName != tc.wantPattern {
				t.Errorf("wrong pattern: got %q, want %q", result.PatternName, tc.wantPattern)
			}
		})
	}
}
