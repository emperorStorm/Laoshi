# 师说 Laoshi 静态门户与统一业务后端架构

> 方案 v2.0 · 状态：统一 Go 模块化单体 + PostgreSQL 方案已批准，首先接入岁岁时光；实现与首次上线仍需分别验证。
>
> 目标环境：阿里云 ECS，2 vCPU / 1.8 GiB 内存 / 40 GiB 系统盘，Alibaba Cloud Linux 3。
>
> 正式 Vue SSG 工程、内容校验、浏览器测试、Nginx 模板和部署归档均已创建；当前不连接或修改服务器。

## 1. 结论

师说 Laoshi 以“终身求知，以诚对话。”（Keep learning. Connect through dialogue.）为主宣言，秉持“学无止境，彼此为师；认真倾听，让理解发生。”，通过软件作品分享学习所得、连接交流。工程名为 `laoshi`，正式域名为 https://laoshi.chat；当前是作品展示与下载索引站，不提供站内聊天功能。作品列表、详情、分类筛选、关于页面和外部下载入口不需要服务端动态计算，采用纯静态站即可满足需求。项目目录名为 `Laoshi`，GitHub 仓库地址为 https://github.com/emperorStorm/Laoshi.git。

推荐方案：

| 层 | 选择 | 用途 |
| --- | --- | --- |
| 页面与路由 | Vue 3 + TypeScript | 作品首页、详情页、下载索引、关于页 |
| 构建与预渲染 | Vite + 静态站点生成（SSG） | 构建期生成 HTML、CSS、JS 与页面元数据 |
| 内容数据 | TypeScript 或 JSON 文件 | 作品资料随代码版本管理，构建时读取 |
| Web 服务 | Nginx | HTTPS、静态文件、缓存、安全响应头与 API 反向代理 |
| 业务后端 | Go 模块化单体 | 统一认证与业务模块，首期承接岁岁时光同步 |
| 服务端数据库 | PostgreSQL（PG） | 账号与同步数据持久化，按业务模块隔离 |
| 下载与大文件 | GitHub Releases | 安装包不占 ECS 磁盘 |
| 发布 | 本机构建 + 部署归档 + 版本目录 | 服务器不安装 Node、不运行构建 |

生产常驻栈为 Nginx + Go + PostgreSQL；Go 服务由 systemd 管理。主站继续采用静态门户，不引入 Java/Node 业务进程、MySQL、Redis、消息队列或 Docker。统一后端首先接入岁岁时光，采用本地优先、联网后同步的方式；旧服务仅供业务参考，无需兼容旧接口或协议。

## 2. 需求与约束

- 原型已展示 6 件作品，包含作品卡片、形态筛选、详情抽屉、下载入口和关于区；作品数据集中在 `docs/prototype/assets/js/products.js`。
- 目标 ECS 为 2 vCPU、1.8 GiB 内存、40 GiB 系统盘；已用约 8.7 GiB，另有 4 GiB swap。
- 服务器当前未安装 Nginx、Node、Java、MySQL、Redis、Docker 或 Git。
- Qoder 远程运行时约占 830 MB；生产服务运行期间不应让该远程运行时常驻。
- 安装包和源码继续放在 GitHub Releases 或作品自身仓库，不上传到 ECS。

### 当前不做

- 门户在线内容管理后台、门户登录与用户权限；业务客户端所需的统一认证由 Go API 提供。
- 下载统计、留言、评论、站内搜索服务。
- Redis、消息队列、独立文件上传服务。
- 运行时 SSR、多机高可用、容器编排。

API 与 PostgreSQL 属于已批准的首期范围；后续业务沿用 Go 模块化单体，按真实需求扩展模块，不为静态门户增加动态依赖。

## 3. 架构图

```text
门户浏览器 ── HTTPS laoshi.chat ──┐
岁岁时光客户端                  │
  本地数据优先读写              │
  联网后同步                    ▼
  └── HTTPS laoshi.chat ── Nginx（公网仅 80/443；SSH 受控开放）
                               ├── / → 静态 HTML/CSS/JS/图片
                               │        └── 下载链接 → GitHub Releases
                               └── /api/
                                   /api/v1/auth/...
                                   /api/v1/sui-time/sync/...
                                     │ 完整路径代理，不剥除 /api
                                     ▼
                               Go 模块化单体 127.0.0.1:8787
                                     │ 统一认证 / 岁岁时光模块
                                     ▼
                               本机 PostgreSQL（不对公网开放）

本地内容文件 ── Vite SSG 构建 ── 静态产物
Go 源码 ── Linux 二进制构建 ── 完整部署归档上传到 ECS
```

