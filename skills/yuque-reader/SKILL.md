---
name: yuque-reader
description: 读取语雀(Yuque)文档内容。当用户给出语雀文档链接(形如 https://xxx.yuque.com/团队/知识库/文档),或要求查看、总结、引用语雀文档时使用。通过本机已登录的 yuque-cli 命令行 获取 Markdown 正文作为上下文。
---

# 读取语雀文档

用本机的 `yuque-cli` 命令把语雀文档转成 Markdown。链接需要登录才能访问,所以不要用 WebFetch 或 curl 直接请求。

## 读取文档

```bash
yuque-cli doc get "<文档链接>" --meta
```

- `--meta` 会在正文前输出 YAML front matter(标题、知识库、更新时间),便于引用来源。只需要正文时可省略。
- 链接里的 `#锚点` 和 `?query` 会被忽略。
- 文档很长时,先保存再分段读取,避免撑满上下文:

```bash
yuque-cli doc get "<文档链接>" --meta -o /tmp/yuque-doc.md
```

## 失败时怎么办

按退出码判断:

| 退出码 | 含义 | 处理 |
|---|---|---|
| 3 | 登录态无效或已过期 | 告诉用户在自己的终端运行 `yuque-cli auth login --host <域名>`,完成后再重试 |
| 4 | 文档不存在,或当前账号无权限 | 请用户确认链接和权限,不要反复重试 |
| 2 | 参数或链接格式错误 | 检查链接是否为 `https://域名/团队/知识库/文档` 形式 |
| 127 / command not found | 未安装 yuque-cli 命令行 | 告诉用户先安装 yuque-cli,再重试 |

## 注意

- 不要让用户把 Cookie 或 `_yuque_session` 粘贴到对话里。登录由用户在自己的终端用 `yuque-cli auth login` 完成。
- 不要打印、复述或写入 `~/.config/yuque-cli/config.json` 的内容,里面是登录凭证。
- 只读取用户明确给出的链接,不要自行遍历知识库里的其他文档。
- `yuque-cli auth status` 可以检查已保存的登录态是否有效。
