# CI、Dependabot 与容器化设计

## 目标

为「遇健我」补齐可作为 `master` 分支保护依据的 GitHub Actions 门禁，并让依赖更新、覆盖率和 Go 服务容器具备明确、可重复的维护路径。

本次只验证仓库代码和本地可构造的产物。CI 不连接 PostgreSQL、OIDC、COS 或 EdgeOne 真实环境，也不运行需要审核快照的 `pnpm verify:edgeone`。

## 版本来源

- Node.js 版本范围由根 `package.json` 的 `engines.node` 声明。
- pnpm 版本由根 `package.json` 的 `packageManager` 声明。
- Go 版本由 `apps/server/go.mod` 声明。
- JavaScript 依赖使用各工作区 `package.json` 中的精确版本，并由锁文件固定完整依赖图。
- Docker 基础镜像版本只在 `Dockerfile` 中维护。
- GitHub Actions 引用版本必须写在工作流语法中，由 Dependabot 负责更新。

工作流通过 `node-version-file`、`packageManager` 和 `go-version-file` 读取这些来源，不再重复写 Node.js、pnpm 或 Go 版本。

## 代码验证工作流

`.github/workflows/code-check.yml` 在以下场景运行：

- 推送到 `master`。
- 向 `master` 提交 Pull Request。
- 手动触发。

工作流只授予 `contents: read`，并取消同一分支或 Pull Request 上已经过时的运行。门禁拆为独立任务，便于并行执行和定位失败：

1. 前端完整门禁：安装冻结依赖后运行 `pnpm verify`，检查生成类型没有漂移，并审计生产依赖。
2. Go 完整门禁：运行生成同步、全包测试、`go vet` 和 race 测试，并检查同步契约没有漂移。
3. TypeScript 覆盖率：验证 Schema、管理端和公开站的覆盖率阈值。
4. Go 覆盖率：生成全包原子覆盖率并验证语句阈值。
5. E2E：安装 Chromium，运行 Playwright 与 axe 测试，失败时保留报告。
6. 容器：构建 Go API 镜像、启动开发模式实例、检查 `/healthz`，随后扫描最终镜像的 HIGH 和 CRITICAL 漏洞。

## 覆盖率策略

TypeScript 使用 Vitest 的 V8 覆盖率提供器。三个工作区都执行覆盖率测试，业务源码纳入统计，生成类型、Nuxt 生成目录和测试辅助文件不计入统计。初始全局门槛为：

| 指标 | 门槛 |
| --- | ---: |
| 行 | 80% |
| 语句 | 80% |
| 函数 | 75% |
| 分支 | 70% |

Go 使用 `-covermode=atomic` 生成覆盖率 profile，按 profile 中的语句权重计算全仓覆盖率，门槛为 80%。覆盖率不足时命令返回非零退出码。阈值属于仓库策略，不写入 GitHub Actions 步骤。

## 容器边界

根 `Dockerfile` 只构建 `apps/server/cmd/api`。公开站仍由 EdgeOne Pages 发布，管理端也不进入 API 镜像。

镜像采用多阶段构建：Go 构建阶段下载模块并生成静态二进制，运行阶段使用非 root 的精简基础镜像。镜像默认监听 `0.0.0.0:8080`，不包含源码、测试、Node.js、pnpm 或开发素材。

容器健康检查只证明二进制可以启动并响应 `/healthz`。生产依赖严格校验、数据库迁移和真实服务商连通性仍需部署环境冒烟验证。

## Dependabot

`.github/dependabot.yml` 每周检查并分组更新：

- 根目录 pnpm 工作区。
- `apps/server` Go Modules。
- 根 Dockerfile 基础镜像。
- GitHub Actions。

更新使用中文 Conventional Commits 前缀，并限制同时打开的 Pull Request 数，避免无序刷屏。安全更新继续由 GitHub 按平台规则处理，不因常规分组而关闭。

## 验证边界

仓库内结构测试解析 YAML 并验证工作流、Dependabot、版本来源和 Docker 安全基线。GitHub Ubuntu runner 负责执行需要 CGO 和 C 编译器的 Go race 测试。完成实现后还要运行：

```bash
pnpm verify
pnpm audit --prod --audit-level high --registry=https://registry.npmjs.org
pnpm test:coverage
pnpm test:coverage:go
pnpm verify:go
pnpm --filter @yujian/web test:e2e
docker build --tag yujian-server:ci .
```

若本机缺少 Docker、Chromium 或完整 Go 覆盖率工具，需要明确报告对应验证缺口；GitHub Actions 配置不能替代真实运行结果。
