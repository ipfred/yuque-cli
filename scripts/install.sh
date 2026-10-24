#!/bin/sh
# yuque-cli 一键安装脚本 (macOS / Linux)
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.sh | sh -s -- 0.2.0
#
# 环境变量:
#   YUQUE_CLI_VERSION        指定版本,如 0.2.0 或 v0.2.0(默认取 GitHub 最新 Release)
#   YUQUE_CLI_BIN_DIR        二进制安装目录(默认 ~/.local/bin)
#   YUQUE_CLI_DOWNLOAD_BASE  下载地址前缀(默认 GitHub Releases;内网镜像或本地测试时覆盖,
#                            脚本会从 $BASE/v<版本>/ 下取产物)
#   YUQUE_CLI_SKIP_VERIFY    设为 1 跳过 SHA256 校验(仅限没有 SHA256SUMS 的镜像)
set -eu

REPO="ipfred/yuque-cli"

say() { printf '%s\n' "$*"; }
die() { printf '错误: %s\n' "$*" >&2; exit 1; }

tmpdir=""
tmp_dst=""
cleanup() {
  [ -z "$tmpdir" ] || rm -rf "$tmpdir"
  [ -z "$tmp_dst" ] || rm -f "$tmp_dst"
}
# INT/TERM 只负责退出,清理统一交给 EXIT,否则收到信号后脚本会继续往下跑。
trap cleanup EXIT
trap 'exit 1' INT TERM

# 带超时和重试;进度条只在交互终端显示。
fetch() {
  if [ -t 2 ]; then
    curl -fL -# --connect-timeout 15 --retry 3 "$@"
  else
    curl -fsSL --connect-timeout 15 --retry 3 "$@"
  fi
}
fetch_quiet() { curl -fsSL --connect-timeout 15 --retry 3 "$@"; }

# ---------- 可选位置参数: 版本号 ----------
version="${1:-${YUQUE_CLI_VERSION:-}}"
version="${version#v}"

# ---------- 平台检测 ----------
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  MINGW*|MSYS*|CYGWIN*)
    say "检测到 Windows (Git Bash / MSYS),请改用 PowerShell 运行 install.ps1:"
    say "  irm https://raw.githubusercontent.com/$REPO/main/scripts/install.ps1 | iex"
    exit 1 ;;
  *) die "不支持的操作系统: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "不支持的 CPU 架构: $(uname -m)(目前提供 darwin/linux 的 amd64、arm64)" ;;
esac

# Rosetta 终端里 uname -m 会报 x86_64,但机器其实是 Apple Silicon,装原生版本。
if [ "$os" = darwin ] && [ "$arch" = amd64 ] \
   && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

command -v curl >/dev/null 2>&1 || die "缺少 curl,请先安装"
command -v tar  >/dev/null 2>&1 || die "缺少 tar,请先安装"

bin_dir="${YUQUE_CLI_BIN_DIR:-$HOME/.local/bin}"
base="${YUQUE_CLI_DOWNLOAD_BASE:-https://github.com/$REPO/releases/download}"

# ---------- 解析版本: 默认取最新 Release ----------
if [ -z "$version" ]; then
  say ">> 查询最新版本..."
  # 优先走 /releases/latest 的重定向(不受 API 匿名限流影响),失败再退回 API。
  final=$(fetch_quiet -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" 2>/dev/null) || final=""
  tag=${final##*/}
  case "$tag" in
    v[0-9]*) ;;
    *)
      tag=$(fetch_quiet "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
            | tr ',' '\n' | grep '"tag_name"' | head -n 1 | cut -d'"' -f4) || tag=""
      ;;
  esac
  [ -n "$tag" ] || die "无法获取最新版本号,可设置 YUQUE_CLI_VERSION=<版本> 跳过查询"
  version=${tag#v}
fi

# 版本号会拼进 URL 和文件名,只接受字母、数字、点、下划线、连字符。
case "$version" in
  ''|*[!0-9A-Za-z._-]*|[!0-9A-Za-z]*) die "非法版本号: $version" ;;
esac

asset="yuque-cli_${version}_${os}_${arch}.tar.gz"
url="$base/v$version/$asset"

