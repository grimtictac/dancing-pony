// Command pony runs and administers The Dancing Pony API.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/httpapi"
)

const usage = `Usage: pony <command> [flags]

Commands:
  serve   run the HTTP API
  help    show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to a subcommand and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "serve":
		return serve(args[1:], stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "pony: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func serve(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	handler := httpapi.New(dish.NewMemoryStore(dish.Seed()...))

	log.Println("The Orc Shack API listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}
