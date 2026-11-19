---
name: deploy-laoshi-backend
description: Laoshi 后端 Go API 单包发布：本地交叉编译 linux/amd64、生成 SHA256、上传到阿里云 ECS 114.55.112.83 的 /home/package、备份 PostgreSQL、执行内嵌迁移、原子切换 /opt/laoshi/backend/current、重启 laoshi-api 并做健康检查与失败回滚。用户说“发后端”“部署 Go API”“打包后端上传服务器”“重启 laoshi-api”“后端重新部署”“后端发布”时使用；不用于纯前端静态站发布。
---

# Laoshi 后端单包发布

## 适用范围

只发布 Go API（`server/`），不动前端静态站、不改 Nginx、不改 systemd 单元拓扑。线上布局已实测确认：

- SSH 别名：`ecs-114-55-112-83`（root，密钥登录）
- 上传目录：`/home/package`
- 版本目录：`/opt/laoshi/backend/releases/<version>`
- 当前软链：`/opt/laoshi/backend/current`
- 服务：`laoshi-api`，环境文件 `/etc/laoshi/api.env`
- 服务器：`x86_64`；只构建 `linux/amd64`
- 在本地仓库运行本 skill；服务器没有 Go 工具链，不要在服务器上构建

细节和验证命令见 `references/server-layout.md`，异常处理见 `references/troubleshooting.md`。

## 红线

1. 只允许 `linux/amd64`；架构不符立即停止。
2. 构建失败不得降级为“只发静态包”，也不得跳过迁移。
3. 迁移前必须成功执行 `pg_dump`；备份失败立即停止。
4. 不读取、不打印 `/etc/laoshi/api.env`；不把真实连接串写进命令、日志或回复。
5. 不自动改 systemd 单元、Nginx 或线上目录拓扑；不一致就停止。
6. 不自动删除 release、数据库备份或 `/home/package` 归档。
7. 生产变更前先向用户说明版本、远端当前版本、迁移和备份计划；确认后才加 `--yes`。
8. SSH 不通时保留构建产物并解释，不反复重试、不改用密码登录。

## 执行流程

### 1. 预检 + 构建（不部署）

在仓库根目录执行：

```bash
bash .qoder/skills/deploy-laoshi-backend/scripts/deploy-backend.sh --preflight-only
```

它会：

- 检查 Go 1.23+。本机若 `go` 不在 PATH，脚本会阻断；先征求用户同意再安装，不要静默下载。
- 交叉编译 `linux/amd64`、`CGO_ENABLED=0`、`-mod=readonly`。
- 校验 ELF 架构、`GOOS/GOARCH`、迁移 SQL 数量。
- 生成 `artifacts/backend/laoshi-backend-<version>-linux-amd64.tar.gz` 和 `.sha256`。
- 只读检查 SSH、远端架构、`laoshi-api` 状态、`ExecStart`、`/home/package`。
- 打印部署计划和下一条确认命令，不修改服务器。

版本默认 `<UTC时间>-<短提交>`。需要固定版本时：

```bash
RELEASE_ID=20261119-<sha7> bash .qoder/skills/deploy-laoshi-backend/scripts/deploy-backend.sh --preflight-only
```

只打包、完全不连服务器时用 `--build-only`。

### 2. 请用户确认

报告：版本、Git 提交、是否有未提交改动、迁移文件数、远端当前版本、备份路径规则，并明确会重启 `laoshi-api`。得到确认后再继续。

### 3. 上传并部署

```bash
bash .qoder/skills/deploy-laoshi-backend/scripts/deploy-backend.sh \
  --skip-build --version <version> --yes
```

链路：`scp` 到 `/home/package` → 远端校验 SHA256 → `pg_dump` 到 `/var/backups/laoshi` → 解包到新版本目录 → 新二进制 `-migrate` → 原子切换 `current` → `systemctl restart laoshi-api` → 本机 `healthz/readyz` + 公网 401/404 校验。

任一步在切换后失败，远端脚本自动把 `current` 切回上一版本并重启；数据库迁移不会自动回退。同一版本目录已存在时拒绝覆盖，换新 `RELEASE_ID` 重发。

### 4. 汇报

必须包含：版本、上一版本、产物与 SHA256、数据库备份路径、健康检查结果、是否回滚、`journalctl` 关键输出。部署成功不需要 reload Nginx，也不需要动前端。

## 失败处理

| 现象 | 处理 |
| --- | --- |
| 本机没有 Go / 版本低于 1.23 | 停止；征求同意后安装，不跳过构建 |
| SSH 握手被重置 / 超时 | 产物保留但不上传；提示检查网络/VPN/安全组 22；给出手动上传命令 |
| `ExecStart` 不是 `/opt/laoshi/backend/current/laoshi-api` | 立即停止，不做任何修改，让用户确认拓扑 |
| `pg_dump` 失败 | 停止，当前版本未切换 |
| 迁移失败 | 停止，当前版本未切换；保留新版本目录和备份，禁止启动新二进制 |
| 健康检查失败 | 远端自动回滚；报告备份路径，排查后用新版本号重发 |
| 同一版本目录已存在 | 不覆盖；换 `RELEASE_ID` |
| 公网 502 / 返回主站 HTML | 检查 `laoshi-api` 与 `journalctl`；必要时手工回滚 |

手动上传（自动化通道不可用、但 `ssh` 可用时）：

```bash
scp artifacts/backend/laoshi-backend-<version>-linux-amd64.tar.gz* \
  ecs-114-55-112-83:/home/package/
scp .qoder/skills/deploy-laoshi-backend/scripts/remote-deploy.sh \
  ecs-114-55-112-83:/home/package/deploy-backend-<version>.sh
ssh -t ecs-114-55-112-83 "sudo bash /home/package/deploy-backend-<version>.sh \
  --version <version> \
  --archive /home/package/laoshi-backend-<version>-linux-amd64.tar.gz \
  --sha256-file /home/package/laoshi-backend-<version>-linux-amd64.tar.gz.sha256"
```

## 资源

- `scripts/build-backend.sh`：只构建单包，不上传。
- `scripts/deploy-backend.sh`：本地预检、上传、远端部署编排。
- `scripts/remote-deploy.sh`：服务器端执行脚本，可单独上传运行。
- `references/server-layout.md`：2026-11-19 实测的线上目录、权限、验证命令。
- `references/troubleshooting.md`：SSH、迁移、健康检查、回滚与清理。
