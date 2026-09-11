#!/usr/bin/env bash
# johnnybt-pypi 一键安装 / 升级（Linux + systemd）
#
#   curl -fsSL https://raw.githubusercontent.com/quant-on-quest/johnnybt-pypi/main/scripts/install.sh | sudo bash
#
# 再跑一次就是升级：换二进制、重启服务；配置（/etc/pypi-server/env）和数据不动。
#
# 可选环境变量：
#   VERSION=v0.2.0                 装指定版本（默认最新 release）
#   GH_PROXY=https://ghfast.top/   国内加速：github.com 的下载 URL 前面加这个前缀
#   PREFIX=/usr/local/bin          二进制目录
#   DATA_DIR=/var/lib/pypi-server  数据目录（SQLite、文件、证书、初始密码）
#   ETC_DIR=/etc/pypi-server       配置目录
#   FORCE=1                        版本相同也重装
#   NO_SYSTEMD=1                   只装二进制和配置模板，不建用户、不碰 systemd
#   LOCAL_ARCHIVE=path.tar.gz      用本地压缩包代替下载（离线 / 测试）
set -euo pipefail

REPO="quant-on-quest/johnnybt-pypi"
PREFIX="${PREFIX:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-/var/lib/pypi-server}"
ETC_DIR="${ETC_DIR:-/etc/pypi-server}"
GH_PROXY="${GH_PROXY:-}"
SERVICE="pypi-server"
SVC_USER="pypi-server"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "需要 $1，请先安装（apt install $1 / yum install $1）"; }

# ---- 平台 -------------------------------------------------------------------
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
[ "$os" = "linux" ] || die "只支持 Linux（当前 $os）；macOS 请直接下载 release 里的 darwin 包"
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "不支持的架构 $(uname -m)" ;;
esac
need curl; need tar; need sha256sum

if [ -z "${NO_SYSTEMD:-}" ]; then
  [ "$(id -u)" -eq 0 ] || die "需要 root：请用 sudo 运行（或设置 NO_SYSTEMD=1 只安装二进制）"
  command -v systemctl >/dev/null 2>&1 || die "没有 systemd；设置 NO_SYSTEMD=1 只安装二进制后自行托管"
fi

# ---- 版本 -------------------------------------------------------------------
latest_version() {
  local v=""
  # 1) GitHub API（最准确）
  v="$(curl -fsSL --max-time 15 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)" || true
  # 2) 跟随 releases/latest 的跳转（API 不通、走代理时用）
  if [ -z "$v" ]; then
    v="$(curl -fsSL --max-time 15 -o /dev/null -w '%{url_effective}' "${GH_PROXY}https://github.com/${REPO}/releases/latest" 2>/dev/null \
        | sed -n 's|.*/tag/\(v[^/?#]*\).*|\1|p')" || true
  fi
  [ -n "$v" ] || die "无法获取最新版本号（GitHub 不通？可设置 GH_PROXY=https://ghfast.top/ 或 VERSION=vX.Y.Z）"
  echo "$v"
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -n "${LOCAL_ARCHIVE:-}" ]; then
  [ -f "$LOCAL_ARCHIVE" ] || die "找不到 $LOCAL_ARCHIVE"
  archive="$LOCAL_ARCHIVE"
  VERSION="${VERSION:-local}"
  log "使用本地压缩包 $archive"
else
  VERSION="${VERSION:-$(latest_version)}"
  case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
fi

current=""
if [ -x "$PREFIX/pypi-server" ]; then
  current="$("$PREFIX/pypi-server" version 2>/dev/null || true)"
fi
if [ -n "$current" ] && [ "$current" = "$VERSION" ] && [ -z "${FORCE:-}" ]; then
  log "已是 $VERSION，无需升级（FORCE=1 可强制重装）"
  exit 0
fi

# ---- 下载 + 校验 -------------------------------------------------------------
if [ -z "${LOCAL_ARCHIVE:-}" ]; then
  name="pypi-server_${VERSION}_${os}_${arch}.tar.gz"
  base="${GH_PROXY}https://github.com/${REPO}/releases/download/${VERSION}"
  log "下载 $name"
  curl -fL --progress-bar --retry 3 -o "$tmp/$name" "$base/$name" || die "下载失败：$base/$name"
  curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt" || die "下载 checksums.txt 失败"
  expected="$(grep " $name\$" "$tmp/checksums.txt" | awk '{print $1}')"
  [ -n "$expected" ] || die "checksums.txt 里没有 $name"
  actual="$(sha256sum "$tmp/$name" | awk '{print $1}')"
  [ "$expected" = "$actual" ] || die "sha256 校验失败：期望 $expected 实际 $actual"
  log "sha256 校验通过"
  archive="$tmp/$name"
