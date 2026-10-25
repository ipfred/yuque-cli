package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"yuque-cli/skills"
)

const (
	scopeGlobal  = "global"
	scopeProject = "project"
)

// agentTarget is an AI client that loads skills from <root>/skills.
// Roots are relative to the home directory (global) or the project directory (project).
type agentTarget struct {
	name        string
	label       string
	globalRoot  string
	projectRoot string
}

var agentTargets = []agentTarget{
	{"agents", "通用 .agents 约定", ".agents", ".agents"},
	{"claude", "Claude Code", ".claude", ".claude"},
	{"factory", "Factory Droid", ".factory", ".factory"},
	{"codex", "Codex CLI", ".codex", ".codex"},
	{"opencode", "OpenCode", filepath.Join(".config", "opencode"), ".opencode"},
}

func (a agentTarget) root(scope string) string {
	if scope == scopeProject {
		return a.projectRoot
	}
	return a.globalRoot
}

func (a agentTarget) skillsDir(base, scope string) string {
	return filepath.Join(base, a.root(scope), "skills")
}

// detected reports whether the client's config directory already exists.
func (a agentTarget) detected(base, scope string) bool {
	_, err := os.Stat(filepath.Join(base, a.root(scope)))
	return err == nil
}

func (a agentTarget) installed(base, scope string) bool {
	_, err := os.Stat(filepath.Join(a.skillsDir(base, scope), skills.Name))
	return err == nil
}

// shown is the skills directory as the user would type it: ~/... or ./...
func (a agentTarget) shown(scope string) string {
	p := filepath.ToSlash(filepath.Join(a.root(scope), "skills"))
	if scope == scopeProject {
		return "./" + p
	}
	return "~/" + p
}

const skillUsage = `用法: yuque-cli skill <命令> [选项]

命令:
  install     安装 yuque-reader skill 到 AI 工具
  uninstall   卸载
  print       把 SKILL.md 输出到 stdout

选项(install / uninstall):
  --scope global|project   安装位置: global 装到 ~/ 下(所有项目通用),project 装到当前目录下
  --target <列表>          agents,claude,factory,codex,opencode,all(逗号分隔)
  --dir <目录>             安装到自定义 skills 目录(优先于 --scope 和 --target)
  -y, --yes                不提问,没给出的选项用默认值

在终端里运行且 --scope / --target 没有全部给出时,会交互式询问缺少的选项。
不在终端里(管道、脚本)或加了 --yes 时不提问,默认: 全局,装到已检测到的 AI 工具;
一个都没检测到则装到 agents。uninstall 默认卸载已安装的位置。

位置对照:
  工具       global                       project
  agents     ~/.agents/skills             ./.agents/skills
  claude     ~/.claude/skills             ./.claude/skills
  factory    ~/.factory/skills            ./.factory/skills
  codex      ~/.codex/skills              ./.codex/skills
  opencode   ~/.config/opencode/skills    ./.opencode/skills`

func skillCommand() *Command {
	return &Command{
		Name:  "skill",
		Short: "安装/卸载 AI agent 使用的 skill",
		Usage: skillUsage,
		Sub: []*Command{
			{Name: "install", Short: "安装 skill(终端里交互选择)", Usage: skillUsage, Run: func(ctx context.Context, env *Env, args []string) error {
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
	scope := fs.String("scope", "", "")
	target := fs.String("target", "", "")
	dir := fs.String("dir", "", "")
	yes := fs.Bool("yes", false, "")
	fs.BoolVar(yes, "y", false, "")
	if pos, err := parseFlags(fs, args); err != nil {
		return err
	} else if len(pos) > 0 {
		return usageErr("多余的参数: %s", strings.Join(pos, " "))
	}

	dirs, err := resolveSkillDirs(env, *scope, *target, *dir, *yes, install)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		if install {
			fmt.Fprintln(env.Stderr, "未选择任何位置,已取消")
			return nil
		}
		fmt.Fprintln(env.Stderr, "没有找到已安装的位置(可用 --scope / --target / --dir 指定)")
		return nil
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
			continue
		}
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			fmt.Fprintf(env.Stdout, "跳过 [%s] 未安装\n", d.name)
			continue
		}
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "已卸载 [%s] %s\n", d.name, dest)
	}
	if install {
		fmt.Fprintln(env.Stdout, "新开一个 AI 工具会话后生效。")
	}
	return nil
}

type skillDir struct{ name, path string }

func resolveSkillDirs(env *Env, scope, target, dir string, yes, install bool) ([]skillDir, error) {
	if dir != "" {
		return []skillDir{{"custom", dir}}, nil
	}
	if scope != "" && scope != scopeGlobal && scope != scopeProject {
		return nil, usageErr("--scope 只能是 global 或 project")
	}
	var picked []agentTarget
	if target != "" {
		var err error
		if picked, err = parseTargets(target); err != nil {
			return nil, err
		}
	}

	interactive := !yes && env.interactive() && (scope == "" || target == "")

	if scope == "" {
		scope = scopeGlobal
		if interactive {
			cwd, _ := env.getwd()
			s, err := promptScope(env, cwd)
			if err != nil {
				return nil, err
			}
			scope = s
		}
	}

	var base string
	var err error
	if scope == scopeProject {
		base, err = env.getwd()
	} else {
		base, err = env.homeDir()
	}
	if err != nil {
		return nil, err
	}

	if target == "" {
		if interactive {
			if picked, err = promptTargets(env, scope, base, install); err != nil {
				return nil, err
			}
		} else {
			picked = defaultTargets(scope, base, install)
		}
	}

	out := make([]skillDir, 0, len(picked))
	for _, a := range picked {
		out = append(out, skillDir{a.name, a.skillsDir(base, scope)})
	}
	return out, nil
}

