package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"yuque-cli/internal/config"
	"yuque-cli/internal/yuque"
)

func docCommand() *Command {
	return &Command{
		Name:  "doc",
		Short: "文档操作",
		Usage: "用法: yuque-cli doc <命令> [参数]",
		Sub:   []*Command{docGetCommand()},
	}
}

const docGetUsage = `用法: yuque-cli doc get <文档链接> [选项]

读取一篇语雀文档,默认把 Markdown 正文输出到 stdout。

选项:
  --meta          在正文前输出 YAML front matter(标题、知识库、更新时间等)
  --json          输出 JSON(title/url/book/updated_at/word_count/markdown)
  -o, --output    写入文件,而不是 stdout

退出码: 0 成功, 3 登录态无效(先运行 yuque-cli auth login), 4 文档不存在或无权限`

func docGetCommand() *Command {
	return &Command{
		Name:  "get",
		Short: "读取文档内容(Markdown)",
		Usage: docGetUsage,
		Run:   runDocGet,
	}
}

func runDocGet(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("doc get", flag.ContinueOnError)
	withMeta := fs.Bool("meta", false, "")
	asJSON := fs.Bool("json", false, "")
	var output string
	fs.StringVar(&output, "o", "", "")
	fs.StringVar(&output, "output", "", "")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("需要且只需要一个文档链接\n\n%s", docGetUsage)
	}

	ref, err := yuque.ParseDocURL(pos[0])
	if err != nil {
		return usageErr("%v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	session := cfg.Session(ref.Host)
	if session == "" {
		return authErr(ref.Host, errors.New("尚未登录"))
	}
	client := yuque.New(ref.BaseURL(), session)

	// Metadata is a nice-to-have; fetch it in parallel and degrade on failure.
	type metaResult struct {
		meta *yuque.DocMeta
		err  error
	}
	var metaCh chan metaResult
	if *withMeta || *asJSON {
		metaCh = make(chan metaResult, 1)
		go func() {
			m, err := client.Meta(ctx, ref)
			metaCh <- metaResult{m, err}
		}()
	}

	md, err := client.Markdown(ctx, ref)
	if err != nil {
		return mapAPIError(ref.Host, err)
	}

	var meta *yuque.DocMeta
	if metaCh != nil {
		r := <-metaCh
		if r.err != nil {
			fmt.Fprintf(env.Stderr, "警告: 获取元数据失败,仅输出正文: %v\n", r.err)
		}
		meta = r.meta
	}

	var out []byte
	if *asJSON {
		out, err = renderJSON(ref, meta, md)
		if err != nil {
			return err
		}
	} else {
		out = []byte(md)
		if *withMeta && meta != nil {
			out = append([]byte(renderFrontMatter(ref, meta)), out...)
		}
	}

	if output != "" {
		if err := os.WriteFile(output, out, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(env.Stderr, "已保存到 %s (%d 字节)\n", output, len(out))
		return nil
	}
	_, err = env.Stdout.Write(out)
	return err
}

func authErr(host string, cause error) error {
	return &CodedError{Code: ExitAuth, Err: fmt.Errorf("%w。请运行: yuque-cli auth login --host %s", cause, host)}
}

func mapAPIError(host string, err error) error {
	switch {
	case errors.Is(err, yuque.ErrAuth):
		return authErr(host, err)
	case errors.Is(err, yuque.ErrNotFound):
		return &CodedError{Code: ExitNotFound, Err: err}
	}
	return err
}

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSpace(buf.String())
}

// Double-quoted JSON strings are valid YAML scalars, so titles with colons or quotes stay safe.
func renderFrontMatter(ref yuque.DocRef, m *yuque.DocMeta) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", jsonString(m.Title))
	fmt.Fprintf(&b, "url: %s\n", jsonString(ref.PageURL()))
	if m.BookName != "" {
		fmt.Fprintf(&b, "book: %s\n", jsonString(m.BookName))
	}
	if m.UpdatedAt != "" {
		fmt.Fprintf(&b, "updated_at: %s\n", jsonString(m.UpdatedAt))
	}
	if m.WordCount > 0 {
		fmt.Fprintf(&b, "word_count: %d\n", m.WordCount)
	}
	b.WriteString("---\n\n")
	return b.String()
}

func renderJSON(ref yuque.DocRef, m *yuque.DocMeta, md string) ([]byte, error) {
	v := map[string]any{"url": ref.PageURL(), "markdown": md}
	if m != nil {
		v["title"] = m.Title
		v["book"] = m.BookName
		v["updated_at"] = m.UpdatedAt
		v["word_count"] = m.WordCount
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
