# 遇健我

「遇健我」是面向音乐人王子健的粉丝站点，网址为 `yujian.me`。访客看到的是提前生成的网页；管理员在后台编辑、审核和发布，服务器自动生成新版网页并上传到腾讯云 EdgeOne Pages。

**准备部署？从 [部署操作指南](docs/DEPLOYMENT.md) 开始。** 它按实际操作顺序说明要开通什么、在哪填配置、怎么启动和首次发布。

项目坚持静态优先、契约优先和服务商可替换。公开站不依赖运行时内容 API，第三方音乐与社交平台只作为跳转目标，不作为浏览器端数据源。

## 当前范围

已实现：

- 低饱和深色响应式首页：首屏、音乐、影像、现场、片段、音乐人与平台入口。
- `zh-CN` / `en` 双语：优先读取 `localStorage`，其次读取浏览器语言，无法识别时回退到默认语言并显示非阻断提示。
- 单实例音乐试听：封面播放入口、底部 Dock、进度控制、上一首/下一首和平台降级入口。
- 静态 SEO：canonical、Open Graph、JSON-LD、`robots.txt` 和 `sitemap.xml`。
- Nuxt 管理端：OIDC 托管登录、左侧内容导航、只读历史版本与服务端版本复制、统一保存、动画 Toast、音乐作品与曲目、影像、现场、片段、首页编排、隔离预览、素材管理及审核发布操作台。
- Go 内容服务：PostgreSQL、OIDC、S3 兼容对象存储、EdgeOne 构建触发与后台任务对账。
- Pages 自动发布进程：下载并校验审核快照、静态构建、官方 SDK 上传、持久化幂等和异常恢复。
- 验证体系：ESLint、TypeScript、Vitest 覆盖率、Playwright、axe、Go test、Go vet、Go 覆盖率、容器检查、静态产物校验和独立的生产依赖审计。

当前不包含论坛、首页登录入口、统一身份中心或第三方平台内容自动抓取。后续子站点可以复用统一身份认证，但不应破坏公开首页的静态独立性。

## 系统结构

| 模块 | 技术 | 职责 |
| --- | --- | --- |
| `apps/web` | Nuxt、Vue、Vue I18n | 从构建快照生成公开静态站点 |
| `apps/admin` | Nuxt、Vue | 内容编辑、审核与发布操作台 |
| `apps/server` | Go | 内容版本、素材、鉴权、审计和发布编排 |
| `apps/publisher` | Node.js | 受控静态构建、腾讯云 Pages 上传和持久化部署对账 |
| `packages/schema` | JSON Schema、TypeScript | 内容快照的唯一规范及生成类型 |
| `content/fixtures` | JSON | 仅供本地开发和测试的样例快照 |
| `scripts` | Node.js | fixture 生成、EdgeOne 门禁和静态产物验证 |

内容发布链路如下：

```text
管理端
  -> Go API 创建和审核内容版本
  -> canonical JSON 快照与素材完整性校验
  -> PostgreSQL 冻结发布目标
  -> S3/COS 保存不可变快照
  -> 发布进程读取 COS 快照并注入 CONTENT_SNAPSHOT_PATH
  -> Nuxt 生成纯静态产物
  -> 官方 Pages SDK 上传 Production 部署
  -> EdgeOne Pages 分发 yujian.me
```

## 关键约束

- `packages/schema/schema/content-snapshot.schema.json` 是内容契约的唯一来源。
- `generated.ts`、`generated.go` 等生成文件不得手工修改。
- 公开页面必须从构建时快照渲染，不能在浏览器中调用运行时内容 API。
- 正式构建必须使用已审核快照，不能直接使用 `content/fixtures`。
- 素材地址使用稳定 HTTPS 地址或仓库内 `/media/*` 路径，不能保存短期签名 URL。生产地址在素材创建时持久化，后续发布不根据当前服务商配置重算。
- QQ 音乐、网易云音乐、Bilibili、微博、抖音等第三方站点只作为外链目标。
- PostgreSQL、OIDC、S3/COS 与 EdgeOne 通过明确接口和配置接入，领域层不依赖服务商 SDK 类型。

## 环境要求

| 工具 | 版本 |
| --- | --- |
| Node.js | >=22.22.3，发布镜像使用 25，CI 按清单选择匹配的最新版本 |
| pnpm | 12.11.2 |
| Go | 1.25.1 |
| Chrome | 用于 Playwright E2E |

## 快速开始

