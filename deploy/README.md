# 师说 Laoshi 部署（Alibaba Cloud Linux / Nginx）

工程名 `laoshi`，正式域名 https://laoshi.chat。主宣言“终身求知，以诚对话。”（Keep learning. Connect through dialogue.），理念“学无止境，彼此为师；认真倾听，让理解发生。”。本站是分享软件作品与学习所得的作品门户，不提供站内聊天功能。以下为待管理员执行的部署步骤，不代表已上线；TLS 证书仍需准备。

默认包用于 Vue 3 + Vite SSG 静态站，不需要 Node.js 服务端进程；显式选择完整包后，还包含统一 Go API 与迁移文件（见第 7 节）。下面的服务器命令供管理员手工执行；打包脚本只操作本地文件，不连接服务器、不部署、不自动清理历史版本。

## 1. 本地构建与打包

需要 Node.js 22 LTS（建议 22.12+，同时满足项目所用 Vite 的版本要求）和系统 `tar`。在项目根目录执行已配置的 npm 命令（`package:deploy` 会先构建，再执行 `node scripts/package-deploy.mjs`）：

```bash
npm ci
npm run build
npm run package:deploy
```

`npm run package:deploy` 会先执行完整构建，再检查 `dist/index.html`、`dist/404.html` 并打包。SSG 页面输出为 `dist/work/slug/index.html`，支持 `/work/slug/`，不使用 SPA 首页兜底。底层脚本 `node scripts/package-deploy.mjs` 只打包现有 dist；日常发布应使用 npm 命令，确保产物与环境变量一致。

本地没有域名时可以不设置 `SITE_URL`。此时 release.json 的 siteUrl 为 null，可以生成离线检查包。**正式域名已确定为 https://laoshi.chat，生产发布须设置 SITE_URL 重新构建，再用相同 SITE_URL 打包**，以便构建工具生成正确的 canonical、站点地图、Open Graph 等绝对链接。只在打包时设置 SITE_URL 不会改写已有 HTML。

```bash
SITE_URL=https://laoshi.chat npm run build
SITE_URL=https://laoshi.chat RELEASE_ID=20261022-001 npm run package:deploy
```

SITE_URL 只接受 HTTPS 站点根地址，不允许凭据、子路径、查询或片段。RELEASE_ID 可选，必须以 ASCII 字母或数字开头，总长 1–96 位，只包含字母、数字、`.`、`_`、`-`。不允许 `/`、空格或目录穿越。省略时生成 UTC 时间、Git 短提交（无 Git 则为 nogit）及随机后缀组成的版本。

输出为 `artifacts/laoshi-<version>.tar.gz`，同名文件存在时失败，不覆盖。脚本确认项目根目录存在后创建 artifacts。归档仅包含：

- `web/`：dist 静态文件；不含 macOS `.DS_Store`。
- `nginx/http.conf`、`nginx/tls.conf`、`nginx/security-headers.conf`。
- `README.md`：本文。
- `release.json`：版本、package.json 的 appVersion（可为空）、打包时间、SITE_URL、Git 提交及工作区状态（无 Git 为 null）。

脚本拒绝符号链接、特殊文件、源码扩展名、source map、node_modules、常见密钥文件及带 PEM 私钥标记的文件。请确保 dist 本身仅含公开构建产物，不要将源码、账号、环境文件或任意格式的秘密复制到 public/dist。私钥仅在服务器上管理，不放入项目和部署包。

## 2. Alibaba Cloud Linux 安装 Nginx

下面使用 Alibaba Cloud Linux 3（ALinux 3）的系统软件源，需有 sudo 权限；其他版本需先确认对应软件源可用。

```bash
sudo dnf install -y nginx
sudo systemctl enable --now nginx
sudo install -d -m 755 /opt/laoshi/releases /etc/nginx/snippets
```

在云安全组放行所需的 TCP 80/443。若已启用 firewalld，再按实际需要允许 http/https；不要为上线关闭防火墙或 SELinux。若 SELinux enforcing 且有拒绝访问，检查审计日志，按系统工具给 `/opt/laoshi` 设置可被 httpd 读取的持久文件上下文（通常为 httpd_sys_content_t）。确保 Nginx 用户对所有父目录有执行权限，对静态文件有读权限。