# ---------- 下载 + SHA256 校验 ----------
tmpdir=$(mktemp -d) || die "无法创建临时目录"

say ">> 下载 $url"
fetch "$url" -o "$tmpdir/$asset" \
  || die "下载失败(版本 $version 可能不存在或没有 ${os}/${arch} 的包,也可能网络不通)"

if [ "${YUQUE_CLI_SKIP_VERIFY:-}" = 1 ]; then
  say ">> 警告: 已按 YUQUE_CLI_SKIP_VERIFY=1 跳过 SHA256 校验"
else
  fetch_quiet "$base/v$version/SHA256SUMS" -o "$tmpdir/SHA256SUMS" \
    || die "无法下载 SHA256SUMS,拒绝安装未校验的文件(镜像环境可设 YUQUE_CLI_SKIP_VERIFY=1)"
  want=$(awk -v f="$asset" '$2==f || $2=="*" f {print $1}' "$tmpdir/SHA256SUMS" | head -n 1)
  [ -n "$want" ] || die "SHA256SUMS 里没有 $asset 的条目,拒绝安装"
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmpdir/$asset" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$tmpdir/$asset" | awk '{print $1}')
  else
    die "找不到 sha256sum / shasum,无法校验"
  fi
  [ "$got" = "$want" ] || die "SHA256 校验失败"
  say ">> SHA256 校验通过"
fi

# ---------- 解压安装 ----------
tar -xzf "$tmpdir/$asset" -C "$tmpdir"
src=$(find "$tmpdir" -type f -name yuque-cli | head -n 1)
[ -n "$src" ] || die "压缩包里找不到 yuque-cli 二进制"

mkdir -p "$bin_dir"
dst="$bin_dir/yuque-cli"
# 先写临时文件再 rename:正在运行的旧版本不会被截断,中途失败也不会留下半个文件。
tmp_dst="$bin_dir/.yuque-cli.tmp.$$"
cp "$src" "$tmp_dst"
chmod 0755 "$tmp_dst"
mv -f "$tmp_dst" "$dst"
tmp_dst=""
say ">> 已安装到 $dst"

# ---------- 保证终端里能用这个命令 ----------
# 写进配置文件的路径尽量用 $HOME,换机器/换用户名也不会失效。
case "$bin_dir" in
  "$HOME"/*) shown="\$HOME/${bin_dir#"$HOME"/}" ;;
  *)         shown="$bin_dir" ;;
esac
path_line="export PATH=\"$shown:\$PATH\""

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *)
    profile=""
    shell_name=$(basename "${SHELL:-sh}")
    case "$shell_name" in
      zsh)
        # macOS 的终端是登录 shell,读 .zprofile;Linux 终端多为非登录 shell,读 .zshrc。
        if [ "$os" = darwin ]; then profile="${ZDOTDIR:-$HOME}/.zprofile"
        else profile="${ZDOTDIR:-$HOME}/.zshrc"; fi ;;
      bash)
        if [ "$os" = darwin ]; then
          if [ -f "$HOME/.bash_profile" ]; then profile="$HOME/.bash_profile"
          else profile="$HOME/.profile"; fi
        else
          profile="$HOME/.bashrc"
        fi ;;
      fish) ;;
      *) profile="$HOME/.profile" ;;
    esac

    if [ "$shell_name" = fish ]; then
      say ""
      say "注意: 检测到 fish,请自行执行:  fish_add_path $bin_dir"
    else
      if ! grep -qsF "$shown" "$profile" 2>/dev/null; then
        printf '\n# added by yuque-cli installer\n%s\n' "$path_line" >> "$profile"
        say ">> 已把 $bin_dir 写入 $profile"
      fi
      say ""
      say "注意: 当前终端尚未生效,执行下面这句(或新开一个终端):"
      say "  $path_line"
    fi
    ;;
esac

# ---------- 验证 ----------
say ""
say ">> 验证:"
"$dst" version

say ""
say "安装成功! 后续步骤:"
say "  yuque-cli skill install                      # 把 yuque-reader skill 装进 AI 客户端"
say "  yuque-cli auth login <任意一篇语雀文档链接>    # 登录语雀"
