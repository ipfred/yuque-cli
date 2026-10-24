package yuque

import (
	"fmt"
	"net/url"
	"strings"
)

// DocRef identifies one document by its page URL: {scheme}://{host}/{group}/{book}/{slug}.
type DocRef struct {
	Scheme string
	Host   string
	Group  string
	Book   string
	Slug   string
}

func (d DocRef) BaseURL() string { return d.Scheme + "://" + d.Host }

func (d DocRef) PageURL() string {
	return fmt.Sprintf("%s/%s/%s/%s", d.BaseURL(), d.Group, d.Book, d.Slug)
}

// ParseDocURL accepts a Yuque doc link with or without scheme, and ignores the
// fragment, query string and trailing action segments such as /edit.
func ParseDocURL(raw string) (DocRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DocRef{}, fmt.Errorf("文档链接为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return DocRef{}, fmt.Errorf("无法解析链接: %q", raw)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return DocRef{}, fmt.Errorf("不支持的协议 %q", u.Scheme)
	}

	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) > 0 && segs[0] == "go" {
		return DocRef{}, fmt.Errorf("暂不支持 /go/... 短链接,请在浏览器打开后复制完整的 {团队}/{知识库}/{文档} 地址")
	}
	if len(segs) < 3 {
		return DocRef{}, fmt.Errorf("不是文档链接(期望 https://%s/{团队}/{知识库}/{文档}): %s", u.Host, raw)
	}
	return DocRef{Scheme: u.Scheme, Host: u.Host, Group: segs[0], Book: segs[1], Slug: segs[2]}, nil
}
