# EdgeOne Pages 发布与构建触发计划

## 目标

让「遇健我」的公开首页通过 EdgeOne Pages 分发，让 Go 服务通过可替换的 `BuildTrigger` 接口触发构建并查询状态。业务层只看稳定的发布请求和构建结果，不保存 EdgeOne 专有资源类型。

## 发布边界

- Pages 只托管 `apps/web/.output/public` 静态产物。
- 构建前注入已经审核的不可变快照；浏览器不在运行时请求内容服务。
- 动态 API 使用独立源站或子域名，再由 EdgeOne 回源规则代理 `/api/*`。
- 构建触发器使用 `apps/server/internal/providers/edgeone` 的 HTTPS 适配器。适配器端点由部署环境提供，可接 EdgeOne API 网关、Webhook 或内部发布工作流。
- Token 只从环境变量读取，不写入快照、日志、通知和错误响应。

## Pages 配置

仓库根目录的 `edgeone.json` 固化以下构建参数：

- 安装：`pnpm install --frozen-lockfile`
- 构建：`pnpm --filter @yujian/web generate`
- 输出：`apps/web/.output/public`
- Node.js：`22`

控制台配置与文件保持一致时，EdgeOne Pages 可以从仓库根目录直接构建。`apps/web/EDGEONE.md` 记录缓存和部署后冒烟检查。

## 构建触发器契约

`EdgeOne.Client` 使用两个 HTTPS 端点：

- `EDGEONE_TRIGGER_URL`：`POST`，请求体为 `releaseId`、`snapshotKey`、`snapshotChecksum`。
- `EDGEONE_STATUS_URL`：`GET /{buildId}`，返回 `id`、`status`、可选 `previewUrl` 和 `error`。

端点返回的状态必须是 `pending`、`building`、`succeeded` 或 `failed`。HTTP 非 2xx、无构建 ID、超大响应和无效 JSON 都视为失败；错误不透传供应商响应正文。

## 接入步骤

1. 在 EdgeOne Pages 创建生产项目，连接站点仓库。
2. 设置构建参数并关闭 SPA fallback，确保 `robots.txt` 和 `sitemap.xml` 是真实静态文件。
3. 配置 `CONTENT_SNAPSHOT_PATH`，由发布工作流把指定快照复制到构建输入目录。
4. 配置 Go 服务的 `EDGEONE_TRIGGER_URL`、`EDGEONE_STATUS_URL` 和 `EDGEONE_TOKEN`。
5. 通过 `go test ./internal/providers/edgeone` 验证请求体、鉴权头、状态映射和供应商错误隔离。
6. 在真实环境执行发布后冒烟：首页、站点地图、资源、试听 Range 请求和 API 回源均返回预期状态。

## 缓存与回滚

构建产物使用文件名哈希的 Nuxt 资源长期缓存； HTML 和快照入口必须重新验证。回滚只重新构建历史快照，不覆盖原有快照对象，也不修改已经记录的 checksum。

## 验收证据

- `edgeone.json` 与 `apps/web/EDGEONE.md` 参数一致。
- EdgeOne 适配器单元测试全部通过。
- 静态产物校验器拒绝运行时内容 API 标记和缺失本地资源。
- 生产发布完成后保存 EdgeOne 构建 ID、快照 checksum、预览 URL（如有）和审计记录。
