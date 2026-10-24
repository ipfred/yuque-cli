package config

import (
	"os"
	"testing"
)

func TestExtractSession(t *testing.T) {
	cases := map[string]string{
		"abc==":                                 "abc==",
		"  abc==\n":                             "abc==",
		"_yuque_session=abc==":                  "abc==",
		"lang=zh-cn; _yuque_session=abc==; x=1": "abc==",
		`curl 'https://h/x' -b 'a=b; _yuque_session=abc==; c=d' -H 'k: v'`: "abc==",
		`"abc=="`: "abc==",
	}
	for in, want := range cases {
		if got := ExtractSession(in); got != want {
			t.Errorf("ExtractSession(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveLoadAndPermissions(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvSession, "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetSession("a.yuque.com", "s1")
	cfg.SetSession("b.yuque.com", "s2")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	p, _ := Path()
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode = %v, err %v", fi.Mode(), err)
	}

	got, _ := Load()
	if got.Session("a.yuque.com") != "s1" || got.Session("b.yuque.com") != "s2" || got.DefaultHost != "a.yuque.com" {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	t.Setenv(EnvSession, "_yuque_session=fromenv")
	if got.Session("a.yuque.com") != "fromenv" {
		t.Error("env var should override stored session")
	}
	t.Setenv(EnvSession, "")

	got.Remove("a.yuque.com")
	if got.DefaultHost != "b.yuque.com" {
		t.Errorf("default host after remove = %q", got.DefaultHost)
	}
}
