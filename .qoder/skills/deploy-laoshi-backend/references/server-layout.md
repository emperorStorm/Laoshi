# Laoshi 后端线上布局（2026-11-19 实测）

## 主机与目录

- SSH 别名：`ecs-114-55-112-83`，用户 `root`
- ECS 主机名：`iZbp15dz2x7l47nmbhhlmtZ`，架构 `x86_64`
- 上传目录：`/home/package`
- 后端根目录：`/opt/laoshi/backend`
- 版本目录：`/opt/laoshi/backend/releases/<version>`
- 当前软链：`/opt/laoshi/backend/current`，实测指向 `releases/20261118-0e68b59`
- 环境文件：`/etc/laoshi/api.env`，权限 `600 root:root`
- 日志：`/var/log/laoshi/backend-deploy-<version>.log`
- 数据库备份：`/var/backups/laoshi/`（目录初始不存在，脚本会创建为 `700`）

已保留的历史版本示例：`v1.1.0`、`20261105T070616Z-...`、`20261105T083908Z-...`、`20261118-0e68b59`。

## 版本目录内容

```text
/opt/laoshi/backend/releases/<version>/
├── laoshi-api                 # linux/amd64，root:root 755
├── openapi.yaml
├── release.json
└── db/migrations/*.sql
```

`release.json` 含版本、appVersion、打包时间、Go 版本、GOOS/GOARCH、迁移数量、Git 提交及 dirty 状态。

## systemd 单元

`/etc/systemd/system/laoshi-api.service` 的关键行：

```ini
WorkingDirectory=/opt/laoshi/backend/current
EnvironmentFile=/etc/laoshi/api.env
ExecStart=/opt/laoshi/backend/current/laoshi-api
```

服务用户 `laoshi`，监听 `127.0.0.1:8787`，`MemoryMax=384M`。部署脚本会校验这三行，不一致就停止，不会改单元。

## Nginx

- 站点配置：`/etc/nginx/conf.d/laoshi.conf`
- API 片段：`/etc/nginx/snippets/laoshi-api-location.conf`
- `location ^~ /api/` 代理到 `http://127.0.0.1:8787`，保留完整 `/api` 前缀，`proxy_intercept_errors off`。
- 后端单包发布不需要改 Nginx，也不需要 reload。

## 数据库与工具

- `pg_dump` 位于 `/usr/bin/pg_dump`，版本 PostgreSQL 17.11。
- 数据库连接串只在 `/etc/laoshi/api.env`，脚本用子 shell 加载，不输出。
- 服务器没有 Go 工具链，必须在本地交叉编译 `linux/amd64`。
- 磁盘 `/` 约 40G，实测可用约 28G；脚本要求至少 200MB。

## 只读检查命令

```bash
ssh ecs-114-55-112-83 'uname -m'
ssh ecs-114-55-112-83 'systemctl is-active laoshi-api'
ssh ecs-114-55-112-83 'systemctl cat laoshi-api | sed -n "s/^ExecStart=//p"'
ssh ecs-114-55-112-83 'readlink -f /opt/laoshi/backend/current'
ssh ecs-114-55-112-83 'ls -la /opt/laoshi/backend/releases'
ssh ecs-114-55-112-83 'curl -sS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8787/healthz'
```

## 健康与路由验证

```bash
# 本机
curl -fsS http://127.0.0.1:8787/healthz
curl -fsS http://127.0.0.1:8787/readyz

# 公网：未认证应为 401 JSON
curl -sS -o /dev/null -w '%{http_code} %{content_type}\n' https://laoshi.chat/api/v1/auth/me

# 未知 API 路径应为 404 纯文本，不能是主站 HTML
curl -sS -o /dev/null -w '%{http_code} %{content_type}\n' https://laoshi.chat/api/v1/definitely-missing
```

## 手工回滚

自动回滚由 `remote-deploy.sh` 完成。需要人工回滚时，先确认旧版本目录存在且二进制完整：

```bash
OLD=releases/<旧版本>
sudo test -x "/opt/laoshi/backend/$OLD/laoshi-api"
NEXT=/opt/laoshi/backend/.rollback-$$
sudo ln -s "$OLD" "$NEXT"
sudo mv -Tf "$NEXT" /opt/laoshi/backend/current
sudo systemctl restart laoshi-api
curl -fsS http://127.0.0.1:8787/readyz
```

数据库迁移不会自动回退。只有确认旧二进制与新 schema 不兼容时，才评估用 `/var/backups/laoshi/*.dump` 恢复；恢复前先停止 API，并再次备份当前库。

## 变更影响

| 改动 | 需要 |
| --- | --- |
| Go 二进制 / 迁移 | 上传新版本、迁移、切 current、重启 `laoshi-api` |
| `/etc/laoshi/api.env` | 重启 `laoshi-api` |
| `laoshi-api.service` | `systemctl daemon-reload` 后重启 |
| Nginx 配置 | `nginx -t` 成功后 reload |
| 前端静态站 | 不走本 skill，按 `deploy/README.md` 切 current |

## 保留策略

- 旧 release、数据库备份、`/home/package` 归档都不自动删除。
- 需要清理时先列清单并让用户确认；至少保留当前版本和最近一个可回滚版本。
