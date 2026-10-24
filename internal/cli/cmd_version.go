package cli

import (
	"context"
	"fmt"
)

func versionCommand() *Command {
	return &Command{
		Name:  "version",
		Short: "显示版本",
		Usage: "用法: yuque-cli version",
		Run: func(_ context.Context, env *Env, _ []string) error {
			fmt.Fprintf(env.Stdout, "yuque-cli %s\n", Version)
			return nil
		},
	}
}
