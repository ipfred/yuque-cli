package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"yuque-cli/internal/config"
	"yuque-cli/internal/yuque"
)

func authCommand() *Command {
	return &Command{
		Name:  "auth",
		Short: "管理登录态(session)",
		Usage: "用法: yuque-cli auth <命令> [参数]",
		Sub: []*Command{
			{
				Name:  "login",
				Short: "保存并校验 _yuque_session",
				Usage: authLoginUsage,
				Run:   runAuthLogin,
			},
			{
				Name:  "status",
				Short: "检查已保存的登录态是否有效",
				Usage: "用法: yuque-cli auth status [--host <域名>]",
				Run:   runAuthStatus,
			},
			{
				Name:  "logout",
				Short: "删除已保存的登录态",
				Usage: "用法: yuque-cli auth logout [--host <域名>] [--all]",
				Run:   runAuthLogout,
			},
		},
	}
}

const authLoginUsage = `用法: yuque-cli auth login [文档链接] [选项]

保存语雀登录态。企业空间通常不允许创建个人 Token,因此使用浏览器里的 _yuque_session。

获取方式:
  浏览器登录语雀 -> F12 -> Application(应用) -> Cookies -> 找到 _yuque_session -> 复制 Value
  也可以直接粘贴整段 Cookie 请求头,或整条 curl 命令,会自动提取。
  终端单行输入有长度上限,粘贴很长的内容时请改用管道:
    pbpaste | yuque-cli auth login --host xxx.yuque.com        (macOS)

选项:
  --host <域名>     语雀域名,如 xxx.yuque.com;也可以改为传一个文档链接
  --session <值>    直接传入(不建议,会留在 shell 历史里);不传则交互输入,或读取环境变量 YUQUE_SESSION
  --no-verify       不联网校验,直接保存

凭证保存在 ~/.config/yuque-cli/config.json(权限 0600)。`

func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

func runAuthLogin(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	host := fs.String("host", "", "")
	session := fs.String("session", "", "")
	noVerify := fs.Bool("no-verify", false, "")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return usageErr("最多传一个文档链接\n\n%s", authLoginUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	h := normalizeHost(*host)
	if h == "" && len(pos) == 1 {
		ref, err := yuque.ParseDocURL(pos[0])
		if err != nil {
			return usageErr("%v", err)
		}
		h = ref.Host
	}
	if h == "" {
		h = cfg.DefaultHost
	}
	if h == "" {
		fmt.Fprint(env.Stderr, "语雀域名(如 xxx.yuque.com): ")
		line, err := readLine(env)
		if err != nil {
			return err
		}
		h = normalizeHost(line)
	}
	if h == "" {
		return usageErr("需要语雀域名,请用 --host 指定")
	}

	value := strings.TrimSpace(*session)
	if value == "" {
		value = strings.TrimSpace(os.Getenv(config.EnvSession))
	}
	if value == "" {
		fmt.Fprintf(env.Stderr, "请粘贴 %s 的 _yuque_session(输入不会回显,回车确认): ", h)
		value, err = readSecret(env)
		fmt.Fprintln(env.Stderr)
		if err != nil {
			return err
		}
	}
	value = config.ExtractSession(value)
	if value == "" {
		return usageErr("没有读到 session 值")
	}

	if !*noVerify {
		user, err := yuque.New(env.BaseURL(h), value).Mine(ctx)
		if err != nil {
			if errors.Is(err, yuque.ErrAuth) {
				return &CodedError{Code: ExitAuth, Err: errors.New("该 session 无效或已过期,请重新从浏览器复制(确认复制的是 Value 且未被截断)")}
			}
			return err
		}
		fmt.Fprintf(env.Stdout, "登录成功: %s (%s) @ %s\n", user.Name, user.Login, h)
	}
	cfg.SetSession(h, value)
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	fmt.Fprintf(env.Stdout, "已保存到 %s\n", p)
	return nil
}

func runAuthStatus(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	host := fs.String("host", "", "")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	hosts := []string{}
	if h := normalizeHost(*host); h != "" {
		hosts = append(hosts, h)
	} else {
		for h := range cfg.Hosts {
			hosts = append(hosts, h)
		}
		sort.Strings(hosts)
	}
	if len(hosts) == 0 {
		return &CodedError{Code: ExitAuth, Err: errors.New("尚未登录。请运行: yuque-cli auth login")}
	}

	bad := false
	for _, h := range hosts {
		s := cfg.Session(h)
		if s == "" {
			fmt.Fprintf(env.Stdout, "✗ %s  未登录\n", h)
			bad = true
			continue
		}
		user, err := yuque.New(env.BaseURL(h), s).Mine(ctx)
		switch {
		case err == nil:
			fmt.Fprintf(env.Stdout, "✓ %s  %s (%s)\n", h, user.Name, user.Login)
		case errors.Is(err, yuque.ErrAuth):
			fmt.Fprintf(env.Stdout, "✗ %s  登录态已过期,请运行: yuque-cli auth login --host %s\n", h, h)
			bad = true
		default:
			fmt.Fprintf(env.Stdout, "? %s  检查失败: %v\n", h, err)
			bad = true
		}
	}
	if bad {
		return &CodedError{Code: ExitAuth, Err: errors.New("存在无效的登录态")}
	}
	return nil
}

func runAuthLogout(_ context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	host := fs.String("host", "", "")
	all := fs.Bool("all", false, "")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch {
	case *all:
		for h := range cfg.Hosts {
			cfg.Remove(h)
		}
	default:
		h := normalizeHost(*host)
		if h == "" {
			h = cfg.DefaultHost
		}
		if h == "" {
			return usageErr("请用 --host 指定域名,或 --all 删除全部")
		}
		if _, ok := cfg.Hosts[h]; !ok {
			return usageErr("%s 没有已保存的登录态", h)
		}
		cfg.Remove(h)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, "已退出登录")
	return nil
}

func readLine(env *Env) (string, error) {
	line, err := env.In.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// readSecret reads one line, hiding the echo when stdin is a terminal.
// If stty is unavailable the input is simply echoed.
func readSecret(env *Env) (string, error) {
	if env.Stdin != nil {
		if fi, err := env.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			off := exec.Command("stty", "-echo")
			off.Stdin = env.Stdin
			if off.Run() == nil {
				defer func() {
					on := exec.Command("stty", "echo")
					on.Stdin = env.Stdin
					_ = on.Run()
				}()
			}
		}
	}
	return readLine(env)
}
