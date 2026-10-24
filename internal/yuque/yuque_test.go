package yuque

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseDocURL(t *testing.T) {
	ok := map[string]DocRef{
		"https://a.yuque.com/g/b/s#frag":                                   {"https", "a.yuque.com", "g", "b", "s"},
		"https://a.yuque.com/g/b/s?x=1#y":                                  {"https", "a.yuque.com", "g", "b", "s"},
		"a.yuque.com/g/b/s/edit":                                           {"https", "a.yuque.com", "g", "b", "s"},
		"  http://127.0.0.1:8080/g/b/s  ":                                  {"http", "127.0.0.1:8080", "g", "b", "s"},
		"https://pingansec.yuque.com/tgyp0t/qaq4gl/nletl9n9kgpgayyf#swj0h": {"https", "pingansec.yuque.com", "tgyp0t", "qaq4gl", "nletl9n9kgpgayyf"},
	}
	for in, want := range ok {
		got, err := ParseDocURL(in)
		if err != nil || got != want {
			t.Errorf("ParseDocURL(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "https://a.yuque.com/g/b", "https://a.yuque.com/", "ftp://a.com/g/b/s", "https://a.yuque.com/go/doc/123"} {
		if _, err := ParseDocURL(in); err == nil {
			t.Errorf("ParseDocURL(%q) should fail", in)
		}
	}
}

func fakeSite(t *testing.T, wantSession string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("_yuque_session")
		authed := c != nil && c.Value == wantSession
		switch r.URL.Path {
		case "/api/mine":
			if !authed {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"login":"fred","name":"张三"}}`))
		case "/g/b/s/markdown":
			if !authed {
				http.Redirect(w, r, "/login?goto=x", http.StatusFound)
				return
			}
			q := r.URL.Query()
			if q.Get("plain") != "true" || q.Get("linebreak") != "false" || q.Get("anchor") != "false" {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte("<html>whole page</html>"))
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("## 正文\n"))
		case "/g/b/s":
			if !authed {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			data := `{"doc":{"title":"A: \"quoted\" <b>","word_count":42,"updated_at":"2026-01-02T03:04:05.000Z"},"book":{"name":"知识库"}}`
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<script>window.appData = JSON.parse(decodeURIComponent("` + url.PathEscape(data) + `"));</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClient(t *testing.T) {
	srv := fakeSite(t, "good")
	ctx := context.Background()
	ref := DocRef{"http", strings.TrimPrefix(srv.URL, "http://"), "g", "b", "s"}

	good := New(srv.URL, "good")
	u, err := good.Mine(ctx)
	if err != nil || u.Login != "fred" || u.Name != "张三" {
		t.Fatalf("Mine = %+v, %v", u, err)
	}
	md, err := good.Markdown(ctx, ref)
	if err != nil || md != "## 正文\n" {
		t.Fatalf("Markdown = %q, %v", md, err)
	}
	m, err := good.Meta(ctx, ref)
	if err != nil || m.Title != `A: "quoted" <b>` || m.BookName != "知识库" || m.WordCount != 42 {
		t.Fatalf("Meta = %+v, %v", m, err)
	}

	bad := New(srv.URL, "expired")
	if _, err := bad.Mine(ctx); !errors.Is(err, ErrAuth) {
		t.Errorf("Mine with bad session: %v", err)
	}
	if _, err := bad.Markdown(ctx, ref); !errors.Is(err, ErrAuth) {
		t.Errorf("Markdown with bad session: %v", err)
	}
	if _, err := bad.Meta(ctx, ref); !errors.Is(err, ErrAuth) {
		t.Errorf("Meta with bad session: %v", err)
	}

	missing := ref
	missing.Slug = "nope"
	if _, err := good.Markdown(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("Markdown for missing doc: %v", err)
	}
}

func TestParseMetaWithoutAppData(t *testing.T) {
	if _, err := parseMeta("<html></html>"); err == nil {
		t.Error("expected error")
	}
}
