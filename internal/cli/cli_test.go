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