## 3. 上传与解包到新版本目录

先在本地人工核对归档目录，随后上传。以下版本号与主机名均需替换为实际值：

```bash
tar -tzf artifacts/laoshi-20261022-001.tar.gz
scp artifacts/laoshi-20261022-001.tar.gz deploy@your-server:/tmp/
```

登录服务器后，在 Bash 中执行。只解包由自己构建并可信的归档，核对 release.json 与预期版本一致。`mkdir` 故意不带 `-p`，如果版本已存在应停止，改用新版本号，避免覆盖历史版本。

```bash
set -e
VERSION=20261022-001
[[ "$VERSION" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]]
BASE=/opt/laoshi
sudo mkdir -m 755 "$BASE/releases/$VERSION"
sudo tar --no-same-owner -xzf "/tmp/laoshi-$VERSION.tar.gz" -C "$BASE/releases/$VERSION"
sudo chmod -R a+rX "$BASE/releases/$VERSION"
sudo test -f "$BASE/releases/$VERSION/web/index.html"
sudo test -f "$BASE/releases/$VERSION/web/404.html"
sudo less "$BASE/releases/$VERSION/release.json"
```

首次上线可先切换 current，再安装配置。后续发布只需要切换 current；不要每次都复制 Nginx 配置。

## 4. 原子切换 current

同一服务器上的发布和回滚应串行执行。先记下 current 的旧目标供回滚；首次发布没有旧目标是正常情况。

```bash
readlink /opt/laoshi/current || true
```

使用新临时软链加 GNU `mv -T` 原子替换 current，临时软链与 current 位于同一目录、同一文件系统。不要直接对 current 执行普通 `ln -sfn`，也不要预先删掉 current。

```bash
set -e
VERSION=20261022-001
[[ "$VERSION" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]]
BASE=/opt/laoshi
sudo test -f "$BASE/releases/$VERSION/web/index.html"
NEXT="$BASE/.current-$VERSION-$$"
sudo ln -s "releases/$VERSION" "$NEXT"
sudo mv -Tf "$NEXT" "$BASE/current"
```

current 应当是软链，若同名真实目录存在请人工检查，不要删除目录来强行切换。**常规静态版本发布无需 reload Nginx**：Nginx 根据 current 读取文件；模板未启用 open_file_cache。原子切换保证单次路径解析看到有效版本，但页面和资源的多个请求并非一个事务；旧页面的哈希资源若新包已不含，可能 404，应结合访问情况在新 dist 保留仍需服务的旧哈希资源。旧 release 目录保留用于手工回滚。

## 5. 首次安装或修改 Nginx 配置

HTTP 示例可按 IP 或本地域名验证。检查 `/etc/nginx/nginx.conf` 是否 include `/etc/nginx/conf.d/*.conf`，处理系统默认站点和 default_server 冲突。

```bash
sudo install -m 644 /opt/laoshi/current/nginx/security-headers.conf /etc/nginx/snippets/laoshi-security-headers.conf
sudo install -m 644 /opt/laoshi/current/nginx/http.conf /etc/nginx/conf.d/laoshi.conf
sudo nginx -t
sudo systemctl reload nginx
```

只有 `nginx -t` 成功后才能 reload（逐条执行并检查退出状态，或在 `set -e` 下执行）。不要同时启用 HTTP 和 TLS 两套站点模板。TLS 模板已使用 `laoshi.chat`，证书与私钥路径分别为 `/etc/nginx/certs/laoshi.chat/fullchain.pem`、`/etc/nginx/certs/laoshi.chat/privkey.pem`；这些路径不表示证书已经存在。先准备该域名的有效证书与私钥，核对服务器上的实际路径，再将 TLS 模板安装为 `/etc/nginx/conf.d/laoshi.conf`，测试并 reload。证书、续签和私钥权限由服务器管理员维护；部署包不包含证书或私钥。

