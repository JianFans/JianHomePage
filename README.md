# 遇健我

「遇健我」是面向音乐人王子健的粉丝站点，目标域名为 `yujian.me`。公开首页采用 Nuxt 静态生成，内容来自经过审核的不可变快照；管理端与 Go 服务负责内容版本、素材和发布编排。

项目坚持静态优先、契约优先和服务商可替换。公开站不依赖运行时内容 API，第三方音乐与社交平台只作为跳转目标，不作为浏览器端数据源。

## 当前范围

已实现：

- 低饱和深色响应式首页：首屏、音乐、影像、现场、片段、音乐人与平台入口。
- `zh-CN` / `en` 双语：优先读取 `localStorage`，其次读取浏览器语言，无法识别时回退到默认语言并显示非阻断提示。
- 单实例音乐试听：封面播放入口、底部 Dock、进度控制、上一首/下一首和平台降级入口。
- 静态 SEO：canonical、Open Graph、JSON-LD、`robots.txt` 和 `sitemap.xml`。
- Nuxt 管理端：快照编辑、审核、发布状态和回滚操作台。
- Go 内容服务：PostgreSQL、OIDC、S3 兼容对象存储、EdgeOne 构建触发与后台任务对账。
- 验证体系：ESLint、TypeScript、Vitest 覆盖率、Playwright、axe、Go test、Go vet、Go 覆盖率、容器检查、静态产物校验和独立的生产依赖审计。

当前不包含论坛、首页登录入口、统一身份中心或第三方平台内容自动抓取。后续子站点可以复用统一身份认证，但不应破坏公开首页的静态独立性。

## 系统结构

| 模块 | 技术 | 职责 |
| --- | --- | --- |
| `apps/web` | Nuxt、Vue、Vue I18n | 从构建快照生成公开静态站点 |
| `apps/admin` | Nuxt、Vue | 内容编辑、审核与发布操作台 |
| `apps/server` | Go | 内容版本、素材、鉴权、审计和发布编排 |
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
  -> EdgeOne 构建读取 CONTENT_SNAPSHOT_PATH
  -> Nuxt 生成纯静态产物
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
| Node.js | 22 |
| pnpm | 11.23.0 |
| Go | 1.25.1 |
| Chrome | 用于 Playwright E2E |

