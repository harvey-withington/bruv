// Package repocli implements the operator-facing `repo` subcommand
// for the headless server binary. Manipulates <configDir>/repos.json
// directly — no HTTP, no device token. The trust model is the same
// as the bootstrap token: file-system access to the server's config
// directory is itself the credential.
//
// Used in two places:
//
//   - cmd/bruv-server: invoked as `bruv-server.exe repo <subcmd>`
//   - main.go (the unified bruv.exe): invoked as `bruv.exe repo
//     <subcmd>` alongside `--server`, `service install`, etc.
//
// Both binaries call Run and exit with its return code.
//
// Subcommands:
//
//	repo list                     Print the registry as a table
//	repo add <path> [--name N]    Append a new entry (idempotent on path)
//	repo enable <id>              Set Disabled=false
//	repo disable <id>             Set Disabled=true
//	repo remove <id>              Drop from the registry (data on disk untouched)
//	repo rename <id> <name>       Update the display name
//
// Shared helpers (resolveID, printRepos) keep the dispatch table small
// and the per-subcommand functions readable.
package repocli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"bruv/internal/config"
)

// Run dispatches a `repo` subcommand and returns the process exit code.
// args is the full argv (including the program name); args[0] is the
// command verb (e.g. "list", "add"). The caller has already stripped
// "repo" from the front.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return cmdList(stdout, stderr)
	case "add":
		return cmdAdd(args[1:], stdout, stderr)
	case "enable":
		return cmdSetDisabled(args[1:], false, stdout, stderr)
	case "disable":
		return cmdSetDisabled(args[1:], true, stdout, stderr)
	case "remove", "rm":
		return cmdRemove(args[1:], stdout, stderr)
	case "rename":
		return cmdRename(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown repo subcommand: %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: repo <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list                       List registered repos")
	fmt.Fprintln(w, "  add <path> [--name N]      Register a vault at <path> (idempotent)")
	fmt.Fprintln(w, "  enable <id>                Re-enable a disabled vault")
	fmt.Fprintln(w, "  disable <id>               Disable a vault without removing it")
	fmt.Fprintln(w, "  remove <id>                Drop a vault from the registry")
	fmt.Fprintln(w, "  rename <id> <name>         Change a vault's display name")
}

func cmdList(stdout, stderr io.Writer) int {
	store, err := config.LoadRepos()
	if err != nil {
		fmt.Fprintf(stderr, "load repos: %v\n", err)
		return 1
	}
	if len(store.Repos) == 0 {
		fmt.Fprintln(stdout, "No vaults registered. Add one with: repo add <path>")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tPATH")
	for _, r := range store.Repos {
		status := "enabled"
		if r.Disabled {
			status = "disabled"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Name, status, r.Path)
	}
	_ = tw.Flush()
	return 0
}

func cmdAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("repo add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "display name (defaults to the directory's basename)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(stderr, "usage: repo add <path> [--name N]")
		return 2
	}
	entry, err := config.AppendRepo(rest[0], *name)
	if err != nil {
		fmt.Fprintf(stderr, "add: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Added %s (%s)\n  ID:   %s\n  Path: %s\n", entry.Name, statusLabel(entry.Disabled), entry.ID, entry.Path)
	return 0
}

func cmdSetDisabled(args []string, disabled bool, stdout, stderr io.Writer) int {
	verb := "enable"
	if disabled {
		verb = "disable"
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: repo %s <id>\n", verb)
		return 2
	}
	id, err := resolveID(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verb, err)
		return 1
	}
	if err := config.SetRepoDisabled(id, disabled); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verb, err)
		return 1
	}
	fmt.Fprintf(stdout, "%sd %s\n", verb, id)
	if !disabled {
		fmt.Fprintln(stdout, "Restart the server for the runtime to come back up.")
	}
	return 0
}

func cmdRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: repo remove <id>")
		return 2
	}
	id, err := resolveID(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "remove: %v\n", err)
		return 1
	}
	if err := config.RemoveRepo(id); err != nil {
		fmt.Fprintf(stderr, "remove: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Removed %s from registry. The vault on disk was not touched.\n", id)
	return 0
}

func cmdRename(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: repo rename <id> <new-name>")
		return 2
	}
	id, err := resolveID(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "rename: %v\n", err)
		return 1
	}
	name := strings.Join(args[1:], " ")
	if err := config.SetRepoName(id, name); err != nil {
		fmt.Fprintf(stderr, "rename: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Renamed %s to %q\n", id, name)
	return 0
}

// resolveID accepts either an exact UUID, a unique ID prefix (>=4
// chars), or a unique exact name match, and returns the canonical ID.
// Rejects ambiguous prefixes / names rather than silently picking one.
func resolveID(input string) (string, error) {
	store, err := config.LoadRepos()
	if err != nil {
		return "", fmt.Errorf("load repos: %w", err)
	}
	if len(store.Repos) == 0 {
		return "", fmt.Errorf("no vaults registered")
	}

	// Exact ID match wins immediately — the common case for `repo list`
	// → copy-paste workflows.
	for _, r := range store.Repos {
		if r.ID == input {
			return r.ID, nil
		}
	}

	// Otherwise, accumulate prefix + name matches and require uniqueness.
	var matches []config.RepoEntry
	for _, r := range store.Repos {
		if (len(input) >= 4 && strings.HasPrefix(r.ID, input)) || r.Name == input {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no vault matched %q (run `repo list` to see registered vaults)", input)
	case 1:
		return matches[0].ID, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q is ambiguous, matched:", input)
		for _, m := range matches {
			fmt.Fprintf(&b, "\n  %s  %s", m.ID, m.Name)
		}
		return "", fmt.Errorf("%s", b.String())
	}
}

func statusLabel(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return "enabled"
}