所有门户浏览器访问同一套静态页面，公开作品内容由版本化文件构建。岁岁时光的业务数据保存在客户端本地，联网并认证后通过统一 API 同步到 PostgreSQL；客户端不直接连接数据库，提醒继续在本地调度。

### 请求路径

1. 门户浏览器通过 HTTPS 请求页面，Nginx 返回预渲染 HTML 和带内容哈希的静态资源。
2. 门户 Vue 页面在浏览器中完成交互、分类筛选和详情导航，不请求业务 API。
3. 图片作为静态资源由 Nginx 返回；点击下载时直接跳转到作品的 GitHub Releases 或仓库页面。
4. 岁岁时光客户端访问 `https://laoshi.chat/api/v1/auth/...` 和 `https://laoshi.chat/api/v1/sui-time/sync/...`；Nginx 将完整 URI 转发到 `127.0.0.1:8787`，不剥除 `/api` 前缀，Go 完成认证、用户数据隔离与同步读写。
5. 修改作品内容后，在本机重新构建发布；仅静态内容变更不需要重启 Go 或迁移数据库。API 更新按第 8 节执行迁移、完整包切换与服务重启。

### 前后端边界

门户的 `content/`、`src/` 和 SSG 产物独立于业务后端；门户页面不承担账号或同步功能。`server/` 为统一 Go 模块化单体，集中提供认证和业务模块，以 PostgreSQL 保存服务端状态。岁岁时光保持本地优先，服务端负责认证后的跨设备同步，不接管客户端提醒调度。桌面端 `SUI_TIME_SYNC_BASE_URL` 与移动端 `VITE_SUI_TIME_SYNC_BASE_URL` 均填写 `https://laoshi.chat`，由客户端拼接完整 API 路由。

## 4. 当前工程结构

```text
Laoshi/
├── content/                   # 作品内容与类型
├── src/                       # Vue 页面、组件、路由与样式
├── public/brand/              # 品牌与作品图标
├── public/shots/              # 作品截图
├── scripts/                   # 内容/产物校验、预览与归档
├── tests/                     # Chrome 端到端验证
├── server/                    # Go 模块化单体、业务模块、SQL 迁移与 API 契约
├── deploy/
│   ├── nginx/                 # HTTP、TLS、安全头与 API 反向代理模板
│   ├── laoshi-api.service     # systemd 服务模板
│   ├── laoshi-api.env.example # API 环境变量示例，不含真实密钥
│   └── README.md              # 部署步骤，第 7 节为统一 API 部署约定
├── docs/                      # 原型与架构文档
├── vite.config.ts
└── package.json
```

原型归档继续保留在 `docs/prototype/`；正式站以 `content/works.ts` 为唯一作品数据源。构建期会校验 slug 唯一、必填字段、HTTPS 外链、颜色与本地图片是否存在。

## 5. 前端与页面生成

- Vue 3 + TypeScript + Vite；使用适配 Vue/Vite 的 SSG 方式生成首页、关于页和每个作品详情页。
- 已生成 `/`、`/about/`、六个 `/work/<slug>/` 目录页及 `/404.html`；Nginx 使用 `try_files $uri $uri/ =404`，详情刷新读取对应 `index.html`，缺失地址返回真实 HTTP 404，不回退首页。
- 分类筛选、菜单、详情预览等交互在浏览器本地完成；不为 6 件作品引入搜索服务。
- 作品资料保存在 `content/works.ts` 或 JSON，作为前端内容数据；不把用户私密信息、密钥或可变业务状态放入静态文件。
- 图片使用 WebP/AVIF 或优化后的 PNG/JPEG；懒加载非首屏图片，指定宽高以减少布局偏移。
- SSG 输出每页独立标题、描述、Open Graph 元数据，以及 `robots.txt`、`sitemap.xml`。
- 详情 URL 可直接打开、刷新和分享；继续兼容原型 `#product=<slug>` 深链接时，通过前端解析映射到正式详情路由。
- 字体使用系统字体，不依赖网络字体；首屏图片和 JS 体积作为构建验收项。

## 6. 静态资源与安全

### Nginx 职责

- 监听 80 并跳转 HTTPS；443 提供 TLS 1.2/1.3。
- 直接返回预渲染 HTML、JS、CSS、图片、图标与 SEO 文件。
- HTML 使用短缓存或不缓存；带内容哈希的 `/assets/` 长期缓存并标记 `immutable`。
- 开启 gzip；不需要 Brotli 模块或额外代理层。
- 设置 `X-Content-Type-Options: nosniff`、`Referrer-Policy` 等安全响应头；按正式资源清单配置 CSP。
- 关闭版本号暴露、限制访问日志保留期限。

