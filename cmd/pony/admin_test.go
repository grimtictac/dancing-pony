package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

func TestAdminCommands(t *testing.T) {
	db := storetest.Migrated(t)
	ctx := context.Background()
	if _, err := store.Seed(ctx, db.OwnerURL, store.DemoRestaurants); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == "PONY_MIGRATE_DATABASE_URL" {
			return db.OwnerURL
		}
		return ""
	}
	run := func(input string, args ...string) (int, string) {
		t.Helper()
		stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		code := run(args, env, &stdout, &stderr)
		return code, stdout.String() + stderr.String()
	}

	if code, out := run("gandalf-the-grey-pw\n", "admin", "create", "--email", "Gandalf@Valinor.example"); code != 0 || !strings.Contains(out, "gandalf@valinor.example is a platform admin") {
		t.Errorf("create: exit %d, output %q", code, out)
	}
	if code, out := run("short\n", "admin", "create", "--email", "saruman@isengard.example"); code != 1 || !strings.Contains(out, "password must be") {
		t.Errorf("short password: exit %d, output %q", code, out)
	}
	if code, out := run("", "admin", "grant", "--email", "gandalf@valinor.example", "--restaurant", "orc-shack", "--role", "staff"); code != 0 {
		t.Errorf("grant: exit %d, output %q", code, out)
	}
	if code, out := run("", "admin", "grant", "--email", "gandalf@valinor.example", "--restaurant", "orc-shack", "--role", "king"); code != 1 || !strings.Contains(out, "role must be") {
		t.Errorf("bad role: exit %d, output %q", code, out)
	}
}
