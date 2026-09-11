# johnnybt-pypi

私有 PyPI 服务，用来分发收费的 Python 包。一个 Go 二进制，内嵌管理界面，`pip` / `uv` / `twine` 原生兼容，不需要 Docker、不需要反向代理。

- **按用户授权**：给某个用户开某个包的权限（可设到期日），他的 token 才看得到、装得上
- **客户不用注册**：你在后台建用户 → 授权 → 生成 token → 发给他；客户在首页粘 token 就能自查
- **管理员首启自动生成**：密码打印在日志并写入数据目录；忘了 `pypi-server admin reset-password`
- **HTTPS 内置**：填一个域名就自动申请 Let's Encrypt 证书并续期
- **文件存储可换**：本机目录 / 阿里云 OSS（官方 SDK）/ 腾讯 COS / MinIO / R2 …，改一个 URL 即迁移
- **协议**：PEP 503 HTML + PEP 691 JSON、PEP 658 wheel 元数据、`/legacy/` 上传（`uv publish` / `twine`）

技术栈：Go 1.27 标准库 `net/http`、SQLite（纯 Go，无 cgo）、`gocloud.dev/blob`、Vue 3 + Nuxt UI。

---

## 目录

1. [安装](#1-安装)
2. [首次登录](#2-首次登录)
3. [域名与 HTTPS](#3-域名与-https)
4. [阿里云 OSS 存储](#4-阿里云-oss-存储)
5. [日常使用](#5-日常使用)
6. [升级](#6-升级)
7. [配置项一览](#7-配置项一览)
8. [运维](#8-运维)
9. [安全边界](#9-安全边界)
10. [开发](#10-开发)

---

## 1. 安装

要求：Linux（x86_64 或 arm64）+ systemd。阿里云 / 腾讯云的 Ubuntu、Debian、Alibaba Cloud Linux、CentOS/Rocky 都行。

### 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/quant-on-quest/johnnybt-pypi/main/scripts/install.sh | sudo bash
```

国内服务器连不上 GitHub 时走加速代理（把 `ghfast.top` 换成任何可用的 GitHub 代理都可以）：

```bash
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/quant-on-quest/johnnybt-pypi/main/scripts/install.sh \
  | sudo GH_PROXY=https://ghfast.top/ bash
```

脚本做的事（全部幂等，重复执行安全）：

| 步骤 | 位置 |
|---|---|
| 下载对应架构的 release 包，校验 sha256 | — |
| 安装二进制 | `/usr/local/bin/pypi-server` |
| 创建系统用户 `pypi-server`、数据目录 | `/var/lib/pypi-server/` |
| 生成配置文件（已存在则不动） | `/etc/pypi-server/env` |
| 安装 systemd 单元并启动 | `/etc/systemd/system/pypi-server.service` |

结束时会打印管理员密码和访问地址。

可选参数（环境变量）：`VERSION=v0.2.0` 装指定版本、`PREFIX` / `DATA_DIR` / `ETC_DIR` 改目录、`NO_SYSTEMD=1` 只装二进制。

### 手动安装

```bash
# 从 https://github.com/quant-on-quest/johnnybt-pypi/releases 下载对应平台的包
tar xzf pypi-server_v0.1.0_linux_amd64.tar.gz
sudo install -m 755 pypi-server /usr/local/bin/
sudo useradd --system --home-dir /var/lib/pypi-server --shell /usr/sbin/nologin pypi-server
sudo mkdir -p /var/lib/pypi-server /etc/pypi-server
sudo chown pypi-server:pypi-server /var/lib/pypi-server
sudo install -m 600 env.example /etc/pypi-server/env
sudo cp pypi-server.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now pypi-server
```

> `go install github.com/quant-on-quest/johnnybt-pypi/cmd/pypi-server@latest` 也能编出二进制，但**不含管理界面**（前端构建产物不在源码里）。请用 release 包，或自己 `make build`。

## 2. 首次登录

服务第一次启动会生成管理员账号，密码只显示一次：

```bash
sudo journalctl -u pypi-server -n 30          # 日志里有
sudo cat /var/lib/pypi-server/initial_admin_password   # 也写在这里，登录后请删除
```

- 客户自查页（首页）：`http://<服务器IP>:8080/`
- 管理后台：`http://<服务器IP>:8080/admin`（前缀可改，见 `PYPI_ADMIN_PATH`）

安全组要放行 8080（或者直接做完第 3 节，用 443）。忘记密码：

```bash
sudo -u pypi-server PYPI_DATA_DIR=/var/lib/pypi-server pypi-server admin reset-password
```

## 3. 域名与 HTTPS

不配域名也能用（`http://IP:8080`），但正式给客户用建议配上：token 走 Basic auth，明文 HTTP 等于裸奔。

### 3.1 准备

1. **DNS**：在域名服务商加一条 A 记录，`pypi.example.com → 服务器公网 IP`。`dig +short pypi.example.com` 能解析出来再往下。
2. **安全组 / 防火墙**：入方向放行 TCP **80** 和 **443**。80 不能省——Let's Encrypt 通过 80 端口验证域名归属，之后 80 只负责跳转到 443。
3. **备案**：服务器在中国大陆地域（阿里云华东/华北/华南等）时，域名必须完成 ICP 备案，否则 80/443 会被运营商拦截。香港、新加坡等海外地域不需要。

### 3.2 配置

编辑 `/etc/pypi-server/env`：

```bash
PYPI_TLS_DOMAINS=pypi.example.com
PYPI_TLS_EMAIL=you@example.com        # 可选，证书到期前 Let's Encrypt 会邮件提醒
```

```bash
sudo systemctl restart pypi-server
```

就这样。设置了 `PYPI_TLS_DOMAINS` 之后：

- 监听自动变为 `:443`（服务）和 `:80`（ACME 验证 + 301 跳转 https）
- 第一次有人访问时申请证书，几秒钟；证书缓存在 `/var/lib/pypi-server/certs/`，到期前自动续
- 对外地址 `PYPI_BASE_URL` 自动为 `https://pypi.example.com`，安装片段里的地址随之正确
- 多个域名用逗号分隔，第一个作为对外地址

验证：

```bash
curl -I https://pypi.example.com/            # 200，证书有效
curl -I http://pypi.example.com/             # 301 → https
sudo journalctl -u pypi-server -f            # 看 ACME 过程；失败通常是 80 没通或 DNS 没生效
```

### 3.3 已经有 nginx / Caddy 在前面

不设 `PYPI_TLS_DOMAINS`，改成内网监听 + 告诉服务对外地址：

```bash
PYPI_ADDR=127.0.0.1:8080
PYPI_BASE_URL=https://pypi.example.com
```

nginx 片段：

```nginx
server {
    listen 443 ssl;
    server_name pypi.example.com;
    # ssl_certificate ...

    client_max_body_size 600m;           # 上传 wheel
    proxy_request_buffering off;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
    }
}
```

## 4. 阿里云 OSS 存储

默认文件存在本机 `/var/lib/pypi-server/blobs/`，小规模够用。放到 OSS 的好处：下载 302 到 OSS 的签名 URL，**带宽和流量不经过你的 ECS**（ECS 公网带宽通常是瓶颈和主要成本）；服务器坏了数据也在。

OSS 走的是阿里云**官方 Go SDK**（不是 S3 兼容层——那层不支持 AWS SDK 的分块上传，踩过坑了）。腾讯 COS、MinIO、Cloudflare R2 通过 S3 协议接入，见 4.5。

### 4.1 创建 Bucket

OSS 控制台 → 创建 Bucket：

| 项 | 选 | 说明 |
|---|---|---|
| 地域 | 随意，建议离客户近 | 服务器和桶不必同地域 |
| 存储类型 | 标准存储 | |
| **读写权限** | **私有** | 绝不能公共读，否则谁都能下 |
| 版本控制 | 不开启 | |

记下 Bucket 名和地域 ID（Bucket 概览页「访问端口」里 Endpoint 去掉 `oss-` 前缀和 `.aliyuncs.com` 就是，例如 `oss-cn-guangzhou.aliyuncs.com` → `cn-guangzhou`）。

### 4.2 创建 RAM 子账号（不要用主账号 AccessKey）

RAM 控制台 → 用户 → 创建用户，勾选「使用永久 AccessKey 访问」，记下 **AccessKey ID** 和 **AccessKey Secret**（Secret 只显示一次）。

然后给这个用户加一条**自定义权限策略**，只允许操作这一个 Bucket：

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["oss:GetObject", "oss:PutObject", "oss:DeleteObject", "oss:GetBucketInfo"],
      "Resource": ["acs:oss:*:*:my-bucket", "acs:oss:*:*:my-bucket/*"]
    }
  ]
}
```

把 `my-bucket` 换成你的 Bucket 名。

### 4.3 配置

编辑 `/etc/pypi-server/env`（文件权限已是 600，只有 root 可读）：

```bash
PYPI_BLOB_URL=oss://my-bucket?region=cn-guangzhou
OSS_ACCESS_KEY_ID=LTAI5t...            # RAM 用户的 AccessKey ID
OSS_ACCESS_KEY_SECRET=...              # AccessKey Secret
```

URL 参数：

| 参数 | 说明 |
|---|---|
| `oss://my-bucket` | Bucket 名 |
| `region=cn-guangzhou` | **必填**，Bucket 所在地域 ID |
| `prefix=pypi/` | 可选，所有对象放到桶内这个目录下，方便和别的东西共用一个桶 |
| `endpoint=` | 可选，默认按 region 生成公网 Endpoint；自定义域名 / 加速域名时填 |
| `internal=true` | 可选，走内网 Endpoint（见 4.5 的取舍） |

### 4.4 验证，然后重启

**不要直接重启**，先用自检命令把写入、读回、签名 URL 下载、删除全走一遍：

```bash
sudo -u pypi-server bash -c 'set -a; source /etc/pypi-server/env; set +a; PYPI_DATA_DIR=/var/lib/pypi-server pypi-server blob check'
```

正常输出：

```
存储: oss://my-bucket?region=cn-guangzhou
签名 URL: true
✓ 写入探测对象 _probe/3ce9e4aa8896091b.txt
✓ 读回并比对
✓ 通过签名 URL 下载（https://my-bucket.oss-cn-guangzhou.aliyuncs.com/_probe/...?x-oss-signature-version=OSS4-HMAC-SHA256&...）
✓ 删除探测对象
存储配置可用
```

常见失败：

| 错误里包含 | 原因 |
|---|---|
| `AccessDenied` | RAM 策略没覆盖这个 Bucket，或 AccessKey 填错 |
| `NoSuchBucket` | Bucket 名不对，或 region 不是它所在的地域 |
| `InvalidAccessKeyId` | AccessKey ID 不存在 / 被禁用 |
| DNS 解析失败 | region 拼错（应形如 `cn-guangzhou`，不带 `oss-`） |
| 签名 URL 下载 403 | 时间不同步（`timedatectl`），或 Bucket 开了特殊的防盗链 / Referer 白名单 |

通过之后：

```bash
sudo systemctl restart pypi-server
```

已经在本机存了文件的话，先把 `/var/lib/pypi-server/blobs/` 整体同步到桶里（目录结构就是对象 key，原样拷）：

```bash
# ossutil：https://help.aliyun.com/zh/oss/developer-reference/ossutil
ossutil cp -r /var/lib/pypi-server/blobs/ oss://my-bucket/
```

### 4.5 几个要知道的事

- **内网 Endpoint 的取舍**。ECS 和 OSS 同地域时 `internal=true` 上传下载不计流量费，但**签名 URL 里也会是内网域名，客户打不开**。所以：默认（`PYPI_BLOB_SIGNED_URLS=true`，客户直连 OSS）必须走公网；如果你更在意 OSS 外网流量费，可以 `PYPI_BLOB_SIGNED_URLS=false` + `internal=true`，让 ECS 转发，那时走的就是 ECS 的公网带宽。
- **费用**：OSS 公网下行流量按量计费（约 0.5 元/GB），wheel 通常几 MB 到几十 MB，一般可以忽略；量大可以考虑 OSS 的传输加速或 CDN。
- **签名 URL 有效期 10 分钟**，pip/uv 拿到 302 后立刻下载，够用；URL 泄漏出去 10 分钟后就失效。
- **腾讯 COS**：`PYPI_BLOB_URL=s3://my-bucket-1250000000?endpoint=https://cos.ap-guangzhou.myqcloud.com&region=ap-guangzhou&request_checksum_calculation=when_required&response_checksum_validation=when_required`，凭证放 `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`（填 COS 的 SecretId / SecretKey）。
- **MinIO / 自建**：`s3://pypi?endpoint=http://minio.internal:9000&region=us-east-1&use_path_style=true&disable_https=true`。
- **迁移到别的服务商**：用 `rclone sync`（或 `ossutil` / `coscli`）把旧桶拷到新桶，改 `PYPI_BLOB_URL`，重启。数据库里只存相对 key，不含桶信息。
- 需要 GCS / Azure：`internal/blob/gocloud.go` 加一行 `_ "gocloud.dev/blob/gcsblob"` 重新编译。

## 5. 日常使用

### 5.1 发布包

后台「用户」页 → 点 `admin` → 生成一个 **write** token。然后在你的包目录：

```bash
uv build
uv publish --publish-url https://pypi.example.com/legacy/ --username __token__ --password jbt_xxx dist/*
```

twine 也行：`twine upload --repository-url https://pypi.example.com/legacy/ -u __token__ -p jbt_xxx dist/*`。或者直接在后台「上传」页拖文件。

同一文件名不允许重复上传（twine 的 `--skip-existing` 能识别）；要覆盖先在包页面删掉那个版本。发错了用 **yank**：版本还在，但 pip/uv 不会再自动选它。

### 5.2 给客户开通

1. 「用户」→ 新建用户，名字随意（GitHub 名 / 微信名），备注写公司、付款信息
2. 进入该用户 → 选包、填到期日（留空 = 永久）→ 授权
3. 生成 token（read）。页面会**显示一次** token，并附三步走的 uv 用法（登录 → 全局索引 → 正常用），整段复制发给客户

到期后 pip 会收到 403（提示信息清楚），续费就改到期日。客户 token 泄漏了就吊销再发一个。

### 5.3 客户怎么装（uv 优先）

需要 uv ≥ 0.8.18（旧版 `uv self update`）。前两步只做一次：

```bash
# 1. 把 token 存进 uv 的凭证仓库（只发给这个地址，不进 shell 历史、不进 uv.lock）
uv auth login https://pypi.example.com/simple/ --token jbt_xxx
```

```toml
# 2. ~/.config/uv/uv.toml（macOS / Linux）或 %APPDATA%\uv\uv.toml（Windows）
[[index]]
name = "johnnybt"
url = "https://pypi.example.com/simple/"
authenticate = "always"
```

之后就是普通 uv：

```bash
uv add johnnybt          # 加进项目
uvx johnnybt-cli         # 直接跑包里的命令行工具
uv pip install johnnybt  # 装进当前环境
```

公开包照常从 PyPI 来：uv 先问私有索引，我们对不认识的名字回 404，uv 就去 PyPI；只有你发布的包才从这里拿。`authenticate = "always"` 让 uv 第一次就带凭证，省掉一次 401 往返。

**高级用法**（后台生成 token 的页面里也有，折叠在「高级」下）：

- **项目级钉死**，只在这个项目启用私有索引，其它包名根本不会来探，也是防依赖混淆的正解：
  ```toml
  # pyproject.toml
  [[tool.uv.index]]
  name = "johnnybt"
  url = "https://pypi.example.com/simple/"
  explicit = true

  [tool.uv.sources]
  johnnybt = { index = "johnnybt" }
  ```
- **CI / 容器**没法 `uv auth login` 时用环境变量（Secret 注入）：`UV_INDEX_JOHNNYBT_USERNAME=__token__`、`UV_INDEX_JOHNNYBT_PASSWORD=jbt_xxx`
- **pip**：`~/.pip/pip.conf` 里 `[global] extra-index-url = https://__token__:jbt_xxx@pypi.example.com/simple/`

客户随时可以打开首页 `https://pypi.example.com/` 粘 token，看自己有哪些包、什么时候到期，页面上同样有这套说明。

## 6. 升级

再跑一次安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/quant-on-quest/johnnybt-pypi/main/scripts/install.sh | sudo bash
```

它会下载最新 release、校验、原子替换二进制、`systemctl restart`。配置和数据不动。数据库结构变更在启动时自动迁移。

- 装指定版本：`VERSION=v0.2.0 ... | sudo bash`
- 看当前版本：`pypi-server version`
- 回滚：`VERSION=v0.1.0 FORCE=1 ... | sudo bash`

## 7. 配置项一览

全部在 `/etc/pypi-server/env`，改完 `sudo systemctl restart pypi-server`。

| 变量 | 默认 | 说明 |
|---|---|---|
| `PYPI_TLS_DOMAINS` | 空 | 逗号分隔的域名；设置即启用内置 HTTPS（见第 3 节） |
| `PYPI_TLS_EMAIL` | 空 | Let's Encrypt 账号邮箱（可选） |
| `PYPI_ADDR` | `:8080`（TLS 时 `:443`） | 监听地址 |
| `PYPI_HTTP_ADDR` | `:80` | 仅 TLS 模式：ACME + 跳转的明文端口 |
| `PYPI_BASE_URL` | 按请求推断（TLS 时 `https://<第一个域名>`） | 对外地址；反向代理后面必须设 |
| `PYPI_ADMIN_PATH` | `/admin` | 管理后台前缀。首页永远是客户自查页；想藏后台就设成 `/manage-x7k2` 之类 |
| `PYPI_BLOB_URL` | 空 = `$PYPI_DATA_DIR/blobs` | 对象存储地址：`oss://`（阿里云）或 `s3://`（其它），见第 4 节 |
| `PYPI_BLOB_SIGNED_URLS` | `true` | 下载 302 到签名 URL；`false` 由本机转发 |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | 空 | 阿里云 OSS 凭证 |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | 空 | S3 兼容服务的凭证 |
| `PYPI_DATA_DIR` | `/var/lib/pypi-server`（systemd 单元里设） | SQLite、本地文件、证书、初始密码 |
| `PYPI_SESSION_TTL` | `168h` | 后台登录有效期 |
| `PYPI_MAX_UPLOAD_MB` | `512` | 单文件上传上限 |
| `TZ` | 系统 | 授权到期日按此时区的当天结束计算；建议 `Asia/Shanghai` |

## 8. 运维

```bash
sudo systemctl status pypi-server
sudo journalctl -u pypi-server -f              # 每个请求一行：方法、路径、状态、耗时、IP、UA
pypi-server version
```

**备份**：`/var/lib/pypi-server/` 整个目录（SQLite 在 WAL 模式，直接拷文件在绝大多数情况下可用；严格一点用 `sqlite3 pypi.db ".backup out.db"`）。用了 OSS 的话文件在桶里，本机只剩数据库和证书。

**改后台路径**：`PYPI_ADMIN_PATH=/manage-x7k2`，重启后旧的 `/admin` 就是普通 404，后台在新路径下。

**自定义 systemd 单元**：不要直接改 `/etc/systemd/system/pypi-server.service`（升级会覆盖），放到 `sudo systemctl edit pypi-server` 打开的 override 里。

**卸载**：

```bash
sudo systemctl disable --now pypi-server
sudo rm /etc/systemd/system/pypi-server.service /usr/local/bin/pypi-server
sudo rm -r /etc/pypi-server /var/lib/pypi-server      # 数据也一起删，慎重
sudo userdel pypi-server
```

## 9. 安全边界

- index 页按 token 裁剪，返回 `Cache-Control: no-store`，避免共享缓存把 A 的列表给 B（uv 的本机缓存会忽略 URL 里的凭证，这一点被测试钉死）
- token 只存 sha256；管理员密码 argon2id；后台 API 有跨站请求保护；`/etc/pypi-server/env` 是 600
- 未授权访问包返回 403
- 服务器只能管"谁能下载"。wheel 下下来就能复制，代码本身的保护（编译、许可证校验）是另一个问题
- **依赖混淆**：uv 的 first-index 策略下私有索引排在 PyPI 前面，你有的包一定从这里拿；pip 用户若用 `--extra-index-url` 则公网同名包会被优先。稳妥做法：在 PyPI 上占坑同名空壳包，或项目里 `[[tool.uv.index]] explicit = true` + `[tool.uv.sources]` 钉死

## 10. 开发

```bash
make            # 前端 + Go → bin/pypi-server
make dev        # Go :8080；另开终端 make dev-web 得到 Vite HMR :5173
make test       # Go + 前端测试 + 类型检查
make e2e        # 真实 uv publish → 授权 → uv pip install 回归
make release    # 本地交叉编译出 dist/*.tar.gz + checksums.txt
```

约定：**测试先行**。新端点先写 `_test.go` 里的 httptest 用例，新页面先写 `*.test.ts`。

发版：`git tag v0.2.0 && git push origin v0.2.0`，GitHub Actions 会构建三个平台的包并创建 release，安装脚本随即可用。

```
cmd/pypi-server/     入口；子命令 admin reset-password / blob check / version
internal/
  pypi/              /simple /files /legacy —— pip/uv 面对的协议层
  api/               /api/v1 —— 管理界面用的 JSON API
  store/             SQLite 与迁移（migrations/*.sql 内嵌）
  blob/              存储接口：本地目录 + 阿里云 OSS 官方 SDK + gocloud（s3/file/mem）+ 自检
  pkgmeta/           包名归一化、文件名解析、METADATA 抽取
  auth/              argon2 密码、token 生成/校验
  server/            路由拼装、中间件、autocert
  web/               内嵌构建好的前端，注入 adminPath
web/                 Vite + Vue 3 + Nuxt UI
scripts/             install.sh（安装/升级）、build-release.sh、e2e.sh
deploy/              systemd 单元、env 模板
```

## License

MIT
