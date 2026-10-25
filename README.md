# yuque-cli

读取语雀文档的命令行工具,并附带一个让 AI agent 使用它的 skill。单个 Go 二进制,无第三方依赖。

企业空间通常不允许创建个人 Token,所以本工具使用浏览器登录后的 `_yuque_session`,而不是官方 OpenAPI。

## 安装

### 方式一:一键脚本(推荐,无需安装 Go)

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.sh | sh
# 或指定版本
curl -fsSL https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.sh | sh -s -- 0.1.2
```

Windows(PowerShell):

```powershell
irm https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.ps1 | iex
```

脚本自动识别系统与 CPU 架构(包括 Apple Silicon 上的 Rosetta 终端、Windows ARM64),从 GitHub Releases 下载预编译二进制并做 SHA256 校验,装好后保证终端里能用 `yuque-cli`:

- **校验是强制的**:`SHA256SUMS` 缺失、没有对应条目或哈希不匹配都会拒绝安装。
- macOS / Linux 装到 `~/.local/bin`;若该目录不在 PATH,自动写入 shell 配置,新开终端即生效:

  | Shell | 写入的文件 |
  |---|---|
  | zsh | macOS: `~/.zprofile`;Linux: `~/.zshrc`(遵循 `$ZDOTDIR`) |
  | bash | macOS: `~/.bash_profile`(不存在则 `~/.profile`);Linux: `~/.bashrc` |
  | fish | 不写文件,提示执行 `fish_add_path <目录>` |
  | 其他 | `~/.profile` |

- Windows 装到 `%LOCALAPPDATA%\yuque-cli`,并写入用户 PATH(注册表持久化,保留原有的 `%VAR%` 引用),新开终端即生效。覆盖安装时若 `yuque-cli.exe` 正在运行,会先把旧文件改名再替换。
- 出错时 PowerShell 窗口不会被关闭,只打印错误信息。`install.ps1` 为纯 ASCII(英文提示),避免 Windows PowerShell 5.1 按 ANSI 读取时解析失败。

环境变量(两个脚本通用):

| 变量 | 作用 |
|---|---|
| `YUQUE_CLI_VERSION` | 指定版本,如 `0.1.2` 或 `v0.1.2`(默认取最新 Release) |
| `YUQUE_CLI_BIN_DIR` | 自定义安装目录 |
| `YUQUE_CLI_DOWNLOAD_BASE` | 内网/镜像的下载前缀,需保持 `<base>/v<版本>/yuque-cli_<版本>_<os>_<arch>.tar.gz`(Windows 为 `.zip`)和 `SHA256SUMS` 的结构 |
| `YUQUE_CLI_SKIP_VERIFY=1` | 跳过 SHA256 校验,仅限没有 `SHA256SUMS` 的镜像 |

### 方式二:手动下载

从 [Releases](https://github.com/ipfred/yuque-cli/releases) 下载对应平台的压缩包,解压后把二进制放进任意 PATH 目录。

### 方式三:从源码(需要 Go 1.22+)

```bash
make install            # 编译到 ~/.local/bin/yuque-cli,并把 skill 装进本机已检测到的 AI 工具(不提问)
# 或
go install ./cmd/yuque-cli  # 只装二进制;之后运行 yuque-cli skill install
```

确保 `~/.local/bin`(或 `$(go env GOPATH)/bin`)在 `PATH` 里。

### 卸载

- macOS / Linux:先 `yuque-cli skill uninstall`,再 `rm -f ~/.local/bin/yuque-cli`,并删除 shell 配置里 `# added by yuque-cli installer` 注释下的那行 export。
- Windows:删除 `%LOCALAPPDATA%\yuque-cli`,并在「编辑账户的环境变量」里移除对应 PATH 条目。

## 配置:登录

1. 浏览器登录语雀,按 F12 → Application(应用)→ Cookies → 复制 `_yuque_session` 的 Value。
2. 运行:

