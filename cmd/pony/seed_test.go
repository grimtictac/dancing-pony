package main

import (
	"bytes"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/store/storetest"
)

func TestSeedCommand(t *testing.T) {
	db := storetest.Migrated(t)
	env := func(k string) string {
		if k == "PONY_MIGRATE_DATABASE_URL" {
			return db.OwnerURL
		}
		return ""
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"seed"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	for _, want := range []string{"orc-shack: dishes added: 2", "green-dragon: dishes added: 1"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
}

func TestSeedCommandNeedsDatabaseURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"seed"}, noEnv, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "PONY_MIGRATE_DATABASE_URL is not set") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