// defaultTargets: for install, the clients already present (agents if none);
// for uninstall, the places where the skill is installed.
func defaultTargets(scope, base string, install bool) []agentTarget {
	var out []agentTarget
	for _, a := range agentTargets {
		if (install && a.detected(base, scope)) || (!install && a.installed(base, scope)) {
			out = append(out, a)
		}
	}
	if install && len(out) == 0 {
		out = append(out, agentTargets[0])
	}
	return out
}

// parseTargets parses --target. Only names (and "all") are accepted.
func parseTargets(list string) ([]agentTarget, error) {
	out, err := matchTargets(list, false)
	if err != nil {
		return nil, usageErr("--target: %v(可选: %s, all)", err, targetNames())
	}
	return out, nil
}

// pickTargets parses a menu answer, where numbers are accepted too.
func pickTargets(answer string) ([]agentTarget, error) {
	return matchTargets(answer, true)
}

// matchTargets resolves a comma/space separated list of names (and menu numbers when
// allowIndex is set). "a" / "all" selects every target.
func matchTargets(list string, allowIndex bool) ([]agentTarget, error) {
	var out []agentTarget
	seen := map[string]bool{}
	for _, t := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == '，' || r == ' ' }) {
		t = strings.ToLower(t)
		if t == "a" || t == "all" || t == "全部" {
			return append([]agentTarget(nil), agentTargets...), nil
		}
		var hit *agentTarget
		for i := range agentTargets {
			if agentTargets[i].name == t || (allowIndex && fmt.Sprint(i+1) == t) {
				hit = &agentTargets[i]
			}
		}
		if hit == nil {
			return nil, fmt.Errorf("%q 不是有效的选项", t)
		}
		if !seen[hit.name] {
			seen[hit.name] = true
			out = append(out, *hit)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("没有选择任何项")
	}
	return out, nil
}

func targetNames() string {
	names := make([]string, len(agentTargets))
	for i, a := range agentTargets {
		names[i] = a.name
	}
	return strings.Join(names, ", ")
}

func promptScope(env *Env, cwd string) (string, error) {
	if cwd == "" {
		cwd = "当前目录"
	}
	fmt.Fprintln(env.Stderr, "安装位置:")
	fmt.Fprintln(env.Stderr, "  1) 全局        装到 ~/ 下(如 ~/.claude/skills),所有项目通用")
	fmt.Fprintf(env.Stderr, "  2) 当前项目    装到 %s 下(如 ./.claude/skills),只对该项目生效\n", cwd)
	for i := 0; i < 3; i++ {
		line, err := ask(env, "选择 [1]: ")
		if err != nil {
			return "", err
		}
		switch strings.ToLower(line) {
		case "", "1", "g", "global", "全局":
			return scopeGlobal, nil
		case "2", "p", "project", "项目", "当前项目":
			return scopeProject, nil
		}
		fmt.Fprintln(env.Stderr, "输入无效,请输入 1 或 2")
	}
	return "", usageErr("多次输入无效")
}

func promptTargets(env *Env, scope, base string, install bool) ([]agentTarget, error) {
	verb := "安装"
	if !install {
		verb = "卸载"
	}
	def := defaultTargets(scope, base, install)

	fmt.Fprintf(env.Stderr, "\n%s到哪些 AI 工具?\n", verb)
	tw := tabwriter.NewWriter(env.Stderr, 0, 0, 2, ' ', 0)
	for i, a := range agentTargets {
		mark := ""
		switch {
		case a.installed(base, scope):
			mark = "  [已安装]"
		case a.detected(base, scope):
			mark = "  [已检测到]"
		}
		fmt.Fprintf(tw, "  %d) %s\t%s\t%s%s\n", i+1, a.name, a.shown(scope), a.label, mark)
	}
	tw.Flush()
	fmt.Fprintln(env.Stderr, "可多选: 输入编号或名称,逗号分隔(如 1,3);a = 全部。")

	names := make([]string, len(def))
	for i, a := range def {
		names[i] = a.name
	}
	defText := strings.Join(names, ",")
	if defText == "" {
		defText = "无"
	}
	prompt := fmt.Sprintf("选择 [回车 = %s]: ", defText)

	for i := 0; i < 3; i++ {
		line, err := ask(env, prompt)
		if err != nil {
			return nil, err
		}
		if line == "" {
			return def, nil
		}
		picked, err := pickTargets(line)
		if err == nil {
			return picked, nil
		}
		fmt.Fprintln(env.Stderr, "输入无效:", err)
	}
	return nil, usageErr("多次输入无效")
}

// ask prints a prompt on stderr (stdout may be piped) and reads one line.
func ask(env *Env, prompt string) (string, error) {
	fmt.Fprint(env.Stderr, prompt)
	line, err := readLine(env)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", usageErr("输入已结束。非交互场景请用 --scope / --target 指定,或加 --yes 使用默认值")
		}
		return "", err
	}
	return line, nil
}