### 门户与 API 安全边界

门户保持纯静态，没有门户管理员会话或用户写接口；外部链接使用明确的 HTTPS 地址，不提供由用户参数控制的任意跳转。业务 API 则必须处理认证、用户数据隔离、请求限流和输入校验；数据库使用专用账号与最小权限，PostgreSQL 与 Go 的 8787 端口均不向公网开放。

同源主站页面调用 `/api/` 无需跨域授权，其他 origin 的浏览器 H5 同步只允许 `CORS_ORIGINS` 中明确列出的页面 origin，不使用通配符；CORS 不能替代认证。Nginx 与 Go 的请求体上限保持一致（单次同步 8 MiB，纪念日照片合计文本 6 MiB），超大同步应缩小批次。环境密钥只保存在服务器受限权限文件中；日志不记录密码、令牌或完整私密同步数据。`/healthz` 与 `/readyz` 只供本机检查，不经公网代理；未知 API 路径返回 404，不回退门户页面。认证、隔离、限流和 CORS 均需作为上线前验收项。

## 7. 资源预算

以下为规划估算，实际部署后应以 ECS 监控数据校正。

### 内存

| 组件 | 常驻估算 | 说明 |
| --- | ---: | --- |
| 系统与云盾/监控 Agent | 200–280 MB | 基于现有服务器情况 |
| Nginx | 12–20 MB | 静态服务与 API 代理 |
| Go API | 40–100 MB | 低并发同步估算，请求体和并发会影响峰值 |
| PostgreSQL | 180–300 MB | 小连接池、轻负载估算，含共享缓冲区与连接开销 |
| Laoshi 页面 | 0 MB 服务端常驻 | HTML、JS、CSS 由 Nginx 读取；浏览器承担交互 |
| **预计合计** | **约 0.42–0.68 GiB** | 不含 page cache、备份与迁移等临时峰值 |
| **1.8 GiB 可用余量** | **约 1.1–1.4 GiB** | 需为系统缓存、同步与数据库峰值留余量 |

以上仅为容量规划估算，不代表实测或已满足生产负载。PostgreSQL 初始可采用 `shared_buffers=128MB`、`work_mem=4MB`，Go 连接池最多 5 个连接，后续依据监控调整。Vite/Node 与 Go 编译在开发机或 CI 执行，生产仅运行 Go 二进制，不要求安装 Go 编译工具链。Qoder 远程运行时约 830 MB，不应与生产栈长期并行。

### 磁盘

| 用途 | 规划 | 说明 |
| --- | ---: | --- |
| 系统与系统软件 | 11 GiB | 当前约已用 8.7 GiB，预留更新空间 |
| Nginx 与站点配置 | 0.5 GiB | 含证书、配置和日志空间 |
| 静态资源、Go 二进制与最近 3 个版本 | 1 GiB | 实际取决于截图数量与尺寸 |
| PostgreSQL 数据与 WAL | 3 GiB | 首期预算，非固定上限，需监控增长 |
| 站点与数据库本地备份暂存 | 2 GiB | 仅作异地备份前暂存，配置保留期 |
| 日志与临时空间 | 1 GiB | 配置轮转与上限 |
| **规划占用** | **约 18.5 GiB** | 包含当前系统用量及上述估算预留 |
| **40 GiB 余量** | **约 21.5 GiB** | 需按实际文件系统、数据与 WAL 增长校正 |

图片和安装包不要无限堆积在 ECS。安装包仍托管于 GitHub Releases；站点图片尽量纳入仓库或后续使用对象存储/CDN。同步照片会增加 PostgreSQL 与备份体积，需持续监控。备份应另存到 OSS 私有 Bucket 或另一台机器，不能只留在同一块系统盘。

### CPU 与带宽

- 对当前低频更新的展示站，2 vCPU 可作为起步配置；静态页面无需应用服务计算，Go 同步与 PG 查询的并发能力仍需实测。
- 首屏目标控制在 500 KB 左右，非首屏截图懒加载。
- 门户静态资源直接由 Nginx 服务；访问量或跨地域速度有明确问题后再评估 CDN。

## 8. 部署与回滚

### 目录布局

