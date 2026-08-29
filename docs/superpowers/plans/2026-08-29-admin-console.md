# Vue 管理端实现计划

## 目标

提供一个不参与公开首页构建的 Vue 管理端，用于编辑版本化快照、提交审核、查看发布任务和发起回滚。管理端不保存访客数据，也不在浏览器中渲染任意 HTML、脚本或 CSS。

## 当前实现

- `apps/admin` 使用 Nuxt/Vue SPA，默认 `noindex,nofollow`。
- `utils/admin-api.ts` 统一处理 Bearer Token、`If-Match`、`Idempotency-Key`、请求错误和 URL 编码。
- `utils/admin-workflow.ts` 统一表示载入、保存、审核、发布和错误状态。
- 首页工作台提供连接设置、JSON 快照编辑、预览、审核按钮、发布状态和回滚按钮。
- Token 只存在当前页面内存，不写入 URL、`localStorage` 或构建产物。
- `zh-CN` / `en` 界面切换保存在 `localStorage`，不改变 URL；API 返回的 `requestId` 会在错误提示中保留。

## API 对接

管理端只依赖以下稳定路径：

- 版本：创建、读取、带 `If-Match` 更新、提交审核、通过和退回。
- 发布：创建任务、查询任务、刷新构建状态。
- 回滚：带 `Idempotency-Key` 创建任务。

服务端再次校验 RBAC、快照契约和状态转换；管理端按钮的禁用状态只是体验优化，不能替代服务端授权。

## 验证

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```

生产接入前仍需补充真实 OIDC 登录回调、CSRF 策略、PostgreSQL 仓储和 EdgeOne 构建状态端点。这些接入不应把 Token 或服务商专有对象写进公开快照。
