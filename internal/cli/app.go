// Package cli wires commands together. To add a feature, write a *Command
// and register it in newRoot; nothing else needs to change.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// Version is overridden at build time with -ldflags "-X yuque-cli/internal/cli.Version=...".
var Version = "dev"

// Exit codes. Agents rely on these, so keep them stable.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitAuth     = 3
	ExitNotFound = 4
)

// CodedError carries a specific process exit code.
type CodedError struct {
	Code int
	Err  error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

func usageErr(format string, a ...any) error {
	return &CodedError{Code: ExitUsage, Err: fmt.Errorf(format, a...)}
}

// Env is everything a command touches outside its own arguments, so tests can swap it.
type Env struct {
	In     *bufio.Reader
	Stdin  *os.File // nil when not a real file (tests); used to detect a terminal
	Stdout io.Writer
	Stderr io.Writer
	// BaseURL maps a host name to its site root. Defaults to https://{host}.
	BaseURL func(host string) string
}

func DefaultEnv() *Env {
	return &Env{
		In:      bufio.NewReader(os.Stdin),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		BaseURL: func(host string) string { return "https://" + host },
	}
}

type Command struct {
	Name    string
	Aliases []string
	Short   string
	Usage   string // shown by -h; group commands get an auto-generated list
	Run     func(ctx context.Context, env *Env, args []string) error
	Sub     []*Command
}

func (c *Command) matches(name string) bool {
	if c.Name == name {
		return true
	}
	for _, a := range c.Aliases {
		if a == name {
			return true
		}
	}
	return false
}

func newRoot() *Command {
	return &Command{
		Name:  "yuque-cli",
		Short: "语雀命令行工具",
		Usage: "用法: yuque-cli <命令> [参数]\n       yuque-cli <文档链接>      等同于 yuque-cli doc get <文档链接>",
		Sub: []*Command{
			docCommand(),
			authCommand(),
			skillCommand(),
			versionCommand(),
		},
	}
}

// Run executes the CLI and returns the process exit code.
func Run(ctx context.Context, env *Env, args []string) int {
	root := newRoot()
	cmd, rest := resolve(root, args)

	// `yuque-cli https://...` is shorthand for `yuque-cli doc get https://...`.
	if cmd == root && len(rest) > 0 && looksLikeURL(rest[0]) {
		cmd, rest = resolve(root, append([]string{"doc", "get"}, rest...))
	}

	if len(rest) > 0 && (rest[0] == "-h" || rest[0] == "--help" || rest[0] == "help") {
		printHelp(env.Stdout, cmd)
		return ExitOK
	}
	if cmd.Run == nil {
		if len(rest) > 0 {
			fmt.Fprintf(env.Stderr, "未知命令: %s\n\n", strings.Join(rest[:1], " "))
			printHelp(env.Stderr, cmd)
			return ExitUsage
		}
		printHelp(env.Stdout, cmd)
		return ExitOK
	}

	err := cmd.Run(ctx, env, rest)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, flag.ErrHelp) {
		printHelp(env.Stdout, cmd)
		return ExitOK
	}
	fmt.Fprintln(env.Stderr, "错误:", err)
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ExitError
}

func resolve(root *Command, args []string) (*Command, []string) {
	cmd := root
	for len(args) > 0 {
		var next *Command
		for _, s := range cmd.Sub {
			if s.matches(args[0]) {
				next = s
				break
			}
		}
		if next == nil {
			break
		}
		cmd, args = next, args[1:]
	}
	return cmd, args
}

func looksLikeURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func printHelp(w io.Writer, c *Command) {
	if c.Name == "yuque-cli" {
		fmt.Fprintf(w, "%s (%s)\n\n", c.Short, Version)
	}
	if c.Usage != "" {
		fmt.Fprintln(w, c.Usage)
	}
	if len(c.Sub) == 0 {
		return
	}
	fmt.Fprintln(w, "\n命令:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range c.Sub {
		fmt.Fprintf(tw, "  %s\t%s\n", s.Name, s.Short)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n使用 \"%s <命令> -h\" 查看详细说明。\n", c.Name)
}

// parseFlags parses flags that may appear before or after positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, usageErr("%v", err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}
