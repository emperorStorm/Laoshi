# 失败排查与回滚

## SSH 连接

症状：`kex_exchange_identification: read: Connection reset by peer`、连接超时，或脚本退出码 4。

1. 先确认本机网络/VPN 是否切换过；切换后重试一次即可。
2. 检查云安全组是否放行当前出口 IP 的 22 端口。
3. `ssh -v ecs-114-55-112-83 true` 看握手停在哪一步。
4. 不要反复重试或改密码登录；可能是 fail2ban/sshd 限制，继续尝试只会延长封禁。
5. 自动化通道不通时，产物已保留；按 `SKILL.md` 的手动上传命令，在 Qoder 远程终端或阿里云控制台执行远端脚本。

SSH 中途断开：远端脚本可能已切换版本。先只读确认，不要直接重跑同一版本：

```bash
readlink -f /opt/laoshi/backend/current
systemctl is-active laoshi-api
tail -n 80 /var/log/laoshi/backend-deploy-<version>.log
curl -fsS http://127.0.0.1:8787/readyz
```

## 构建

- `未找到 go`：安装 Go 1.23+（macOS 可 `brew install go`）；先问用户，不要静默安装。
- `Go 版本过低`：升级到 1.23+。
- 模块下载失败：检查网络/代理，`go env GOPROXY`；不要用 `-mod=mod` 改仓库。
- 产物不是 `linux/amd64`：检查是否设置了异常的 `GOARCH/GOOS` 环境变量；脚本已固定为 `linux/amd64`。
- 工作区有未提交改动：脚本会警告并在 `release.json` 记录 `dirty=true`；发布前建议提交，但不自动提交。
- 同版本归档已存在：换 `RELEASE_ID`，脚本不会覆盖。

## 数据库迁移

- 迁移在切换 `current` 之前执行；失败时当前版本未切换，线上旧版本继续服务。
- 新版本目录会保留用于排查；不要启动新二进制，也不要手工切 `current`。
- goose 迁移按版本事务执行；不要手工执行 down。
- 迁移前备份在 `/var/backups/laoshi/laoshi-<version>-<ts>.dump`。
- 只有确认旧二进制与新 schema 不兼容时才评估恢复：

```bash
sudo systemctl stop laoshi-api
sudo -u postgres pg_restore --clean --if-exists -d laoshi /var/backups/laoshi/<file>.dump
sudo systemctl start laoshi-api
```

恢复前先对当前库再做一次备份；恢复会覆盖数据，必须让用户确认。

## 健康检查失败

远端脚本在切换后依次检查：

1. `systemctl is-active laoshi-api`
2. `http://127.0.0.1:8787/healthz`
3. `http://127.0.0.1:8787/readyz`
4. 公网 `/api/v1/auth/me` 为 401 JSON
5. 公网未知 API 路径为 404 且不是主站 HTML

任一项失败会自动回滚软链并重启旧版本。回滚后：

```bash
readlink -f /opt/laoshi/backend/current
systemctl status laoshi-api --no-pager
journalctl -u laoshi-api -n 80 --no-pager
```

数据库迁移不会回退。若旧版本因 schema 变化无法启动，按上一节评估恢复备份，并立即告知用户。

## 公网 502 / 返回主站 HTML

- `502`：后端未监听或启动失败。查 `systemctl status laoshi-api`、`journalctl`、`curl http://127.0.0.1:8787/healthz`。
- 返回主站 HTML：Nginx 的 `location ^~ /api/` 可能被静态 `location /` 覆盖，或 `proxy_intercept_errors` 打开。检查 `/etc/nginx/conf.d/laoshi.conf` 与 `/etc/nginx/snippets/laoshi-api-location.conf`。
- 改 Nginx 后必须 `nginx -t` 成功再 reload；后端单包发布本身不需要改 Nginx。

## 版本与回滚

- 版本目录已存在时脚本拒绝覆盖；换新版本号。
- 手工回滚命令见 `server-layout.md`。
- 旧 release、数据库备份、`/home/package` 归档都不自动删除；清理前列清单并让用户确认。
- 至少保留当前版本和最近一个可回滚版本。

## 凭据

- 不要 `cat`、`sudo cat` 或回显 `/etc/laoshi/api.env`。
- 不要在任何命令、日志或回复里粘贴 `DATABASE_URL`。
- 脚本通过子 shell 加载环境文件；`ps` 中短暂出现连接串是服务器本机行为，不要把输出转贴到聊天。
