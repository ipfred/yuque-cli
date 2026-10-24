# yuque-cli

读取语雀文档的命令行工具,并附带一个让 AI agent 使用它的 skill。单个 Go 二进制,无第三方依赖。

企业空间通常不允许创建个人 Token,所以本工具使用浏览器登录后的 `_yuque_session`,而不是官方 OpenAPI。

## 安装

需要 Go 1.22+。

```bash
make install            # 编译到 ~/.local/bin/yuque-cli,并把 skill 装进本机已有的 AI 客户端
# 或
go install ./cmd/yuque-cli  # 只装二进制;之后运行 yuque-cli skill install
```

确保 `~/.local/bin`(或 `$(go env GOPATH)/bin`)在 `PATH` 里。`make dist` 可交叉编译 macOS / Linux / Windows 版本。

## 配置:登录

1. 浏览器登录语雀,按 F12 → Application(应用)→ Cookies → 复制 `_yuque_session` 的 Value。
2. 运行:

```bash
yuque-cli auth login https://xxx.yuque.com/团队/知识库/文档    # 或 --host xxx.yuque.com
```

按提示粘贴(不回显)。整段 Cookie 或整条 curl 命令也行,会自动提取。凭证校验通过后才会保存到 `~/.config/yuque-cli/config.json`(权限 0600)。可按域名保存多个空间。

| 命令 | 说明 |
|---|---|
| `yuque-cli auth status` | 检查已保存的登录态是否有效 |
| `yuque-cli auth logout [--host h] [--all]` | 删除登录态 |
| 环境变量 `YUQUE_SESSION` | 临时覆盖,优先于配置文件 |
| 环境变量 `YUQUE_CONFIG_DIR` | 改变配置目录 |

session 会过期(通常几天到几周)。过期后命令以退出码 3 结束,重新 `auth login` 即可。

## 使用

```bash
yuque-cli doc get <文档链接>            # Markdown 正文输出到 stdout
yuque-cli doc get <文档链接> --meta     # 正文前加 YAML front matter(标题、知识库、更新时间)
yuque-cli doc get <文档链接> --json     # JSON 输出
yuque-cli doc get <文档链接> -o a.md    # 写入文件
yuque-cli <文档链接>                    # 简写
```

退出码:`0` 成功,`1` 其他错误,`2` 用法错误,`3` 登录态无效,`4` 文档不存在或无权限。

## Skill

`yuque-cli skill install` 会把内嵌的 `yuque-reader` skill 写入已检测到的客户端目录(`~/.claude/skills`、`~/.factory/skills`、`~/.codex/skills`、`~/.config/opencode/skills`)。

```bash
yuque-cli skill install --target claude,factory   # 指定客户端
yuque-cli skill install --dir /path/to/skills     # 自定义目录
yuque-cli skill uninstall
yuque-cli skill print                             # 查看内容
```

之后在对话里给 agent 一个语雀链接,它会调用 `yuque-cli doc get`。skill 源文件是 `skills/yuque-reader/SKILL.md`,编译时嵌入二进制,修改后需重新编译再 `skill install`。

## 实现说明

读取走的是 Web 端接口,不是 `/api/v2`:

- 正文:`GET /{团队}/{知识库}/{文档}/markdown?plain=true&linebreak=false&anchor=false`,只需要 `_yuque_session`。三个查询参数缺一就会返回整页 HTML。
- 元数据:文档页内嵌的 `window.appData`。
- 登录态检查:`GET /api/mine`。
- 登录态失效时接口会 302 到 `/login`,客户端据此返回退出码 3。

这些是非公开接口,语雀改版后可能失效。出问题时先看 `internal/yuque/client.go`。

## 扩展:新增命令

```
cmd/yuque-cli/main.go           入口
internal/cli/app.go         命令注册表、帮助、退出码
internal/cli/cmd_*.go       每组命令一个文件
internal/yuque/client.go    语雀 HTTP 客户端(session 认证)
internal/config/            配置与凭证存储
skills/                     内嵌的 agent skill
```

新增功能(例如 `doc update`):

1. 在 `internal/yuque/client.go` 加方法。写操作需要 CSRF:请求头 `x-csrf-token` 取自 Cookie `yuque_ctoken`,目前只存了 `_yuque_session`,需要在 `config` 里一并保存或从页面取得。
2. 在 `internal/cli/` 写一个 `*Command`,加入 `docCommand()` 的 `Sub`(或 `newRoot()`)。
3. 在 `SKILL.md` 补充用法。写操作要在 skill 里要求先征得用户确认。
4. 补测试:`go test ./...`。

## 多平台构建与发布(GitHub Actions)

workflow 文件:`.github/workflows/build.yml`。流程:`go vet` + `go test` → 交叉编译 → 打包 → (可选)发布 Release。

产物(`<版本>` 不带 `v`):

| 平台 | 文件 |
|---|---|
| macOS Apple Silicon / Intel | `yuque-cli_<版本>_darwin_arm64.tar.gz` / `..._darwin_amd64.tar.gz` |
| Linux x86_64 / ARM64 | `yuque-cli_<版本>_linux_amd64.tar.gz` / `..._linux_arm64.tar.gz` |
| Windows x86_64 | `yuque-cli_<版本>_windows_amd64.zip` |
| 校验和(仅 Release) | `SHA256SUMS` |

### 触发方式

**方式一:推送 tag(推荐,自动发布 Release)**

```bash
git tag v0.2.0
git push origin v0.2.0
```

构建完成后,在仓库 Releases 页面下载全部平台的包。

**方式二:手动触发,只构建不发布**

GitHub 页面:Actions → build → Run workflow(版本号可留空,默认 `dev-<短 commit>`),完成后在该次运行页面底部的 Artifacts 下载。

用 [GitHub CLI](https://cli.github.com/):

```bash
gh workflow run build.yml                                  # 只构建
gh workflow run build.yml -f version=0.2.0 -f release=true # 构建并发布 v0.2.0
gh run list --workflow build.yml                           # 查看运行状态
gh run watch                                               # 实时跟踪
gh run download <run-id>                                   # 下载 Artifacts
```

注意:

- 手动触发时,workflow 必须已经在默认分支上。
- 发布 Release 必须填写版本号;tag 不存在时会自动在触发时的 commit 上创建。
- 版本号通过 `-ldflags -X` 写入二进制,`yuque-cli version` 可以确认。
- 需要仓库允许 Actions 写权限:Settings → Actions → General → Workflow permissions 选 "Read and write permissions"(只影响发布 Release)。

### 安装下载的包

```bash
tar -xzf yuque-cli_0.2.0_darwin_arm64.tar.gz
install -m 0755 yuque-cli_0.2.0_darwin_arm64/yuque-cli ~/.local/bin/
yuque-cli skill install
```

macOS 从浏览器下载的二进制可能被 Gatekeeper 拦截,执行 `xattr -d com.apple.quarantine <文件>` 即可。

## 开发

```bash
make test
make build && ./bin/yuque-cli -h
```
