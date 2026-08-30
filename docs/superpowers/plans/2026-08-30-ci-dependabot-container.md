# CI、Dependabot 与容器化实施计划

> **执行要求：** 按 TDD 顺序逐项实现；每个任务先证明缺失行为，再补最小实现并运行窄测试。

**目标：** 为仓库增加代码验证、覆盖率、Go API 容器和 Dependabot，并形成可用于 `master` 分支保护的稳定门禁。

**架构：** GitHub Actions 只编排仓库已有命令，Node.js、pnpm 和 Go 版本分别由 `package.json` 与 `go.mod` 提供。覆盖率策略由 Vitest 配置和仓库脚本管理；Docker 只封装 Go API。

**技术栈：** GitHub Actions、Dependabot、pnpm、Vitest V8 coverage、Go coverage profile、Docker、Trivy、Playwright。

---

## 任务 1：建立自动化配置契约测试

**文件：**

- 新建：`scripts/automation-config.test.mjs`
- 修改：`package.json`
- 修改：`pnpm-lock.yaml`

1. 添加 YAML 解析依赖和自动化配置测试。
2. 断言版本只从清单读取、工作流包含完整门禁、Dependabot 覆盖四类生态、Docker 使用非 root 运行时。
3. 运行窄测试，确认因配置缺失而失败。

## 任务 2：实现覆盖率命令

**文件：**

- 新建：`scripts/go-coverage.mjs`
- 新建：`scripts/go-coverage.test.mjs`
- 新建：`packages/schema/vitest.config.ts`
- 修改：`apps/web/vitest.config.ts`
- 修改：`apps/admin/vitest.config.ts`
- 修改：三个工作区的 `package.json`
- 修改：根 `package.json`
- 修改：`pnpm-lock.yaml`

1. 先为 Go profile 解析和阈值失败行为添加测试。
2. 实现跨平台 Go 覆盖率运行器和 80% 语句门槛。
3. 为三个 Vitest 工作区启用 V8 覆盖率与全局门槛。
4. 运行覆盖率命令；覆盖率不足时补关键业务测试，不降低已批准门槛。

## 任务 3：实现 Go API 容器

**文件：**

- 新建：`Dockerfile`
- 新建：`.dockerignore`
- 修改：`README.md`
- 修改：`apps/server/README.md`

1. 添加多阶段构建和非 root 运行镜像。
2. 验证镜像只包含 Go API 所需产物。
3. 构建并运行镜像，检查 `GET /healthz`。
4. 记录容器构建、运行和生产配置边界。

## 任务 4：实现 GitHub Actions 与 Dependabot

**文件：**

- 新建：`.github/workflows/code-check.yml`
- 新建：`.github/dependabot.yml`

1. 添加前端、Go、覆盖率、E2E 和容器并行任务。
2. 使用最小权限、并发取消、冻结安装和失败制品上传。
3. 检查生成文件漂移，执行生产依赖审计和 Go race 测试。
4. 添加 pnpm、Go Modules、Docker 和 GitHub Actions 的每周分组更新。
5. 运行自动化配置契约测试，确认通过。

## 任务 5：完整验证与原子提交

1. 运行 `pnpm verify`、两类覆盖率、`pnpm verify:go` 和 Playwright E2E。
2. 运行 Docker 构建与健康检查；可用时执行本地漏洞扫描。
3. 运行 `git diff --check`，审查完整 diff 和暂存清单。
4. 请求合并前代码审查并处理真实问题。
5. 按覆盖率与容器、代码验证工作流、Dependabot 三个职责创建中文 Conventional Commits。
