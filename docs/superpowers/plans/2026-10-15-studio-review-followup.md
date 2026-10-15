# 管理端会话与无障碍修复计划

> **执行方式：** 在现有工作树使用 executing-plans 逐项实现；每项先观察失败测试，再修复、验证并独立提交。收尾进行独立代码审查。

**目标：** 修复最新完整审查确认的 3 项问题，不改变登录协议、快照契约或发布流程。

**架构：** 会话基础设施故障与无效认证分别返回 503 和 401；复用既有安全错误体。弹窗将预览 iframe 纳入键盘焦点范围。新作品的字段诊断使用应用事务的追加索引。

**技术栈：** Go、Vue、Vitest、Playwright。

## 任务 1：会话存储故障保持可重试

涉及文件：`apps/server/internal/auth/middleware.go`、`apps/server/internal/auth/session.go`、`apps/server/internal/auth/session_lifecycle_test.go`、`apps/server/README.md`。

- [x] 扩展真实会话中间件测试，故障前后同一 Cookie 可用；查询故障时，受保护接口和退出均返回 503，不调用业务处理器、不清除 Cookie、不泄漏内部错误。既有测试继续覆盖发现接口 503、过期会话不认证和错误 CSRF 403；新增用例覆盖受保护接口及退出对无效或撤销会话返回 401。
- [x] 运行 `go test ./internal/auth -run 'TestSessionStorage|TestCookieSession' -count=1 -v`，新增 4 个 503 断言因实际 401 而失败。
- [x] 在认证中间件及退出的读取失败分支识别 `domain.ErrForbidden`、`domain.ErrNotFound`，其他存储错误返回安全的 `unavailable` 响应。
- [x] 目标包及 `pnpm verify:go` 通过，已同步会话文档并独立提交。

## 任务 2：预览 iframe 可由键盘进入

涉及文件：`apps/admin/components/StudioDialog.vue`、`apps/admin/test/e2e/homepage-preview.spec.ts`、`apps/admin/README.md`。

- [x] 添加真实浏览器回归：清空 Toast 后，从预览工具栏按 Tab 进入 iframe，聚焦首页首个控件；反向导航仍可回到弹窗工具栏。
- [x] 运行 `pnpm --filter @yujian/admin test:e2e homepage-preview.spec.ts --grep '键盘' --workers=2`，桌面及手机共 2 个用例因焦点被提前绕回关闭按钮而失败。
- [x] 将 iframe 纳入现有焦点候选，保留背景 inert、忙碌守卫和焦点恢复。
- [x] 重跑回归 2/2 通过，lint 和管理端类型检查通过，已独立提交。

## 任务 3：新增作品的封面诊断使用正确索引

涉及文件：`apps/admin/components/MusicWorkbench.vue`、`apps/admin/test/unit/workbench-field-issues.test.ts`、`apps/admin/test/e2e/music-workbench.spec.ts`。

- [x] 新增作品，填写其他必填字段，只漏选封面；断言路径为 `/releases/${fixture.releases.length}/coverAssetId`、存在错误说明、聚焦封面且不发送保存请求。选择封面后可成功保存。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/workbench-field-issues.test.ts --maxWorkers=4`，新增作品错误路径断言因实际索引 0、预期索引 5 而失败。
- [x] 新作品使用 `snapshot.releases.length`，已有作品保持真实索引；不修改校验器或应用事务。窄测试 5/5 通过。
- [x] 管理端完整单元测试、覆盖率及 Chrome E2E 通过；类型检查和 lint 通过。以 `fix(音乐工作台): 定位新增作品的封面错误` 独立提交。

## 收尾

- [x] 对本轮修改执行独立审查，复核错误分类、安全响应、键盘访问与诊断路径。未发现新增代码缺陷；已修正过期会话测试的文档表述。
- [x] 检查各修复提交的暂存区职责与 `git diff --check`；本轮变更按职责独立提交。

## 验证记录

| 命令 | 结果 |
| --- | --- |
| `pnpm lint` | 通过 |
| `pnpm --filter @yujian/admin typecheck` | 通过 |
| `pnpm --filter @yujian/admin test --maxWorkers=4` | 38 个文件、344 项测试通过 |
| `pnpm --filter @yujian/admin test:coverage --maxWorkers=4` | 344 项测试通过；语句 88.71%、分支 80.93%、函数 85.97%、行 91.77% |
| `pnpm --filter @yujian/admin test:e2e --workers=2` | Chrome 桌面和手机共 90 项测试通过 |
| `go test ./internal/auth -count=1`（在 `apps/server` 执行） | 通过 |
| `pnpm verify:go` | 通过 |
| `pnpm test:coverage:go` | 通过；全包语句覆盖率 83.42% |

覆盖率首次与 Chrome E2E、Go 覆盖率并发运行时，一个管理端测试超过默认 5 秒。结束其他套件后，以 4 个 worker 重跑通过，未调整测试断言、超时或覆盖率阈值。

修复提交：`d08587d`（会话错误分类）、`d9a7bfa`（预览键盘访问）、`10c53aa`（新增作品封面诊断）。

不调用真实云 API；本地验证不能代替 OIDC、COS、Pages 和 Linux 镜像验收。
