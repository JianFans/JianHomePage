# 遇健我

「遇健我」是音乐人王子健的官方站点，目标域名为 `yujian.me`。公开首页采用 Nuxt 静态生成，内容来自版本化快照；后续管理端使用 Vue，内容与发布服务使用 Go。

## 当前能力

- 低饱和深色响应式首页：首屏、音乐、影像、现场、片段、音乐人与官方平台入口。
- `zh-CN` / `en` 双语：语言偏好保存在 `localStorage`，不修改 URL。
- 单实例音乐试听：封面播放、底部 Dock、进度控制和平台降级入口。
- 静态 SEO：canonical、Open Graph、JSON-LD、`robots.txt` 和 `sitemap.xml`。
- 自动验证：Vitest、Playwright、axe、类型检查、ESLint 和静态产物预算。
- Vue 管理端：会话令牌、快照编辑、审核、发布状态和回滚操作台。
- Go 内容服务：内容状态、素材上传、发布编排、标准库 HTTP API 和 EdgeOne 触发适配器。

## 环境要求

- Node.js 22
- pnpm 11.23.0
- Go 1.25.1（服务端单独 module）

## 本地开发

```bash
pnpm install
pnpm dev
```

开发服务器默认由 Nuxt 选择可用端口。完整验证命令：

```bash
pnpm verify
pnpm --filter @yujian/web test:e2e
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
cd apps/server
$env:GOCACHE = (Resolve-Path '.cache\\go-build').Path
$env:GOTMPDIR = (Resolve-Path '.cache\\tmp').Path
go test ./...
go vet ./...
```

## 目录

```text
apps/web/          Nuxt 公开站
apps/admin/        Nuxt/Vue 管理端（SPA，默认不索引）
apps/server/       Go 内容与发布服务
content/fixtures/  开发用版本化内容快照
packages/schema/   JSON Schema 与生成类型
scripts/           fixture 与静态产物工具
```

## 构建与部署

```bash
pnpm verify
```

静态输出位于 `apps/web/.output/public`。EdgeOne Pages 的构建参数、缓存策略和冒烟检查见 [apps/web/EDGEONE.md](apps/web/EDGEONE.md)。

静态构建默认读取 `content/fixtures/homepage.json`。发布工作流可设置 `CONTENT_SNAPSHOT_PATH`（相对仓库根目录或绝对路径）注入已经审核的不可变快照；构建会在启动阶段执行 canonical JSON Schema 和跨记录引用校验。

## 生产接入

Go 服务已装配 PostgreSQL、OIDC、S3 兼容对象存储和 EdgeOne HTTPS 触发器。账号侧仍需提供实际数据库、腾讯云 COS/EdgeOne 端点、OIDC 凭据与 `/api/*` 回源规则；完整变量和启动示例见 [apps/server/README.md](apps/server/README.md)。这些动态依赖不会改变公开首页的静态独立性。