fi

mkdir -p "$tmp/pkg"
tar -xzf "$archive" -C "$tmp/pkg"
[ -x "$tmp/pkg/pypi-server" ] || die "压缩包里没有 pypi-server 可执行文件"
got="$("$tmp/pkg/pypi-server" version)"

# ---- 安装二进制（原子替换，正在运行的进程不受影响） ------------------------------------
mkdir -p "$PREFIX"
install -m 0755 "$tmp/pkg/pypi-server" "$PREFIX/pypi-server.new"
mv -f "$PREFIX/pypi-server.new" "$PREFIX/pypi-server"
if [ -n "$current" ]; then
  log "二进制已升级：$current → $got（$PREFIX/pypi-server）"
else
  log "二进制已安装：$got（$PREFIX/pypi-server）"
fi

# ---- 配置模板（已存在则不动） ---------------------------------------------------
if mkdir -p "$ETC_DIR" 2>/dev/null; then
  if [ ! -f "$ETC_DIR/env" ]; then
    install -m 0600 "$tmp/pkg/env.example" "$ETC_DIR/env"
    log "已生成配置文件 $ETC_DIR/env（全部注释，按需打开）"
  else
    install -m 0644 "$tmp/pkg/env.example" "$ETC_DIR/env.example"
    log "配置文件 $ETC_DIR/env 已存在，保持不变（新版模板见 env.example）"
  fi
else
  warn "无法写入 $ETC_DIR，跳过配置模板"
fi

if [ -n "${NO_SYSTEMD:-}" ]; then
  log "NO_SYSTEMD=1：不配置 systemd。手动启动：PYPI_DATA_DIR=... $PREFIX/pypi-server"
  exit 0
fi

# ---- 用户、数据目录、systemd -----------------------------------------------------
if ! id -u "$SVC_USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin "$SVC_USER" 2>/dev/null \
    || useradd --system --home-dir "$DATA_DIR" --shell /sbin/nologin "$SVC_USER"
  log "已创建系统用户 $SVC_USER"
fi
mkdir -p "$DATA_DIR"
chown -R "$SVC_USER:$SVC_USER" "$DATA_DIR"
chmod 750 "$DATA_DIR"

# 单元文件由本脚本维护；个性化改动请放 ${UNIT_DIR}/pypi-server.service.d/override.conf
sed -e "s|^ExecStart=.*|ExecStart=$PREFIX/pypi-server|" \
    -e "s|/var/lib/pypi-server|$DATA_DIR|g" \
    -e "s|/etc/pypi-server/env|$ETC_DIR/env|g" \
    "$tmp/pkg/pypi-server.service" > "$UNIT_DIR/$SERVICE.service"
systemctl daemon-reload

if systemctl is-active --quiet "$SERVICE"; then
  systemctl restart "$SERVICE"
  log "服务已重启"
else
  systemctl enable --now "$SERVICE" >/dev/null 2>&1
  log "服务已启用并启动"
fi

for _ in $(seq 1 20); do
  systemctl is-active --quiet "$SERVICE" && break
  sleep 0.5
done
if ! systemctl is-active --quiet "$SERVICE"; then
  systemctl --no-pager -l status "$SERVICE" || true
  die "服务没有正常启动，看日志：journalctl -u $SERVICE -n 50"
fi

# ---- 收尾提示 -----------------------------------------------------------------
echo
domains="$(sed -n 's/^PYPI_TLS_DOMAINS=//p' "$ETC_DIR/env" 2>/dev/null | head -n1 | cut -d, -f1)"
addr="$(sed -n 's/^PYPI_ADDR=//p' "$ETC_DIR/env" 2>/dev/null | head -n1)"
admin_path="$(sed -n 's/^PYPI_ADMIN_PATH=//p' "$ETC_DIR/env" 2>/dev/null | head -n1)"
admin_path="${admin_path:-/admin}"
if [ -n "$domains" ]; then
  url="https://$domains"
else
  port="${addr##*:}"; port="${port:-8080}"
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  url="http://${ip:-127.0.0.1}:$port"
fi
if [ -f "$DATA_DIR/initial_admin_password" ]; then
  echo "首次启动已生成管理员账号（登录后请删除 $DATA_DIR/initial_admin_password）："
  sed 's/^/    /' "$DATA_DIR/initial_admin_password"
  echo
fi
cat <<MSG
客户自查页:   $url/
管理后台:     $url$admin_path
配置文件:     $ETC_DIR/env   （改完 sudo systemctl restart $SERVICE）
日志:         journalctl -u $SERVICE -f
升级:         再执行一次本脚本
MSG