```bash
yuque-cli auth login https://xxx.yuque.com/团队/知识库/文档    # 或 --host xxx.yuque.com
```

按提示粘贴(粘贴时不回显)。整段 Cookie 或整条 curl 命令也行,会自动提取。回车后会显示打码后的内容,确认已经粘贴成功:

```
已读取 _yuque_session: AbCd***WxYz (共 128 个字符)
```

只显示首尾各 4 个字符和总长度。长度正好是 1023/1024 之类的终端单行上限时会提示可能被截断,这时请改用管道(`pbpaste | yuque-cli auth login --host ...`)。凭证校验通过后才会保存到 `~/.config/yuque-cli/config.json`(权限 0600)。可按域名保存多个空间。

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

`yuque-cli skill install` 把内嵌的 `yuque-reader` skill 写入 AI 工具的 skills 目录。在终端里直接运行会交互式询问两件事:

```
$ yuque-cli skill install
安装位置:
  1) 全局        装到 ~/ 下(如 ~/.claude/skills),所有项目通用
  2) 当前项目    装到 /path/to/project 下(如 ./.claude/skills),只对该项目生效
选择 [1]: 2

安装到哪些 AI 工具?
  1) agents    ./.agents/skills            通用 .agents 约定
  2) claude    ./.claude/skills            Claude Code  [已检测到]
  3) factory   ./.factory/skills           Factory Droid
  4) codex     ./.codex/skills             Codex CLI
  5) opencode  ./.opencode/skills          OpenCode
可多选: 输入编号或名称,逗号分隔(如 1,3);a = 全部。
选择 [回车 = claude]: 1,2
```

| 工具 | 全局 | 项目 |
|---|---|---|
| agents | `~/.agents/skills` | `./.agents/skills` |
| claude | `~/.claude/skills` | `./.claude/skills` |
| factory | `~/.factory/skills` | `./.factory/skills` |
| codex | `~/.codex/skills` | `./.codex/skills` |
| opencode | `~/.config/opencode/skills` | `./.opencode/skills` |

"当前项目"指运行命令时所在的目录。菜单里标 `[已检测到]` 的是配置目录已存在的工具,回车默认选它们(一个都没有则选 agents)。`uninstall` 同样交互,默认选已安装的位置。

不想被询问时用选项直接指定;只给出一部分,则只询问缺少的那部分:

```bash
yuque-cli skill install --scope global --target claude,factory   # 全局,指定工具
yuque-cli skill install --scope project --target all             # 当前目录下,所有工具
yuque-cli skill install --dir /path/to/skills                    # 自定义目录
yuque-cli skill install --yes                                    # 不提问,全局 + 已检测到的工具
yuque-cli skill uninstall
yuque-cli skill print                                            # 查看内容
```

不在终端里运行(管道、脚本、CI)时从不提问,等同于 `--yes`。`make install` 也是如此。

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

workflow 文件:`.github/workflows/build.yml`。流程:`go vet` + `go test` → 交叉编译 → 打包 → 用构建产物测试安装脚本 → (可选)发布 Release。

安装脚本测试(任一失败都会阻止发布):`install.sh` 在 Ubuntu 和 macOS 上跑,`install.ps1` 在 Windows PowerShell 5.1 上跑。覆盖校验和缺失/不匹配/非法版本号被拒绝、正常安装、PATH 写入、重复安装、`iex` 下失败不关闭会话。

产物(`<版本>` 不带 `v`):

| 平台 | 文件 |
|---|---|
| macOS Apple Silicon / Intel | `yuque-cli_<版本>_darwin_arm64.tar.gz` / `..._darwin_amd64.tar.gz` |
| Linux x86_64 / ARM64 | `yuque-cli_<版本>_linux_amd64.tar.gz` / `..._linux_arm64.tar.gz` |
| Windows x86_64 / ARM64 | `yuque-cli_<版本>_windows_amd64.zip` / `..._windows_arm64.zip` |
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
