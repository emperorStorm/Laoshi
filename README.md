# 师说 Laoshi

> 终身求知，以诚对话。
>
> Keep learning. Connect through dialogue.

师说是一个围绕终身学习、分享与沟通的个人作品门户。我们相信“学无止境，彼此为师；认真倾听，让理解发生。”，希望通过软件作品分享探索所得，让使用体验与交流反馈成为继续学习的起点。门户介绍和索引课思库、智链中枢、岁岁时光、岁音、岁岁食光小厨房与岁影，提供作品详情、下载与源码入口。

工程名为 `laoshi`，正式域名为 https://laoshi.chat；域名中的 chat 承载以诚对话的理念，当前门户不提供站内聊天功能。项目目录名为 `Laoshi`，GitHub 仓库地址为 https://github.com/emperorStorm/Laoshi.git。

门户采用 **Vue 3 + TypeScript + Vite SSG**，页面在构建期预渲染，由 Nginx 静态托管，门户浏览不依赖业务 API。统一业务后端采用 **Go 模块化单体 + PostgreSQL**，首先接入岁岁时光的身份认证与跨设备同步；生产不需要 Node、Java、MySQL 或 Redis。原型归档保留在 `docs/prototype/`。

## 本地运行

需要 Node.js 22.12+（建议 Node 22 LTS）与 npm。若使用 nvm，先执行 `nvm use`；当前终端若仍使用 Node 16，需要切换后再安装依赖。

```bash
npm ci
npm run dev
```

浏览器访问终端输出的本地地址，默认 `http://127.0.0.1:5173`。

## 构建与预览

```bash
npm run build
npm run preview
```

生产产物位于 `dist/`，预览默认地址为 `http://127.0.0.1:4173`。预览服务器遵循静态目录规则，不存在的页面返回 HTTP 404。

构建会执行类型检查、内容校验、SSG 和资源完整性检查，生成首页、关于页、六个作品详情页、404、robots.txt 与 sitemap.xml。页面正文无需 JavaScript 即可阅读；分类筛选、移动菜单与快捷预览由 Vue 增强。

## 打包部署

```bash
npm run package:deploy
```

输出 `artifacts/laoshi-<版本>.tar.gz`，默认包含 `web/` 静态产物、Nginx 模板、中文部署说明及版本元数据；不会强制构建 Go。打包不会连接或修改阿里云服务器。岁岁时光后端需显式选择 `npm run package:deploy:api`，该命令在静态打包基础上跨编译 Linux API 并附带迁移 SQL、OpenAPI 和 systemd/env 示例。真实 env 文件及密钥不进入归档。

正式域名已确定为 https://laoshi.chat；上线前使用该地址重新构建并打包：

```bash
SITE_URL=https://laoshi.chat RELEASE_ID=20261022-001 npm run package:deploy
```

未设置 `SITE_URL` 仍可运行与打包，生成的 robots 禁止收录，sitemap 为空，不输出虚构 canonical。生产发布须设置 `SITE_URL=https://laoshi.chat` 并重新构建。

阿里云部署、Nginx 配置、原子版本切换和回滚步骤见 [deploy/README.md](deploy/README.md)。主站保持静态；API 使用独立 systemd 服务监听 `127.0.0.1:8787`，数据库仅监听回环地址，公网不开放数据库端口。

## 后端本地运行

需要 Go 1.23+ 和 PostgreSQL。在本机测试库执行二进制内嵌的 goose 迁移后启动服务；请先在当前终端设置自己的 `DATABASE_URL`，不要把真实密码写入版本库。

```bash
# 在仓库根目录执行，DATABASE_URL 指向自己的本机开发库。
(cd server && go run ./cmd/api -migrate)
(cd server && go run ./cmd/api)
```

默认监听 `127.0.0.1:8787`。公共认证为 `/api/v1/auth/register|login|logout|me`，岁岁时光同步为 `/api/v1/sui-time/sync/state|push|pull`。桌面和移动客户端分别设置 `SUI_TIME_SYNC_BASE_URL`、`VITE_SUI_TIME_SYNC_BASE_URL` 为服务 origin（例如 `http://127.0.0.1:8787`），不附加路径；生产值为 `https://laoshi.chat`，最终请求为 `https://laoshi.chat/api/v1/...`。桌面原生请求不受浏览器 CORS 约束，同源主站页面无需跨域授权，其他 H5 页面需在 `CORS_ORIGINS` 配置实际页面 origin。

客户端继续优先读写本地数据，Go 负责认证和跨设备同步；提醒由设备本地运行。服务端数据库不会在启动时自动迁移。生产前置、完整包和重启要求见部署说明第 7 节。

## 内容维护

- 修改 [content/works.ts](content/works.ts) 维护作品、版本、能力和下载链接；类型在 `content/types.ts`。
- 品牌图放 `public/brand/`，截图放 `public/shots/`，数据中使用根路径引用。
- `available` 提供下载入口，`source` 提供源码，`soon` 只展示“即将开放”。
- 图标、版本与下载状态沿用原型；未收录截图的作品保留真实空状态，安装包以作品自身的 Releases 为准。
- 更新后重新构建并发布，不用修改数据库。

## 常用命令

| 命令 | 用途 |
| --- | --- |
| `npm run dev` | Vite 开发服务器 |
| `npm run typecheck` | Vue 与 TypeScript 类型检查 |
| `npm run content:validate` | 作品内容、外链与素材校验 |
| `npm run build` | 完整静态构建与产物检查 |
| `npm run preview` | 生产产物预览与真实 404 |
| `npm run test:e2e` | 使用本机 Chrome 验证主要页面和交互（需先构建） |
| `npm run package:deploy` | 构建并生成静态站阿里云部署包 |
| `npm run package:deploy:api` | 显式构建并生成含岁岁时光 Go API 的部署包 |

## 工程结构

```text
content/          作品数据与类型
src/              Vue 页面、组件、路由与纸感样式
public/           品牌图标和截图
scripts/          内容/产物校验、预览与打包
tests/            浏览器端到端验证
deploy/           Nginx 模板、API systemd/env 示例与部署说明
docs/             原型归档、架构与设计文档
server/           Go 模块化单体 API、数据库迁移与 OpenAPI
```

## 视觉与说明

沿用原型的纸感编辑风：暖白纸底、墨色正文、朱砂红强调、宋体大标题、编号区块及细线排版。支持手机、平板、桌面布局，保留快捷预览抽屉，并增加可直接刷新与分享的 `/work/<slug>/` 详情页。

“师说 / Laoshi”是作品门户品牌，各作品保留自身名称。正式域名已确定，备案、TLS 证书、正式品牌素材和许可证仍需上线前准备或确认；本文不代表站点已部署成功。

架构与资源预算见 [docs/architecture.md](docs/architecture.md)，架构图见 [docs/architecture/overview.html](docs/architecture/overview.html)，原型说明见 [docs/README.md](docs/README.md)。