```text
/opt/laoshi/
├── releases/
│   ├── 20261022-01/
│   │   ├── web/
│   │   ├── nginx/
│   │   ├── api/               # 完整包：laoshi-api、db/migrations/、api/openapi.yaml
│   │   ├── deploy/            # 完整包：systemd 与环境变量模板
│   │   ├── README.md
│   │   └── release.json
│   └── 20261022-02/
└── current -> releases/20261022-02

/etc/nginx/conf.d/laoshi.conf
/etc/nginx/snippets/laoshi-security-headers.conf
/etc/nginx/certs/laoshi.chat/
/etc/laoshi/api.env
/etc/systemd/system/laoshi-api.service
```

仅在正式部署时创建这些服务器目录。TLS 模板使用 `laoshi.chat`，仍需准备该域名的有效证书与私钥（`fullchain.pem`、`privkey.pem`）并核对路径；证书私钥权限应限制为 root 与 Nginx 必要读取权限，私钥不进入仓库和发布包。已有 `/opt/suiworks` 的环境须按 `deploy/README.md` 手工迁移，确保 root、include 与 current 软链配套；本次修改不自动迁移线上。

### 发布流程

```text
本机 SITE_URL=https://laoshi.chat npm run package:deploy:api
  → 类型检查、内容校验、SSG、产物检查与 Linux Go 二进制构建
  → 生成 artifacts/laoshi-<版本>.tar.gz 完整包
  → 上传并解包到 releases/<版本>，核对元数据与完整性
  → 备份数据库，手工执行新版本 SQL 迁移
  → 原子切换 current 软链，重启 laoshi-api
  → 本机 healthz/readyz、主站页面、API 认证与客户端同步检查
```

`npm run package:deploy` 仅生成静态包，不包含 API；`npm run package:deploy:api` 才生成包含 `api/` 和服务模板的完整包。**一旦启用 API，current 的后续发布与回滚都必须指向完整包**，即使只修改门户内容，也不能切换到缺少 API 二进制的静态包。仅静态内容变更无需迁移或重启 Go；API 更新须先备份并迁移，再切换完整包并执行 `systemctl restart laoshi-api`，检查就绪状态与客户端同步。迁移不由 API 启动过程自动执行。

常规静态内容切换无需 reload Nginx。只有首次安装或修改 Nginx 配置时，才执行 `nginx -t`，成功后 reload。修改 API 环境文件需重启服务，修改 service 需先 daemon-reload 再重启。回滚 API 二进制前先确认数据库迁移兼容，切换兼容的完整包并重启服务，不自动执行数据库 down。保留最近若干发布目录，清理前先核实当前软链与旧哈希资源依赖。详细命令、顺序及约束以 `deploy/README.md` 第 7 节为准。

### 初次部署步骤

1. 确认 `laoshi.chat` 的 DNS、备案与 ECS 安全组，主站与 API 共用域名；公网仅开放受控 SSH 和 80/443，不开放 PostgreSQL 或 8787。
2. 创建 ECS 快照，安装 Nginx、PostgreSQL 与 rsync；准备独立 `laoshi` 数据库和专用数据库用户，配置小连接池与数据库资源参数，不安装 Java/Node 业务服务、MySQL 或 Redis。
3. 在本地或 CI 构建完整包，上传到新版本目录；创建无登录权限的 `laoshi` 系统用户，配置 `/etc/laoshi/api.env`，真实连接串与密钥仅保留在服务器受限权限文件中。
4. 先备份已有数据库，再执行新版本二进制的 `laoshi-api -migrate` 应用内嵌 goose 迁移；确认成功后原子切换 current，安装 `laoshi-api.service`，执行 daemon-reload 并启用 Go 服务，检查本机 `/healthz` 与 `/readyz`。
5. 准备主站 `laoshi.chat` 的有效证书，在同一 TLS 虚拟主机配置静态站与 `/api/` 反向代理；保留完整 `/api` 路径转发至 `127.0.0.1:8787`，API 错误不回退静态页面，`nginx -t` 成功后 reload，无需 API 子域名或额外证书。
6. 检查主站深链接、移动端布局、下载与 404；验证 API 未认证 JSON 401、未知路径 404、认证、数据隔离、限流、CORS 与岁岁时光本地优先同步，并演练兼容迁移的完整包回滚。
7. 配置证书续期、Nginx/Go/PG 日志保留、主站和 API 监控、数据库异地备份与恢复演练，以及磁盘/内存告警。

服务器目前没有被操作；以上是待实际部署时执行的步骤。

## 9. 备份、监控与维护

