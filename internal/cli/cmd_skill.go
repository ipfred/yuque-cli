package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"yuque-cli/skills"
)

// agentTarget is an AI client that loads skills from a directory.
type agentTarget struct {
	name string
	// root is the client's config dir; if it exists the client is considered installed.
	root string
	// skills is where SKILL.md folders go.
	skills string
}

func agentTargets(home string) []agentTarget {
	return []agentTarget{
		{"claude", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "skills")},
		{"factory", filepath.Join(home, ".factory"), filepath.Join(home, ".factory", "skills")},
		{"codex", filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "skills")},
		{"opencode", filepath.Join(home, ".config", "opencode"), filepath.Join(home, ".config", "opencode", "skills")},
	}
}

const skillUsage = `用法: yuque-cli skill <命令> [选项]

命令:
  install     安装 yuque-reader skill 到 AI 客户端
  uninstall   卸载
  print       把 SKILL.md 输出到 stdout

选项(install / uninstall):
  --target <列表>   claude,factory,codex,opencode,all(逗号分隔)。默认:自动检测已安装的客户端
  --dir <目录>      安装到自定义 skills 目录(优先于 --target)`

func skillCommand() *Command {
	return &Command{
		Name:  "skill",
		Short: "安装/卸载 AI agent 使用的 skill",
		Usage: skillUsage,
		Sub: []*Command{
			{Name: "install", Short: "安装 skill", Usage: skillUsage, Run: func(ctx context.Context, env *Env, args []string) error {
				return runSkill(env, args, true)
			}},
			{Name: "uninstall", Short: "卸载 skill", Usage: skillUsage, Run: func(ctx context.Context, env *Env, args []string) error {
				return runSkill(env, args, false)
			}},
			{Name: "print", Short: "输出 SKILL.md", Usage: "用法: yuque-cli skill print", Run: func(ctx context.Context, env *Env, args []string) error {
				b, err := skills.FS.ReadFile(skills.Name + "/SKILL.md")
				if err != nil {
					return err
				}
				_, err = env.Stdout.Write(b)
				return err
			}},
		},
	}
}

func runSkill(env *Env, args []string, install bool) error {
	fs := flag.NewFlagSet("skill", flag.ContinueOnError)
	target := fs.String("target", "", "")
	dir := fs.String("dir", "", "")
	if pos, err := parseFlags(fs, args); err != nil {
		return err
	} else if len(pos) > 0 {
		return usageErr("多余的参数: %s", strings.Join(pos, " "))
	}

	dirs, err := resolveSkillDirs(*target, *dir, install)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return usageErr("没有找到可用的目录")
	}

	content, err := skills.FS.ReadFile(skills.Name + "/SKILL.md")
	if err != nil {
		return err
	}
	for _, d := range dirs {
		dest := filepath.Join(d.path, skills.Name)
		if install {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), content, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "已安装 [%s] %s\n", d.name, dest)
		} else {
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				fmt.Fprintf(env.Stdout, "跳过 [%s] 未安装\n", d.name)
				continue
			}
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "已卸载 [%s] %s\n", d.name, dest)
		}
	}
	return nil
}

type skillDir struct{ name, path string }

func resolveSkillDirs(target, dir string, install bool) ([]skillDir, error) {
	if dir != "" {
		return []skillDir{{"custom", dir}}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	all := agentTargets(home)

	wanted := map[string]bool{}
	for _, t := range strings.Split(target, ",") {
		if t = strings.TrimSpace(t); t != "" {
			wanted[t] = true
		}
	}

	var out []skillDir
	for _, a := range all {
		switch {
		case wanted["all"], wanted[a.name]:
			out = append(out, skillDir{a.name, a.skills})
		case len(wanted) == 0:
			if _, err := os.Stat(a.root); err == nil {
				out = append(out, skillDir{a.name, a.skills})
			}
		}
	}
	for t := range wanted {
		if t == "all" {
			continue
		}
		found := false
		for _, a := range all {
			found = found || a.name == t
		}
		if !found {
			return nil, usageErr("未知的 target %q,可选: claude, factory, codex, opencode, all", t)
		}
	}
	if len(out) == 0 && len(wanted) == 0 && install {
		// Nothing detected: fall back to Claude's layout, which most clients also read.
		out = append(out, skillDir{"claude", all[0].skills})
	}
	return out, nil
}
