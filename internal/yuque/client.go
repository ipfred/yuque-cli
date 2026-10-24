package yuque

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrAuth means the session is missing, invalid or expired.
	ErrAuth = errors.New("登录态无效或已过期")
	// ErrNotFound means the doc does not exist or the account cannot read it.
	ErrNotFound = errors.New("文档不存在或无权限访问")
)

const sessionCookie = "_yuque_session"

// Client talks to a Yuque site using the browser session cookie. The public
// /api/v2 API needs a personal token that this workspace does not allow, so
// everything here goes through the same endpoints the web UI uses.
type Client struct {
	BaseURL   string
	Session   string
	UserAgent string
	HTTP      *http.Client
}

func New(baseURL, session string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Session:   session,
		UserAgent: "yuque-cli",
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			// A redirect to /login is how Yuque reports an invalid session, so it must be visible.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) do(ctx context.Context, path string, query url.Values, accept string) (*http.Response, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Session != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.Session})
	}
	return c.HTTP.Do(req)
}

func isLoginRedirect(resp *http.Response) bool {
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return false
	}
	return strings.Contains(resp.Header.Get("Location"), "/login")
}

// User is the account behind a session.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

// Mine returns the current user and doubles as the session validity check.
func (c *Client) Mine(ctx context.Context) (*User, error) {
	resp, err := c.do(ctx, "/api/mine", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || isLoginRedirect(resp) {
		return nil, ErrAuth
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查登录态失败: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data User `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析 /api/mine 响应失败: %w", err)
	}
	return &body.Data, nil
}

// Markdown downloads the doc body as Markdown.
func (c *Client) Markdown(ctx context.Context, ref DocRef) (string, error) {
	// All three parameters are required: without them the endpoint returns the full HTML page.
	q := url.Values{"plain": {"true"}, "linebreak": {"false"}, "anchor": {"false"}}
	path := fmt.Sprintf("/%s/%s/%s/markdown", ref.Group, ref.Book, ref.Slug)
	resp, err := c.do(ctx, path, q, "text/plain, */*")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch {
	case isLoginRedirect(resp), resp.StatusCode == http.StatusUnauthorized:
		return "", ErrAuth
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusForbidden:
		return "", ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("读取文档失败: HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		return "", fmt.Errorf("读取文档失败: 返回了非 Markdown 内容 (%s),接口可能已变更", ct)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DocMeta is the metadata Yuque embeds in the doc page.
type DocMeta struct {
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	BookID    int64  `json:"book_id"`
	WordCount int    `json:"word_count"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
	BookName  string `json:"-"`
}

var appDataRe = regexp.MustCompile(`window\.appData\s*=\s*JSON\.parse\(decodeURIComponent\("([^"]+)"\)\)`)

// Meta reads title, book and timestamps from the page's embedded appData.
func (c *Client) Meta(ctx context.Context, ref DocRef) (*DocMeta, error) {
	path := fmt.Sprintf("/%s/%s/%s", ref.Group, ref.Book, ref.Slug)
	resp, err := c.do(ctx, path, nil, "text/html")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case isLoginRedirect(resp), resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrAuth
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusForbidden:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("读取文档页面失败: HTTP %d", resp.StatusCode)
	}
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseMeta(string(html))
}

func parseMeta(html string) (*DocMeta, error) {
	m := appDataRe.FindStringSubmatch(html)
	if m == nil {
		return nil, fmt.Errorf("页面中没有找到文档元数据,页面结构可能已变更")
	}
	raw, err := url.PathUnescape(m[1])
	if err != nil {
		return nil, fmt.Errorf("解码文档元数据失败: %w", err)
	}
	var data struct {
		Doc  DocMeta `json:"doc"`
		Book struct {
			Name string `json:"name"`
		} `json:"book"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("解析文档元数据失败: %w", err)
	}
	data.Doc.BookName = data.Book.Name
	return &data.Doc, nil
}