pnpm 版本以根目录 `packageManager` 为准。旧版 Corepack 可能仍查找 `bin/pnpm.cjs`，无法启动 pnpm 12；遇到该错误时升级至支持 pnpm 12 的 Corepack，或使用独立安装的 pnpm 12。发布镜像在构建时通过 npm 安装清单指定的 pnpm，不依赖基础镜像中的 Corepack。

安装依赖并启动公开站：

```bash
pnpm install --frozen-lockfile
pnpm dev
```

启动管理端：

```bash
pnpm --filter @yujian/admin dev
```

开发模式启动 Go 服务：

```bash
cd apps/server
go run ./cmd/api
```

开发模式使用内存仓储和本地素材适配器。服务入口为 `http://127.0.0.1:8080`，健康检查为 `GET /healthz`。生产环境不会自动降级到开发实现。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `pnpm dev` | 启动公开站开发服务器 |
| `pnpm --filter @yujian/admin dev` | 启动管理端开发服务器 |
| `pnpm lint` | 运行 ESLint |
| `pnpm typecheck` | 检查所有 TypeScript/Nuxt 项目 |
| `pnpm test` | 运行 Schema、管理端、公开站和脚本测试 |
| `pnpm test:coverage` | 验证 TypeScript 工作区与发布进程的覆盖率门槛 |
| `pnpm test:coverage:go` | 验证 Go 全包语句覆盖率不低于 80% |
| `pnpm test:automation` | 验证 GitHub Actions、Dependabot 和 Docker 配置契约 |
| `pnpm schema:generate` | 根据 canonical Schema 生成 TypeScript 类型 |
| `pnpm generate` | 生成 Schema 类型及所有静态站点 |
| `pnpm verify` | 运行前端完整门禁并检查静态产物 |
| `pnpm verify:go` | 检查 gofmt，运行 Go generate、全包测试和 Go vet |
| `pnpm --filter @yujian/web test:e2e` | 运行 Chrome E2E 与无障碍测试 |
| `pnpm --filter @yujian/admin test:e2e` | 运行管理端桌面与手机尺寸的音乐、影像、现场、片段、首页编辑、视觉预览及保存契约测试 |
| `pnpm verify:edgeone` | 使用正式内容快照执行 EdgeOne 发布门禁 |
| `pnpm --filter @yujian/publisher test:integration` | 使用独立 PostgreSQL 验证发布网关、任务恢复与进程启动 |
| `pnpm fixture:images` | 重新生成开发图片 fixture |
| `pnpm fixture:audio` | 重新生成开发音频 fixture |

管理端编辑器直接复用 `packages/schema` 的 canonical 校验规则。界面诊断使用 250 ms 防抖，保存和导出始终重新校验当前文本。只有完整通过 Schema 与跨记录语义校验的快照才能保存或导出；本地 JSON 导入不会上传文件，也不会把会话 Token、API 地址或发布状态写入导出内容。

首页视觉预览在独立 iframe 中复用公开站展示组件，支持桌面／手机视口与中英文。打开预览会先校验并应用当前模块的临时表单，失败时保留输入并提示错误；JSON 变化立即撤下旧画面。预览不保存或发布内容，不上传未发布快照，也不传递 Token 或编辑连接；正式公开站仍只读取构建时快照。

管理端通过 OIDC 提供器托管的登录页认证；网站不接收或储存用户密码。会话使用 HttpOnly Cookie、CSRF 校验与服务端角色控制。独立导航只显示当前模块；顶部集中提供版本选择、预览与保存。历史版本保持只读，可从任意版本请求服务端克隆成新草稿。详细配置和 API 行为见 [apps/server/README.md](apps/server/README.md) 与 [apps/admin/README.md](apps/admin/README.md)。

影像工作台维护视频标题、海报、时长、精选、平台链接和可选视频素材；现场工作台维护双语标题、城市、场馆、带时区时间、状态、详情链接和可选海报；片段工作台维护图片／GIF 素材、可选双语说明及内部或外部跳转。新记录先应用到快照，再通过首页工作台加入对应板块；不会自动改变首页排序和展示上限。模块共用未保存保护。素材库首次打开后保持挂载，切换模块不会丢弃上传信息；上传期间禁止退出，尚未上传的素材信息会在退出时明确询问。

素材工作台支持 WebP、GIF、MP3、WAV 和 MP4，使用分块 SHA-256 与对象存储签名直传。上传完成后仍需把 `ready` 素材插入快照、保存草稿并走完审核发布流程。详细限制见 [apps/admin/README.md](apps/admin/README.md)。

## 内容快照

本地构建默认读取 `content/fixtures/homepage.json`。该文件只用于开发和自动测试。

正式 EdgeOne 构建必须设置 `CONTENT_SNAPSHOT_PATH`，指向发布工作流下载或生成的已审核快照：

