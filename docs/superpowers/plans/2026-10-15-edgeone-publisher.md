# EdgeOne Pages 自动发布实现计划

> **面向 AI 代理的工作者：** 使用 `subagent-driven-development` 完成隔离的构建适配器任务，再内联整合持久化网关；各阶段执行规格和质量审查。

**目标：** 接通现有发布按钮与腾讯云 Pages Direct Upload。

**架构：** Go 保留发布领域编排。独立 Node 发布进程通过稳定 HTTP 契约接收任务，PostgreSQL 保存幂等记录，受数据库锁保护的 worker 构建并使用官方 SDK 上传。

**技术栈：** Node 22、pnpm workspace、PostgreSQL、AWS S3 兼容读取、官方 EdgeOne SDK。

## 任务 1：构建与 Pages 适配器（已完成）

文件：`apps/publisher/src/builder.mjs`、`pages.mjs`、对应 `test/*.test.mjs`；依赖在 `apps/publisher/package.json` 固定。

- [x] 先添加并运行失败测试：`node --test apps/publisher/test/builder.test.mjs apps/publisher/test/pages.test.mjs`。
- [x] 实现 `createBuilder({ sourceRoot, workRoot, readSnapshot, run, timeoutMs })`，返回接收任务的异步构建函数。测试校验 checksum、releaseId、大小、环境隔离、旧产物清理、失败阻断上传。
- [x] 实现 `createPagesProvider(client)`，暴露 `deploy(directory, projectId)` 与 `get(projectId, deploymentId)`；固定 Production，不启用写重试，未知状态保持活动且提示。
- [x] 重新运行窄测试、ESLint，完成规格和质量审查。

## 任务 2：持久化发布网关（已完成）

文件：`apps/publisher/src/{config,store,worker,http,main}.mjs`、`apps/publisher/test/*.test.mjs`、`apps/server/internal/store/postgres/migrations/0008_publisher_builds.sql`。

- [x] 先写失败测试，验证 `POST /builds`、`GET /builds/:id`、`POST /builds/:id/reconcile`，Bearer 鉴权和无效请求拒绝。
- [x] PostgreSQL 幂等插入、配置冻结、恢复信息和全局 advisory lock；集成测试在独立临时 schema 内应用真实迁移。
- [x] worker 在上传前保存 `submitting`，重启后不重复上传；已知部署只查询；构建前失败允许终止，提交结果不明保持活动。
- [x] 安装依赖并运行 `pnpm --filter @yujian/publisher test`、覆盖率、`pnpm verify:go`。

## 任务 3：交付与全链路验证（已完成）

文件：`apps/publisher/{Dockerfile,Dockerfile.dockerignore,README.md}`、`.github/workflows/code-check.yml`、`scripts/automation-config.test.mjs`、根 README、`apps/web/EDGEONE.md`、`apps/server/README.md`。

- [x] 容器提供固定源代码、Node/pnpm 和非 root 构建目录；秘密只通过运行时注入。
- [x] CI 增加 PostgreSQL 发布集成验证和容器构建，依赖版本继续来自 manifest。
- [x] 同步部署、恢复、权限、升级顺序和真实云验收说明。
- [x] 用仓库外快照运行真实静态构建，确认发布适配器能读取该产物。
- [x] 运行 `pnpm verify`、`pnpm verify:go`、`pnpm test:automation`、发布覆盖率与集成测试；本地 Docker 不可用，容器验证交由已配置的 CI 执行。
- [x] 独立审查，修复有效问题，检查 diff 并按发布实现、CI 配置和部署文档原子化提交；不推送或部署真实云账号。

## 审查与验证记录

- 独立规格审查与集成质量审查已完成；已修复子进程异常退出清理、只读文件系统期限、Linux 镜像的 pnpm 定位以及版本 ID 长度契约问题，均有回归测试。
- 发布进程单测共 81 项，80 通过、0 失败；Windows 跳过 1 项 POSIX 进程组测试，Linux CI 执行该用例。行覆盖率为 97.03%，函数覆盖率为 92.39%。
- `pnpm verify`、`pnpm verify:go`、自动化契约及生产依赖审计通过。最终前端门禁使用 `VITEST_MAX_WORKERS=2`，处理默认高并发引发的 Nuxt 初始化超时；没有跳过用例、放宽断言或增加超时阈值。
- PostgreSQL 集成测试通过，覆盖重复迁移、并发幂等、活动槽位、重启恢复、锁互斥、HTTP 到 worker 交接及真实进程启动；Go 完整验证包含真实 PostgreSQL 迁移与仓储测试。
- 最终真实静态构建使用固定依赖 manifest 指定的 pnpm CLI、隔离源码、仓库外临时快照与测试媒体 URL，仅验证构建和产物清理，没有云端上传或内容审核结论。
- Docker 镜像构建、非 root 断网构建以及真实 Pages/COS/TLS 验收仍需在可用的容器或云环境执行，不能从本地单测推断通过。
