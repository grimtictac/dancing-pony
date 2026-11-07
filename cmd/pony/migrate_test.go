package main

import (
	"bytes"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/store/storetest"
)

func TestMigrateCommand(t *testing.T) {
	db := storetest.Empty(t)
	env := func(k string) string {
		if k == "PONY_MIGRATE_DATABASE_URL" {
			return db.OwnerURL
		}
		return ""
	}
	migrate := func(action string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"migrate", action}, env, &stdout, &stderr); code != 0 {
			t.Fatalf("pony migrate %s: exit %d, stderr %q", action, code, stderr.String())
		}
		return stdout.String()
	}

	if out := migrate("status"); !strings.Contains(out, "pending") || strings.Contains(out, "applied") {
		t.Errorf("status before up = %q, want only pending migrations", out)
	}
	if out := migrate("up"); !strings.Contains(out, "OK") {
		t.Errorf("up = %q, want it to report applied migrations", out)
	}
	if out := migrate("up"); !strings.Contains(out, "no pending migrations") {
		t.Errorf("second up = %q, want no pending migrations", out)
	}
	if out := migrate("status"); strings.Contains(out, "pending") {
		t.Errorf("status after up = %q, want every migration applied", out)
	}
	migrate("down")
	if out := migrate("status"); !strings.Contains(out, "pending") {
		t.Errorf("status after down = %q, want a pending migration", out)
	}
}

func TestMigrateCommandErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"no database URL", []string{"migrate"}, 2, "PONY_MIGRATE_DATABASE_URL is not set"},
		{"unknown action", []string{"migrate", "sideways"}, 2, `unknown action "sideways"`},
		{"too many arguments", []string{"migrate", "up", "now"}, 2, "Usage: pony migrate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, noEnv, &stdout, &stderr); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