```powershell
$env:CONTENT_SNAPSHOT_PATH = "D:\release\homepage.json"
pnpm verify:edgeone
```

门禁会验证：

- JSON Schema、HTTPS URL、素材 MIME 和跨记录引用。
- 首页编排、板块启用状态、展示上限和时间窗口。
- 内部跳转是否映射到实际生成的静态锚点。
- 本地素材的文件大小、MIME 和 SHA-256。
- 静态产物不包含运行时内容 API 标记。
- `index.html`、`robots.txt`、`sitemap.xml` 和首屏脚本预算。

## 测试与发布门禁

提交前至少运行与改动范围对应的窄测试。准备合并或上线时运行：

```bash
pnpm verify
pnpm test:coverage
pnpm test:coverage:go
pnpm verify:go
pnpm --filter @yujian/web test:e2e
pnpm --filter @yujian/admin test:e2e
```

TypeScript 门槛为行和语句 80%、函数 75%、分支 70%；Go 语句覆盖率门槛为 80%。GitHub Actions 在 `master` 的 push、Pull Request 和手动触发时并行执行前端、Go、覆盖率、E2E 与容器门禁。Go 门禁还会执行 race 测试，前端和 Go 生成命令执行后必须保持已跟踪契约文件不变。失败的 Playwright 报告和覆盖率产物会保留为工作流制品。

Dependabot 每周检查 pnpm 工作区、Go Modules、Docker 基础镜像和 GitHub Actions。Node.js、pnpm、Go 与应用依赖版本由 `package.json`、`go.mod` 和锁文件管理，Docker 基础镜像版本由 `Dockerfile` 管理；工作流不重复声明这些版本。GitHub Actions 引用按工作流语法保留在 YAML 中，并由 Dependabot 更新。前端门禁还会按根锁文件审计生产依赖的高危和严重漏洞。

浏览器单元测试使用 Vitest 5 与 Nuxt 测试环境，确保 Node.js 25 及以后内置的 Web Storage 不覆盖模拟浏览器的 `localStorage` 和 `sessionStorage`。测试无需配置 Node.js 的本地存储文件，语言偏好仍使用浏览器 Storage 并在测试间清理。

生产依赖审计：

```bash
pnpm audit --prod --registry=https://registry.npmjs.org
```

测试通过只表示代码和本地产物达到发布候选标准。未完成腾讯云 COS、EdgeOne、OIDC、数据库迁移和生产域名冒烟前，不应宣称已经上线。

PostgreSQL 集成测试需要显式设置 `YUJIAN_TEST_POSTGRES_URL`，否则会跳过。账号权限和隔离 Schema 的执行方式见 [服务端集成验证](apps/server/README.md#postgresql-集成验证)。`apps/web` 的 Playwright 门禁覆盖公开站；管理端 Playwright 覆盖音乐、影像、现场、片段与宣传位编辑、首页编排、真实 iframe 预览、多表单冲突、导出再导入与保存 API 契约。素材上传目前仍由 Go HTTP 集成测试与 Vue/API 单元测试分别覆盖，尚无串联真实身份、Go 服务和对象存储的完整浏览器上传 E2E。

## 生产上线

按 [部署操作指南](docs/DEPLOYMENT.md) 从头操作。默认方案是：公开首页放 EdgeOne Pages，管理页面、Go 后端和发布程序放自己的服务器，文件放 COS，数据放 PostgreSQL。

第一次需要配置云服务和账号登录。配置完成后，日常更新只需在后台编辑、保存、审核、点发布；发布程序通过腾讯云官方接口自动上传网页。

管理页面需要单独打包，Go 镜像不包含它。代码升级和内容更新也是两件事：换程序需要重新打包部署，改内容直接在后台发布。已有数据库的升级注意事项见 [服务端安全与运维](apps/server/README.md#安全与运维)，发布中断后的处理见 [发布恢复步骤](apps/publisher/README.md#超时恢复与升级)。

## 文档索引

- [部署操作指南：准备、配置、启动和首次发布](docs/DEPLOYMENT.md)
- [公开站 EdgeOne 部署基线](apps/web/EDGEONE.md)
- [Go 服务配置、API 与运维](apps/server/README.md)
- [Pages 自动发布进程部署与恢复](apps/publisher/README.md)
- [管理端开发说明](apps/admin/README.md)
- [生产上线加固计划](docs/superpowers/plans/2026-08-30-production-hardening.md)
- [仓库代理工作指南](AGENTS.md)

参与开发或让编码代理修改仓库前，请先阅读 [AGENTS.md](AGENTS.md)。
