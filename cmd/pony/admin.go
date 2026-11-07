package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5"
	"golang.org/x/term"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/authz"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

const adminUsage = `Usage:
  pony admin create --email <email> [--name <name>]
      Create a platform admin, or promote an existing user. The password is
      prompted for (or read from the first line of stdin), never taken as
      an argument, so it can't end up in shell history.
  pony admin grant --email <email> --restaurant <slug> --role owner|staff
      Give a user a role at a restaurant.

Connects with PONY_MIGRATE_DATABASE_URL: the role that owns the schema.
`

// stdin is the password source; a variable so tests can replace it.
var stdin io.Reader = os.Stdin

func admin(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stdout, adminUsage)
		return 0
	}
	dsn := getenv("PONY_MIGRATE_DATABASE_URL")
	if dsn == "" {
		_, _ = fmt.Fprintln(stderr, "pony admin: PONY_MIGRATE_DATABASE_URL is not set")
		return 2
	}

	fs := flag.NewFlagSet("admin "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "user's email")
	name := fs.String("name", "Platform admin", "user's name (create)")
	slug := fs.String("restaurant", "", "restaurant slug (grant)")
	role := fs.String("role", "", "owner or staff (grant)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *email == "" {
		_, _ = fmt.Fprint(stderr, "pony admin: --email is required\n\n"+adminUsage)
		return 2
	}
	normalised := strings.ToLower(strings.TrimSpace(*email))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch args[0] {
	case "create":
		err = createAdmin(ctx, dsn, strings.TrimSpace(*name), normalised, stderr)
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "%s is a platform admin\n", normalised)
		}
	case "grant":
		err = store.GrantRole(ctx, dsn, normalised, *slug, authz.Role(*role))
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "%s is now %s of %s\n", normalised, *role, *slug)
		}
	default:
		_, _ = fmt.Fprintf(stderr, "pony admin: unknown action %q\n\n%s", args[0], adminUsage)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "pony admin %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

func createAdmin(ctx context.Context, dsn, name, email string, prompt io.Writer) error {
	pw, err := readPassword(prompt)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	// An existing user keeps their password and is promoted.
	_, err = sqlcgen.New(conn).UpsertUser(ctx, sqlcgen.UpsertUserParams{Name: name, Email: email, PasswordHash: hash, IsPlatformAdmin: true})
	return err
}

// readPassword prompts twice without echo on a terminal, or reads one line
// from a pipe (for scripts: printf '%s\n' "$PW" | pony admin create ...).
func readPassword(prompt io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		_, _ = fmt.Fprint(prompt, "Password: ")
		pw, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprint(prompt, "Again: ")
		again, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(prompt)
		if err != nil {
			return "", err
		}
		if string(pw) != string(again) {
			return "", errors.New("passwords don't match")
		}
		return string(pw), nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
