# Go 单程序部署实现计划

> 后续架构更新（2026-10-19）：本计划早期发布器从 COS 读取构建快照。现行实现已改为 PostgreSQL `publish_snapshots`；S3/COS 仅供媒体文件使用。请以 [`apps/server/README.md`](../../../apps/server/README.md) 和 [`2026-10-16-go-all-in-one-design.md`](../specs/2026-10-16-go-all-in-one-design.md) 为准。

**目标：** 服务器只运行一个 Go 二进制，包含管理页面、API、首页生成和腾讯云发布，无 Node 运行依赖。

**架构：** 编译时生成并嵌入 Vue 浏览器资源和封闭的服务端渲染程序；Go 内执行静态渲染并直接上传 Pages。复用现有 PostgreSQL 任务表与发布领域服务。

**技术栈：** Vue、Vite、Nuxt 管理端、Go、goja、PostgreSQL、COS、Pages HTTP API。

采用 TDD 逐项执行；每项先验证失败，再实现和验证。独立范围允许按并行技能分工，统一由主代理集成，禁止多个代理同时修改相同文件或 Go 依赖清单。

## 任务 1：Vue 静态生成和编译资源（已完成）

- [x] 创建 `apps/web/standalone/` 的服务端与浏览器入口，复用 `HomepageView`、`AudioDock`、现有样式和语言规则。
- [x] 创建 `scripts/build-server-assets.mjs`，分别生成管理页面、公开资源和无 Node 模块的渲染程序，写入 `apps/server/internal/webassets/bundle/`。
- [x] 新增真实渲染回归测试：变更标题、板块、媒体地址会改变生成 HTML；默认中文、内嵌 JSON 转义和 SEO 文件正确。
- [x] 在 `apps/server/internal/sitegen` 加入 `Generate(context.Context, []byte) (map[string][]byte, error)`，使用独立 goja 实例，拒绝超时、无效快照、过大输出；验证完整 HTML 无内容 API、无子进程。
- [x] 验证：目标 Go 测试、现有组件测试、公开站 Playwright；先跑最小真实 Vue 渲染，确认引擎兼容后再扩展。

## 任务 2：Go 内置管理页面

- [x] 创建 `internal/webassets` 包，嵌入 `all:bundle`，提供 `Admin() (fs.FS,error)`、`Public() (fs.FS,error)`、`Renderer() ([]byte,error)`；缺少实际资源返回错误。
- [x] 创建可组合的 HTTP handler，管理页面 `/` 与 `/preview`、哈希静态文件正确响应，API 路由交给已有 handler；拒绝穿越、目录列表，正确处理 HEAD、404 和缓存。
- [x] 管理端同源配置支持生产构建，开发模式仍可连接独立 Go 服务。
- [x] 验证：`go test ./internal/webassets ./internal/httpapi`，管理端测试和类型检查；真实二进制提供页面。

## 任务 3：Go Pages 客户端

- [x] 创建 `internal/providers/pages`：`Deployment{ID,ProjectID,Environment,Status}`，`Deploy(ctx, projectID string, files map[string][]byte) (Deployment,error)` 和 `Get(ctx, projectID,deploymentID string) (Deployment,error)`。
- [x] 按当前官方 SDK 协议实现项目检查、ZIP、COS 临时凭据上传、创建和查询，限制响应和上传地址，拒绝重定向泄露凭据。
- [x] 用 httptest 验证真实 JSON 请求与 ZIP 文件、云错误、未知状态、错误项目、凭据不泄露，写接口不重试。
- [x] 验证目标包测试与 race；不调用真实云写接口。

## 任务 4：Go 持久任务和恢复

- [x] 创建 `internal/publisher`，实现 `ports.BuildTrigger` 和定时 `Reconcile`，复用 `publisher_builds`。
- [x] 通过 PostgreSQL 独占连接锁序列化任务；持久化提交边界与冻结目标，保留未知结果。
- [x] 使用注入的 `Generate(context.Context, []byte)` 和 Pages 接口，读取原 COS 快照并核对原字节。
- [x] 加入管理员恢复入口，校验身份、目标、部署及备注，写入恢复记录。
- [x] 验证：状态机失败测试、真实数据库并发／重启／未知结果回归、race。

## 任务 5：接入单个服务和更新发行方式

- [x] 在 `cmd/api` 及 `internal/config` 接入内置任务与 Pages 配置，移除外部发布网关作为默认运行依赖。
- [x] 更新根 Dockerfile 为前端编译、Go 编译、仅 Go 运行三个阶段；增加下载二进制的构建命令。
- [x] 移除独立 Node 发布程序的部署入口与工作区依赖，同步锁文件、自动化测试和 CI，保留迁移历史。
- [x] 验证：生成资源后 `pnpm verify:go`、覆盖率、`pnpm verify`、前后台 E2E、全 Go race、无 Node 的进程集成。

## 任务 6：说明、自审与提交

- [x] 重写部署指南：一个程序、一个后台地址、一个配置文件；明确外部数据库、账号系统和腾讯云配置。
- [x] 同步 README、AGENTS 和模块文档，旧设计标记被本规格替代，不能保留两套当前部署说法。
- [x] 独立审查规格覆盖与代码正确性，修复发现的问题，原子签名提交。
- [x] 记录二进制实际运行验证、全部门禁和尚未完成的真实云集成；保留本地开发分支，未经要求不创建或合并 PR。

## 验证记录

- `pnpm build:server`、`pnpm verify`、`pnpm verify:go` 和 `pnpm test:coverage` 通过。Schema、公开站和管理端共 533 项单元测试，构建与自动化脚本 22 项测试通过。
- `pnpm test:coverage:go` 通过，合并后的 Go 语句覆盖率为 84.57%，门槛保持 80%。
- 全 Go 包 `go test -race ./... -count=1` 在真实 PostgreSQL 连接下通过，覆盖发布恢复、连接丢失和排他锁。
- Windows 和 Linux 的 Go 二进制均在清空 PATH 后成功导出首页。Go 直接提供管理页、预览页、静态资源和会话 API；浏览器资源全部返回 200，控制台无错误。
- 管理端针对真实 Go 静态服务运行桌面和手机共 122 个场景：全量运行通过 120 项；两项旧 API 地址断言改为同源后，定向重跑全部通过。保存与会话仍使用请求拦截验证契约，不代表真实 OIDC 登录或云服务验证。
- `pnpm test:embedded` 通过 12 项真实浏览器测试，使用清空 PATH 后的 Go 导出首页，验证 hydration、SEO、试听、语言持久化、响应式与无障碍。
- 最终审查补齐完整二进制身份、管理端动态资源清单、OpenAPI 同源地址与认证引用校验。没有降低依赖版本、覆盖率或安全校验要求。
- Docker 配置契约通过；镜像实际构建与运行尚未验证。Pages、COS、OIDC、域名和 EdgeOne 仍需真实环境联调，本计划不宣称已上线。