- 代码与作品内容：Git 仓库是主备份；保持远端仓库可用并避免把唯一副本留在 ECS。
- 服务器配置：备份 Nginx、systemd、TLS 续期配置和必要的部署元数据；证书私钥与 API 环境密钥用受控方式保管。
- 发布产物：静态站与 Go 二进制可从指定 Git commit 重新构建；若构建不可复现，再额外归档当前完整发布包。
- PostgreSQL：定期备份账号与同步数据，异地保存并演练恢复；执行迁移前额外备份，不能用客户端本地副本替代服务端备份。
- 监控：使用阿里云云监控关注内存、CPU、磁盘、主站与 API 可用性、Go 服务状态、PG 连接/WAL 增长和证书有效期；不安装 Prometheus/Grafana。本机检查 API 健康端点，公网监控不暴露内部端点。
- 日志：Nginx access/error log、Go journald 与 PG 日志设置总量和保留期，避免日志填满系统盘。
- 维护：更新 Nginx、PostgreSQL 与系统安全补丁；依赖更新在本机构建和检查后再发布，API 更新遵循备份、迁移和重启流程。

## 10. 后续扩展边界

| 新需求 | 最小扩展方向 | 何时再引入 |
| --- | --- | --- |
| 在线编辑作品内容 | 在统一 Go 后端增加管理模块与 PG 存储 | 手工改内容文件已不方便时；门户当前仍无后台 |
| 岁岁时光跨设备同步 | 统一认证 + 岁岁时光同步模块 + PG，客户端本地优先 | 已纳入首期，待实现与上线验证 |
| 其他作品的动态业务 | 在 Go 模块化单体中增加业务模块，按模块隔离数据 | 有明确业务需求时 |
| 下载统计 | 下载重定向服务或第三方分析 | 统计数据会影响决策时 |
| 大量图片或全球访问 | OSS + CDN | ECS 带宽或图片维护成为瓶颈时 |
| 全文检索 | 先评估构建期索引/浏览器搜索 | 内容规模显著增长后 |

统一后端采用 Go + PostgreSQL，不另行引入 Node + SQLite、MySQL 或托管 API 作为并行业务后端。客户端可按自身平台采用本地持久化方案，统一通过 HTTPS API 同步，不直接连接 PG；旧服务仅参考业务语义，无需兼容。Redis 与消息队列不在当前范围内，后续须由明确瓶颈与资源预算驱动评估。

## 11. 验收清单

- [x] 本地 HTTP 下首页、作品详情、关于页可直接打开，详情刷新不 404。
- [x] 六件作品的分类筛选、卡片和详情内容正确；手机与桌面布局通过自动化检查。
- [x] 页面标题、描述、Open Graph、robots.txt 与 sitemap.xml 可在构建期生成。
- [x] 下载、源码和在线原型链接均为经过内容校验的 HTTPS 地址。
- [x] 本地 Chrome 验证图片与脚本无资源 404、关键流程无控制台错误。
- [x] 带 hash 的静态资源配置长期缓存，HTML 配置为 `no-cache`。
- [ ] 正式域名 HTTPS、证书续期、日志轮转与云监控探活。
- [ ] ECS 首次发布与回滚演练。
- [ ] 生产常驻栈为 Nginx + Go + PostgreSQL，无 Java/Node 业务进程、MySQL 或 Redis。
- [ ] API 完整路径代理、认证、用户数据隔离、限流与 CORS 验证，数据库及 8787 不对公网开放。
- [ ] 岁岁时光离线本地读写、联网认证同步、跨设备数据一致性与本地提醒验证。
- [ ] 完整包发布、数据库备份/迁移/恢复、Go 重启和兼容迁移的回滚演练。

## 12. 实施阶段

| 阶段 | 内容 | 产物 |
| --- | --- | --- |
| P0 | 确认原型与静态内容字段 | 已完成 |
| P1 | 建立 Vue 3 + Vite + SSG 前端 | 已完成 |
| P2 | 迁移数据与素材，补充元数据与自动化验证 | 已完成 |
| P3 | 统一 Go 模块化单体、PG、认证与岁岁时光本地优先同步 | 方案已批准；代码与模板已有，待实现验收 |
| P4 | 配置主站/API 域名、Nginx、TLS、systemd、日志和监控 | 待首次上线 |
| P5 | 完整包 ECS 发布、数据库迁移、回滚和备份恢复演练 | 待首次上线 |

正式工程包含 `server/` 与统一 API 部署模板；主站继续静态，生产采用 Nginx + Go + PostgreSQL，不要求 Java/Node 业务进程、MySQL 或 Redis。服务器部署与完整包发布步骤以 `deploy/README.md` 第 7 节及归档内对应说明为准；本架构文档不代表后端验收或生产部署已经完成。