## 快速开始

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
| `pnpm test:coverage` | 验证三个 TypeScript 工作区的覆盖率门槛 |
| `pnpm test:coverage:go` | 验证 Go 全包语句覆盖率不低于 80% |
| `pnpm test:automation` | 验证 GitHub Actions、Dependabot 和 Docker 配置契约 |
| `pnpm schema:generate` | 根据 canonical Schema 生成 TypeScript 类型 |
| `pnpm generate` | 生成 Schema 类型及所有静态站点 |
| `pnpm verify` | 运行前端完整门禁并检查静态产物 |
| `pnpm verify:go` | 检查 gofmt，运行 Go generate、全包测试和 Go vet |
| `pnpm --filter @yujian/web test:e2e` | 运行 Chrome E2E 与无障碍测试 |
| `pnpm verify:edgeone` | 使用正式内容快照执行 EdgeOne 发布门禁 |
| `pnpm fixture:images` | 重新生成开发图片 fixture |
| `pnpm fixture:audio` | 重新生成开发音频 fixture |

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
```

TypeScript 门槛为行和语句 80%、函数 75%、分支 70%；Go 语句覆盖率门槛为 80%。GitHub Actions 在 `master` 的 push、Pull Request 和手动触发时并行执行前端、Go、覆盖率、E2E 与容器门禁。Go 门禁还会执行 race 测试，前端和 Go 生成命令执行后必须保持已跟踪契约文件不变。失败的 Playwright 报告和覆盖率产物会保留为工作流制品。

Dependabot 每周检查 pnpm 工作区、Go Modules、Docker 基础镜像和 GitHub Actions。Node.js、pnpm、Go 与应用依赖版本由 `package.json`、`go.mod` 和锁文件管理，Docker 基础镜像版本由 `Dockerfile` 管理；工作流不重复声明这些版本。GitHub Actions 引用按工作流语法保留在 YAML 中，并由 Dependabot 更新。前端门禁还会按根锁文件审计生产依赖的高危和严重漏洞。

生产依赖审计：

```bash
pnpm audit --prod --registry=https://registry.npmjs.org
```

测试通过只表示代码和本地产物达到发布候选标准。未完成腾讯云 COS、EdgeOne、OIDC、数据库迁移和生产域名冒烟前，不应宣称已经上线。

## 生产上线

### 公开静态站

EdgeOne Pages 使用仓库根目录的 [edgeone.json](edgeone.json)：

- 安装命令：`pnpm install --frozen-lockfile`
- 构建命令：`pnpm verify:edgeone`
- 输出目录：`apps/web/.output/public`
- Node.js：22

发布工作流必须提供 `CONTENT_SNAPSHOT_PATH`。缓存策略、响应头和上线后检查见 [apps/web/EDGEONE.md](apps/web/EDGEONE.md)。

### Go 服务

生产环境需要 PostgreSQL、OIDC、S3/COS、稳定媒体域名、EdgeOne 构建接口和管理端 HTTPS Origin。完整变量、启动示例、API 行为和迁移说明见 [apps/server/README.md](apps/server/README.md)。

根 Dockerfile 只封装 Go API。构建并检查开发模式健康端点：

```bash
docker build --tag yujian-server:local .
docker run --detach --rm --name yujian-server-local --publish 127.0.0.1:8080:8080 yujian-server:local
curl --fail http://127.0.0.1:8080/healthz
docker rm --force yujian-server-local
```

生产运行时通过平台密钥和环境变量注入真实依赖，不把 `.env`、凭据或公开站静态产物写入镜像。

首次部署包含 `0003_publish_target_freeze.sql` 的版本前，必须确认没有 `pending` 或 `building` 发布任务。升级已有数据库时，`0004_asset_source_url.sql` 会使用当前 `MEDIA_PUBLIC_BASE_URL` 回填旧素材；迁移完成前不得切换公开媒体域名。为兼容滚动升级和旧二进制回滚，本版本暂不把 `source_url` 收紧为 `NOT NULL`；旧实例全部退出并度过回滚窗口后，再由后续迁移完成约束收紧。不要手工修改 checksum，也不要跳过迁移。

### 上线检查清单

- 代码已推送到 EdgeOne 可访问的 Git 远端。
- `master` 工作区干净，提交历史符合原子化约束。
- 前端、Go、覆盖率、E2E、容器扫描和生产依赖审计通过。
- 已准备非 fixture 的审核快照并配置 `CONTENT_SNAPSHOT_PATH`。
- PostgreSQL、OIDC、COS、媒体域名和 EdgeOne 参数已配置。
- `MEDIA_PUBLIC_BASE_URL` 指向长期稳定的 EdgeOne 媒体域名；迁移对象存储源站时保持该公开域名不变。
- COS 直传 CORS 允许管理端 Origin、`PUT`、`HEAD`、`Content-Type` 和 `X-Amz-Checksum-Sha256`。
- EdgeOne 已配置 `yujian.me`、TLS、`/api/*` 回源和媒体域名。
- 数据库迁移、`GET /healthz`、首页、素材、发布和回滚已在真实环境冒烟。

## 文档索引

- [公开站 EdgeOne 部署基线](apps/web/EDGEONE.md)
- [Go 服务配置、API 与运维](apps/server/README.md)
- [管理端开发说明](apps/admin/README.md)
- [生产上线加固计划](docs/superpowers/plans/2026-08-30-production-hardening.md)
- [仓库代理工作指南](AGENTS.md)

参与开发或让编码代理修改仓库前，请先阅读 [AGENTS.md](AGENTS.md)。