模板的 root 为 `/opt/laoshi/current/web`。`try_files $uri $uri/ =404` 使 `/work/slug/` 读取目录下的 index.html，缺失页面返回真实 404，并通过 `error_page 404 /404.html` 展示静态错误页。HTML 和普通文件使用 `Cache-Control: no-cache`；只有 assets 下带内容哈希的资源使用一年 immutable 缓存，错误响应不加长期缓存。安全头放入公共片段，声明缓存头的 location 再次 include，避免 Nginx 的 add_header 继承规则使安全头丢失。修改模板时也要遵守此规则。

检查首页、一个真实 SSG 页面和不存在的路径，确认响应码、缓存头和安全头：

```bash
curl -I http://127.0.0.1/
curl -I http://127.0.0.1/work/实际slug/
curl -I http://127.0.0.1/definitely-missing-page
```

TLS 上线后对真实 HTTPS 域名重复检查，并用一个实际 `/assets/文件名-哈希.js` 地址检查长期缓存。不存在的页面应返回 404 而非 200；HTML 应含 no-cache，哈希资源应含 max-age=31536000 和 immutable。

## 已有 /opt/suiworks 的迁移提示

本次品牌与模板更新不自动迁移线上文件或配置。管理员应先备份并核对旧站点，再准备 `/opt/laoshi/releases` 与 `/opt/laoshi/current`；旧、新配置的 `root`、安全头 `include` 路径和 `current` 软链必须配套，不能只改其中一项。启用 `/etc/nginx/conf.d/laoshi.conf` 时须处理旧 `suiworks.conf` 和系统默认站点，避免重复 `server_name` 或 `default_server` 冲突；保留旧配置备份时应放在 Nginx include 范围之外。确认新软链指向完整版本、证书与片段就绪后，执行 `nginx -t`，仅成功后 reload，并检查页面与 404；失败时恢复配套旧配置与软链。普通静态更新只切换 current，无需 reload。旧 release 目录与旧归档均保留，不自动删除。

## 6. 手工回滚

选定此前保留的 release，核对其 release.json，使用相同的原子软链切换步骤，将 VERSION 改为旧版本。回滚完成后用 curl 验证页面。仅切换静态版本不需要 reload；若同时回滚 Nginx 配置，则恢复匹配的配置后，先 `nginx -t` 成功，再 reload。

脚本和以上流程均不自动删除 release 目录或旧归档。磁盘清理由管理员另行评估当前版本、回滚窗口及资源依赖后手工处理。

## 7. 统一 Go API 与岁岁时光

主站仍由 Nginx 直接读取静态文件。客户端访问 `https://laoshi.chat/api/v1/...` 时，Nginx 将完整路径转给本机 `127.0.0.1:8787` 上的 Go 服务；Go 再访问 PostgreSQL。数据库和 8787 端口不对公网开放。认证入口为 `/api/v1/auth/...`，岁岁时光同步入口为 `/api/v1/sui-time/sync/...`。

本地安装 Go 1.23 或更新版本后，生成带 Linux API 二进制的完整包：

```bash
SITE_URL=https://laoshi.chat npm run package:deploy:api
# ARM64 服务器需额外设置 GOARCH=arm64，默认是 amd64。
```

完整包额外包含 `api/laoshi-api`、`api/db/migrations/`、`api/api/openapi.yaml`、`deploy/laoshi-api.service` 和 `deploy/laoshi-api.env.example`。没有真实环境文件、数据库数据、证书或密钥。构建失败会阻断打包。静态包不包含 API，启用 API 后切换 current 必须使用完整包，避免服务下次启动时找不到二进制。

首次部署按以下顺序进行，先在服务器准备独立 PostgreSQL 数据库 `laoshi` 和数据库用户，并为用户授权该库内创建 schema 的权限。请使用管理员在服务器生成的强密码；不要把真实连接串写进仓库。资源较小的服务器先将 PostgreSQL 的 `shared_buffers` 设为 128MB、`work_mem` 设为 4MB，并结合实际负载调整；Go 连接池最多 5 个连接。

