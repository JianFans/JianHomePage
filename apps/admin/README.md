# 遇健我管理端

这是「遇健我」的 Vue 管理端，用于编辑版本化内容快照、提交审核、查看 EdgeOne 构建状态和发起回滚。管理端默认以 SPA 方式构建，不参与公开首页的静态首屏。

## 本地运行

```bash
pnpm --filter @yujian/admin dev
```

默认 API 地址为 `http://127.0.0.1:8080`，也可以通过 `ADMIN_API_BASE_URL` 覆盖：

```bash
ADMIN_API_BASE_URL=https://api.yujian.me pnpm --filter @yujian/admin dev
```

## 使用流程

1. 填写 Go 服务地址和当前会话的 OIDC Bearer Token。
2. 输入版本 ID 并载入现有版本，或直接编辑 JSON 创建草稿。
3. 保存时由服务端返回新的 `ETag` 和 revision；客户端不会覆盖本地未确认的版本。
4. 提交审核、通过或退回，所有状态转换由 Go 服务的 RBAC 再次校验。
5. 发布和回滚自动生成 `Idempotency-Key`，可重复点击而不会创建多个逻辑任务。
6. 发布任务进入构建阶段后，使用「刷新状态」查询 EdgeOne 适配器返回的构建状态。

## 安全边界

- Token 只保存在当前页面的内存状态，不写入 `localStorage`、URL 或静态产物。
- 管理端不解析或渲染任意 HTML、CSS 和脚本；快照以 JSON 编辑和预览为主。
- 生产 API 应使用 HTTPS、短时 OIDC Token、CSP 和独立管理域名。
- 公开首页不依赖管理端；管理端不可用不会影响已发布静态站。

## 验证命令

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```
