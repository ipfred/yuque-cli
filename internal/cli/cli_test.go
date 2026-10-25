package cli

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yuque-cli/internal/config"
)

func site(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("_yuque_session")
		authed := c != nil && c.Value == "good"
		switch r.URL.Path {
		case "/api/mine":
			if !authed {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"data":{"login":"fred","name":"张三"}}`))
		case "/g/b/s/markdown":
			if !authed {
				http.Redirect(w, r, "/login", 302)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("## 正文\n"))
		case "/g/b/s":
			data := `{"doc":{"title":"标题: x","word_count":3,"updated_at":"2026-01-02"},"book":{"name":"库"}}`
			w.Write([]byte(`window.appData = JSON.parse(decodeURIComponent("` + url.PathEscape(data) + `"))`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, strings.TrimPrefix(srv.URL, "http://")
}

func run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	env := &Env{
		In:      bufio.NewReader(strings.NewReader(stdin)),
		Stdout:  &out,
		Stderr:  &errb,
		BaseURL: func(host string) string { return "http://" + host },
	}
	code = Run(context.Background(), env, args)
	return code, out.String(), errb.String()
}

func isolate(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvSession, "")
}

func TestDocGetRequiresLogin(t *testing.T) {
	isolate(t)
	_, host := site(t)
	code, _, stderr := run(t, "", "doc", "get", "http://"+host+"/g/b/s")
	if code != ExitAuth || !strings.Contains(stderr, "yuque-cli auth login --host "+host) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestLoginThenGet(t *testing.T) {
	isolate(t)
	_, host := site(t)

	code, _, stderr := run(t, "bad\n", "auth", "login", "--host", host)
	if code != ExitAuth {
		t.Fatalf("bad session: code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(mustConfigPath(t)); err == nil {
		t.Fatal("invalid session must not be saved")
	}

	// A pasted Cookie header is accepted.
	code, stdout, stderr := run(t, "lang=zh; _yuque_session=good; x=1\n", "auth", "login", "--host", host)
	if code != 0 || !strings.Contains(stdout, "张三") {
		t.Fatalf("login: code=%d out=%q err=%q", code, stdout, stderr)
	}

	link := "http://" + host + "/g/b/s#anchor"
	code, stdout, _ = run(t, "", "doc", "get", link)
	if code != 0 || stdout != "## 正文\n" {
		t.Fatalf("get: code=%d out=%q", code, stdout)
	}

	// URL shorthand and flags after the URL.
	code, stdout, _ = run(t, "", link, "--meta")
	want := "---\ntitle: \"标题: x\"\nurl: \"" + "http://" + host + "/g/b/s\"\nbook: \"库\"\nupdated_at: \"2026-01-02\"\nword_count: 3\n---\n\n## 正文\n"
	if code != 0 || stdout != want {
		t.Fatalf("meta: code=%d\n got %q\nwant %q", code, stdout, want)
	}

	code, stdout, _ = run(t, "", "doc", "get", link, "--json")
	if code != 0 || !strings.Contains(stdout, `"markdown": "## 正文\n"`) || !strings.Contains(stdout, `"title": "标题: x"`) {
		t.Fatalf("json: code=%d out=%q", code, stdout)
	}

	out := filepath.Join(t.TempDir(), "d.md")
	code, stdout, _ = run(t, "", "doc", "get", link, "-o", out)
	b, _ := os.ReadFile(out)
	if code != 0 || stdout != "" || string(b) != "## 正文\n" {
		t.Fatalf("-o: code=%d stdout=%q file=%q", code, stdout, b)
	}

	code, stdout, _ = run(t, "", "auth", "status")
	if code != 0 || !strings.Contains(stdout, "✓ "+host) {
		t.Fatalf("status: code=%d out=%q", code, stdout)
	}

	code, _, _ = run(t, "", "auth", "logout", "--host", host)
	if code != 0 {
		t.Fatalf("logout code=%d", code)
	}
	code, _, _ = run(t, "", "doc", "get", link)
	if code != ExitAuth {
		t.Fatalf("after logout code=%d", code)
	}
}

func TestExpiredSessionAndNotFound(t *testing.T) {
	isolate(t)
	_, host := site(t)
	t.Setenv(config.EnvSession, "expired")
	if code, _, _ := run(t, "", "doc", "get", "http://"+host+"/g/b/s"); code != ExitAuth {
		t.Errorf("expired: code=%d", code)
	}
	t.Setenv(config.EnvSession, "good")
	if code, _, _ := run(t, "", "doc", "get", "http://"+host+"/g/b/missing"); code != ExitNotFound {
		t.Errorf("missing: code=%d", code)
	}
}

func TestUsageErrors(t *testing.T) {
	isolate(t)
	if code, _, _ := run(t, "", "doc", "get"); code != ExitUsage {
		t.Errorf("no url: %d", code)
	}
	if code, _, _ := run(t, "", "doc", "get", "http://h/only/two"); code != ExitUsage {
		t.Errorf("bad url: %d", code)
	}
	if code, _, _ := run(t, "", "doc", "get", "--nope", "x"); code != ExitUsage {
		t.Errorf("bad flag: %d", code)
	}
	if code, _, stderr := run(t, "", "nonsense"); code != ExitUsage || !strings.Contains(stderr, "未知命令") {
		t.Errorf("unknown cmd: %d %q", code, stderr)
	}
	if code, stdout, _ := run(t, "", "doc", "get", "-h"); code != 0 || !strings.Contains(stdout, "--meta") {
		t.Errorf("help: %d %q", code, stdout)
	}
}

func TestSkillInstall(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := run(t, "", "skill", "install", "--dir", dir)
	if code != 0 {
		t.Fatalf("install: %d %q %q", code, stdout, stderr)
	}
	b, err := os.ReadFile(filepath.Join(dir, "yuque-reader", "SKILL.md"))
	if err != nil || !strings.HasPrefix(string(b), "---\nname: yuque-reader\n") {
		t.Fatalf("skill file: %v %q", err, b)
	}
	if code, _, _ := run(t, "", "skill", "uninstall", "--dir", dir); code != 0 {
		t.Fatalf("uninstall: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "yuque-reader")); !os.IsNotExist(err) {
		t.Error("skill dir should be gone")
	}
	if code, _, _ := run(t, "", "skill", "install", "--target", "bogus"); code != ExitUsage {
		t.Errorf("bogus target: %d", code)
	}
}

func mustConfigPath(t *testing.T) string {
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"abc":              "***",
		"abcdefg":          "*******",
		"abcdefgh":         "ab***gh",
		"abcdefghijklmno":  "ab***no",
		"abcdefghijklmnop": "abcd***mnop",
	}
	for in, want := range cases {
		if got := maskSecret(in); got != want {
			t.Errorf("maskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoginEchoesMaskedSession(t *testing.T) {
	isolate(t)
	secret := "abcdefghijklmnopqrstuvwxyz"
	code, _, stderr := run(t, secret+"\n", "auth", "login", "--host", "h.example.com", "--no-verify")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "abcd***wxyz") || !strings.Contains(stderr, "共 26 个字符") {
		t.Errorf("missing masked echo: %q", stderr)
	}
	if strings.Contains(stderr, secret) || strings.Contains(stderr, "efghijklmnopqrstuv") {
		t.Errorf("secret leaked: %q", stderr)
	}

	// A whole Cookie header: the extracted value is what gets masked.
	cookie := "lang=zh; _yuque_session=" + secret + "; x=1"
	_, _, stderr = run(t, cookie+"\n", "auth", "login", "--host", "h.example.com", "--no-verify")
	if !strings.Contains(stderr, "abcd***wxyz") || !strings.Contains(stderr, "已从") {
		t.Errorf("cookie paste: %q", stderr)
	}

	_, _, stderr = run(t, strings.Repeat("a", 1023)+"\n", "auth", "login", "--host", "h.example.com", "--no-verify")
	if !strings.Contains(stderr, "截断") {
		t.Errorf("expected truncation warning: %q", stderr)
	}
}

// skillEnv is an interactive environment rooted in temp dirs.
type skillEnv struct {
	home, cwd string
	interact  bool
}

func newSkillEnv(t *testing.T) *skillEnv {
	return &skillEnv{home: t.TempDir(), cwd: t.TempDir(), interact: true}
}

func (s *skillEnv) run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	env := &Env{
		In:          bufio.NewReader(strings.NewReader(stdin)),
		Stdout:      &out,
		Stderr:      &errb,
		BaseURL:     func(host string) string { return "http://" + host },
		HomeDir:     func() (string, error) { return s.home, nil },
		Getwd:       func() (string, error) { return s.cwd, nil },
		Interactive: func() bool { return s.interact },
	}
	code = Run(context.Background(), env, args)
	return code, out.String(), errb.String()
}

func exists(parts ...string) bool {
	_, err := os.Stat(filepath.Join(parts...))
	return err == nil
}

func TestSkillInteractiveGlobalMultiSelect(t *testing.T) {
	s := newSkillEnv(t)
	code, _, stderr := s.run(t, "\n2,factory\n", "skill", "install")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, d := range []string{".claude", ".factory"} {
		if !exists(s.home, d, "skills", "yuque-reader", "SKILL.md") {
			t.Errorf("%s not installed", d)
		}
	}
	if exists(s.home, ".agents") || exists(s.home, ".codex") {
		t.Error("unselected targets were written")
	}
	if exists(s.cwd, ".claude") {
		t.Error("global install wrote into the project")
	}
	if !strings.Contains(stderr, "~/.claude/skills") {
		t.Errorf("menu should show paths: %q", stderr)
	}
}

func TestSkillInteractiveProjectAll(t *testing.T) {
	s := newSkillEnv(t)
	if code, _, stderr := s.run(t, "2\na\n", "skill", "install"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, d := range []string{".agents", ".claude", ".factory", ".codex", ".opencode"} {
		if !exists(s.cwd, d, "skills", "yuque-reader", "SKILL.md") {
			t.Errorf("project %s not installed", d)
		}
	}
	if exists(s.home, ".claude") || exists(s.home, ".config") {
		t.Error("project install wrote into home")
	}
}

func TestSkillInteractiveDefaults(t *testing.T) {
	// Nothing detected: default is agents.
	s := newSkillEnv(t)
	if code, _, stderr := s.run(t, "\n\n", "skill", "install"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !exists(s.home, ".agents", "skills", "yuque-reader") || exists(s.home, ".claude") {
		t.Error("default should be agents only")
	}

	// A detected client becomes the default.
	s = newSkillEnv(t)
	os.MkdirAll(filepath.Join(s.home, ".claude"), 0o755)
	os.MkdirAll(filepath.Join(s.home, ".config", "opencode"), 0o755)
	code, _, stderr := s.run(t, "\n\n", "skill", "install")
	if code != 0 || !strings.Contains(stderr, "[已检测到]") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !exists(s.home, ".claude", "skills", "yuque-reader") || !exists(s.home, ".config", "opencode", "skills", "yuque-reader") || exists(s.home, ".agents") {
		t.Error("default should be the detected clients")
	}
}

func TestSkillInteractiveInvalidThenValidAndEOF(t *testing.T) {
	s := newSkillEnv(t)
	code, _, stderr := s.run(t, "9\n1\n99\nnope\n3\n", "skill", "install")
	if code != 0 || !strings.Contains(stderr, "输入无效") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	// "9" is invalid scope -> retry "1" (global); "99" invalid target -> "nope" invalid -> "3" = factory.
	if !exists(s.home, ".factory", "skills", "yuque-reader") {
		t.Error("factory should be installed after retries")
	}

	// Input ends while a question is pending.
	s = newSkillEnv(t)
	if code, _, stderr := s.run(t, "", "skill", "install"); code != ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Errorf("EOF: code=%d stderr=%q", code, stderr)
	}
}

func TestSkillFlagsSkipPrompts(t *testing.T) {
	s := newSkillEnv(t)
	// Both flags given: no questions, so the (empty) stdin is never read.
	if code, _, stderr := s.run(t, "", "skill", "install", "--scope", "project", "--target", "claude,opencode"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !exists(s.cwd, ".claude", "skills", "yuque-reader") || !exists(s.cwd, ".opencode", "skills", "yuque-reader") || exists(s.cwd, ".agents") {
		t.Error("flags not honoured")
	}

	// Only --scope given: only the target question is asked.
	s = newSkillEnv(t)
	code, _, stderr := s.run(t, "codex\n", "skill", "install", "--scope", "project")
	if code != 0 || strings.Contains(stderr, "安装位置") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !exists(s.cwd, ".codex", "skills", "yuque-reader") {
		t.Error("codex not installed in project")
	}
}

func TestSkillNonInteractiveAndYes(t *testing.T) {
	for name, tc := range map[string]struct {
		interact bool
		args     []string
	}{
		"not a terminal": {false, []string{"skill", "install"}},
		"--yes":          {true, []string{"skill", "install", "--yes"}},
		"-y":             {true, []string{"skill", "install", "-y"}},
	} {
		s := newSkillEnv(t)
		s.interact = tc.interact
		os.MkdirAll(filepath.Join(s.home, ".factory"), 0o755)
		code, _, stderr := s.run(t, "", tc.args...)
		if code != 0 || strings.Contains(stderr, "选择") {
			t.Errorf("%s: code=%d stderr=%q", name, code, stderr)
		}
		if !exists(s.home, ".factory", "skills", "yuque-reader") || exists(s.home, ".agents") {
			t.Errorf("%s: should install to the detected client only", name)
		}
	}
}

func TestSkillUninstallInteractive(t *testing.T) {
	s := newSkillEnv(t)
	if code, _, _ := s.run(t, "", "skill", "install", "--scope", "global", "--target", "all"); code != 0 {
		t.Fatal("setup install failed")
	}
	// Pick only claude.
	code, stdout, stderr := s.run(t, "\nclaude\n", "skill", "uninstall")
	if code != 0 || !strings.Contains(stderr, "[已安装]") {
		t.Fatalf("code=%d out=%q stderr=%q", code, stdout, stderr)
	}
	if exists(s.home, ".claude", "skills", "yuque-reader") || !exists(s.home, ".factory", "skills", "yuque-reader") {
		t.Error("only claude should be removed")
	}
	// Enter = everything still installed.
	if code, _, _ := s.run(t, "\n\n", "skill", "uninstall"); code != 0 {
		t.Fatal("uninstall default failed")
	}
	for _, d := range []string{".agents", ".factory", ".codex"} {
		if exists(s.home, d, "skills", "yuque-reader") {
			t.Errorf("%s should be removed by the default", d)
		}
	}
	// Nothing left: default is empty and the run is a no-op.
	if code, _, stderr := s.run(t, "\n\n", "skill", "uninstall"); code != 0 || !strings.Contains(stderr, "没有找到已安装") {
		t.Errorf("nothing installed: code=%d stderr=%q", code, stderr)
	}
}

func TestSkillBadFlags(t *testing.T) {
	s := newSkillEnv(t)
	if code, _, _ := s.run(t, "", "skill", "install", "--scope", "everywhere"); code != ExitUsage {
		t.Errorf("bad scope: %d", code)
	}
	if code, _, stderr := s.run(t, "", "skill", "install", "--target", "claude,bogus"); code != ExitUsage || !strings.Contains(stderr, "bogus") {
		t.Errorf("bad target: %d %q", code, stderr)
	}
	// Menu numbers are not valid for --target.
	if code, _, _ := s.run(t, "", "skill", "install", "--target", "2"); code != ExitUsage {
		t.Errorf("numeric --target: %d", code)
	}
}