1. 沿用主站 `laoshi.chat` 的 DNS 记录和有效 TLS 证书，核对 TLS 模板中的证书路径；主站与 `/api/` 共用域名，无需新增 API 子域名或证书。
2. 创建无登录权限的系统用户/组 `laoshi`，创建 `/etc/laoshi`；从完整包复制环境模板为 `/etc/laoshi/api.env`，填入真实连接串。目录权限设为 750，环境文件设为 600，由 root 管理；systemd 会在降权前读取文件。二进制和发布目录保持可供 laoshi 用户读取、执行。
3. 先备份已有数据库，再使用新版本二进制的 `-migrate` 执行内嵌 goose 迁移，无需额外安装迁移工具。迁移不由 API 启动过程自动执行，也不会自动删除旧数据。命令中的连接串使用服务器私有环境变量，不写入共享脚本：

```bash
# 在可信的管理员 shell 中先设置并导出 DATABASE_URL，避免粘贴到聊天记录。
/opt/laoshi/releases/实际版本/api/laoshi-api -migrate
```

4. 按前文切换 current 到完整包，然后安装服务并启动：

```bash
sudo install -m 644 /opt/laoshi/current/deploy/laoshi-api.service /etc/systemd/system/laoshi-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now laoshi-api
curl --fail http://127.0.0.1:8787/healthz
curl --fail http://127.0.0.1:8787/readyz
sudo journalctl -u laoshi-api -n 50 --no-pager
```

5. 将 TLS 模板替换安装到现有站点配置（不要叠加启用 HTTP 模板），`sudo nginx -t` 成功后 reload。主站 TLS 虚拟主机将 `/api/` 完整转发至 Go，其余路径继续读取静态文件；API 的响应状态和正文原样返回，不回退主站 404 页面。HTTP 示例中的 `/api` 和 `/api/` 会以 308 跳转至主站 HTTPS。若 SELinux 阻止本机反向代理，按服务器审计日志配置 httpd 网络访问策略。
6. 从客户端检查注册、登录及首次同步。未登录请求 `/api/v1/auth/me` 应返回 JSON 401；未知路径返回 404，不展示主站页面。`/healthz`、`/readyz` 只供本机验证，不经公网代理。

桌面构建变量 `SUI_TIME_SYNC_BASE_URL` 和移动端 `VITE_SUI_TIME_SYNC_BASE_URL` 都填写 `https://laoshi.chat`（仅 origin，不带 `/api`）。客户端自行拼接新路由。开发环境变量为空会提示未配置同步服务。同源的 `https://laoshi.chat` 页面调用 `/api/` 无需跨域授权；其他域名或本地开发的浏览器 H5 使用时，在 `CORS_ORIGINS` 中明确填入实际页面 origin，不要配置通配符。

单个同步请求最大 8MiB，纪念日照片合计文本最大 6MiB；Nginx 与 Go 的请求体上限一致。照片暂随同步数据保存，多个大照片实体可能超过整批上限，应缩小批次。提醒继续由客户端本地调度。

后续更新 API：先备份并执行新版本迁移，再切换完整包，执行 `sudo systemctl restart laoshi-api`，验证 readyz 和客户端同步。只更新静态文件无需重启 API，但不得将 current 切到缺少 API 的静态包。修改环境文件需要重启 API；修改 service 需要 daemon-reload 后重启；修改 Nginx 需要测试后 reload。回滚二进制前先确认数据库迁移仍兼容，不自动执行 down。

可交给服务器 AI 的提示词：

> 请在当前 ECS 部署 Laoshi 完整发布包，保留现有主站和历史版本。先只读核对系统架构、Nginx 配置、PostgreSQL、主站 DNS 与 laoshi.chat 证书，确认包中的 release.json。按本说明配置本机 PostgreSQL 和 laoshi-api systemd 服务，真实密钥只放服务器私有环境文件。执行备份、增量迁移、完整包原子切换和健康检查，nginx -t 成功后再 reload，最后验证主站、API JSON 401 和双端同步。不要删除历史数据，不开放数据库和 8787 公网端口；执行有破坏性的操作前先说明并停止。
