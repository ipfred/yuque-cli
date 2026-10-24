// Package config stores per-host Yuque session cookies on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvSession overrides the stored session for any host (useful in CI or one-off runs).
const EnvSession = "YUQUE_SESSION"

// EnvConfigDir overrides the directory that holds config.json.
const EnvConfigDir = "YUQUE_CONFIG_DIR"

type HostConfig struct {
	Session string `json:"session"`
}

type Config struct {
	DefaultHost string                `json:"default_host,omitempty"`
	Hosts       map[string]HostConfig `json:"hosts"`
}

func Dir() (string, error) {
	if d := os.Getenv(EnvConfigDir); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "yuque-cli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "yuque-cli"), nil
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Hosts: map[string]HostConfig{}}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("配置文件 %s 格式错误: %w", p, err)
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	return cfg, nil
}

// Save writes the config with 0600 permissions because it holds a login credential.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Session returns the session for host: env var first, then the stored value.
func (c *Config) Session(host string) string {
	if v := strings.TrimSpace(os.Getenv(EnvSession)); v != "" {
		return ExtractSession(v)
	}
	return c.Hosts[host].Session
}

func (c *Config) SetSession(host, session string) {
	c.Hosts[host] = HostConfig{Session: session}
	if c.DefaultHost == "" {
		c.DefaultHost = host
	}
}

func (c *Config) Remove(host string) {
	delete(c.Hosts, host)
	if c.DefaultHost == host {
		c.DefaultHost = ""
		for h := range c.Hosts {
			c.DefaultHost = h
			break
		}
	}
}

var sessionRe = regexp.MustCompile(`_yuque_session=([^;\s'"]+)`)

// ExtractSession accepts a bare cookie value, a "name=value" pair, a full Cookie
// header, or a pasted curl command, and returns just the _yuque_session value.
func ExtractSession(input string) string {
	input = strings.TrimSpace(input)
	if m := sessionRe.FindStringSubmatch(input); m != nil {
		return m[1]
	}
	return strings.Trim(input, `"' `)
}
