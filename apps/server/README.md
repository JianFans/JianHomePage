# 遇健我内容与发布服务

该服务为「遇健我」管理端提供内容版本、审核、素材和发布编排接口。业务层只依赖仓储、对象存储、构建触发和身份提供器接口，因此可以把 EdgeOne、对象存储和 OIDC 服务替换为其他实现。

## 当前边界

- `internal/content`：草稿、乐观锁、审核状态和审计。
- `internal/assets`：图片、音频、视频的 MIME、扩展名、大小和签名上传校验。
- `internal/publish`：不可变快照、SHA-256、发布幂等、构建状态、原子指针切换和回滚。
- `internal/store/postgres`：嵌入式迁移、事务锁和可替换 SQL 执行器；可由 pgx 连接池包装，当前不直接绑定驱动。
- `internal/httpapi`：标准库 `net/http` 管理 API，统一错误体、`ETag`、`If-Match` 和 `Idempotency-Key`。
- `internal/providers/edgeone`：可配置的 EdgeOne 构建触发 HTTP 适配器。

公开首页不依赖该服务在线。Nuxt 构建只读取已经审核的静态快照；服务停止时，已发布的 `apps/web/.output/public` 仍可独立分发。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` 时强制要求数据库和 OIDC 配置，并禁止开发身份。 |
| `HTTP_ADDRESS` | `127.0.0.1:8080` | HTTP 监听地址。 |
| `DATABASE_URL` | 空 | 生产环境必填；不得写入日志。 |
| `OIDC_ISSUER` | 空 | 生产环境必填。 |
| `OIDC_AUDIENCE` | 空 | 生产环境必填。 |
| `ALLOW_DEV_IDENTITY` | `false` | 仅非生产环境显式设置为 `true` 时读取 `X-Dev-Subject` 和 `X-Dev-Roles`。 |
| `SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭等待时间。 |
| `EDGEONE_TRIGGER_URL` | 空 | EdgeOne 构建触发端点。 |
| `EDGEONE_STATUS_URL` | 空 | EdgeOne 构建状态端点，按 `/{buildId}` 查询。 |
| `EDGEONE_TOKEN` | 空 | 仅通过环境变量提供的 Bearer Token。 |

生产环境禁止把开发身份头当作认证信息。OIDC 实现只信任发行方签发的 `sub`、`email`、`name` 和已知角色。

## 本地运行

当前标准库 HTTP 适配器和应用服务可以用内存依赖运行本地闭环：

```powershell
cd apps/server
$env:GOCACHE = (Resolve-Path '.cache\go-build').Path
$env:GOTMPDIR = (Resolve-Path '.cache\tmp').Path
go test ./...
go vet ./...
```

开发环境显式启用开发身份后，可运行完整的草稿、审核和发布请求：

```powershell
$env:APP_ENV = "development"
$env:ALLOW_DEV_IDENTITY = "true"
go run ./cmd/api
```

服务入口提供 `GET /healthz`。生产环境不会自动降级到内存仓储；必须在应用装配时注入持久化仓储、对象存储、OIDC 和构建触发器。接入真实依赖时，启动顺序应为：加载配置 → 建立数据库连接 → 执行迁移 → 构造服务和适配器 → 注册路由 → 启动 HTTP Server。`internal/store/postgres` 的 `Executor`、`Tx` 和 `Row` 接口用于隔离具体驱动。

## 管理 API 示例

创建草稿需要 Bearer Token：

```http
POST /api/v1/versions HTTP/1.1
Authorization: Bearer <session-token>
Content-Type: application/json

{"snapshot":{"schemaVersion":"1.0.0"}}
```

响应会返回 `ETag: "1"`。更新版本必须携带该值：

```http
PUT /api/v1/versions/ver_x HTTP/1.1
If-Match: "1"
Content-Type: application/json

{"snapshot":{}}
```

发布和回滚必须携带长度至少为 8 的 `Idempotency-Key`。重复使用同一个键只返回同一个逻辑任务，不重复写快照或触发构建。

错误响应统一为：

```json
{
  "code": "conflict",
  "message": "Resource was modified by another request.",
  "requestId": "req-123"
}
```

错误正文不会包含 SQL、Token、签名 URL、数据库连接串或内部堆栈。

## 发布状态机

1. 读取已通过审核的 `in_review` 版本。
2. 重新校验快照并规范化 JSON。
3. 写入不可变对象 `snapshots/<releaseId>/<sha256>.json`。
4. 调用 `BuildTrigger`，任务进入 `building`。
5. 构建成功后，在同一事务内归档旧版本、发布新版本、切换 `production` 指针并写审计。
6. 构建失败只标记任务失败，保留上一成功指针。
7. 回滚重新触发历史快照，不覆盖历史对象。

通知器失败只产生告警，不回滚已经成功切换的发布指针。

## EdgeOne 适配器

`internal/providers/edgeone` 不依赖腾讯云 SDK。它向配置的触发端点发送：

```json
{
  "releaseId": "rel_20260830_example",
  "snapshotKey": "snapshots/rel_20260830_example/sha256:...json",
  "snapshotChecksum": "sha256:..."
}
```

状态端点返回 `pending`、`building`、`succeeded` 或 `failed`。端点和 Token 可替换，便于接 EdgeOne API 网关、Webhook 或内部发布工作流。

## 安全与运维

- 所有管理路径使用 Bearer 鉴权，写操作由 RBAC 再次校验。
- 请求正文启用未知字段拒绝和大小上限；外部链接及媒体资源由内容契约校验。
- 上传使用 15 分钟签名 URL，完成时再次校验实际大小、MIME 和 checksum。
- 审核、发布、回滚、资源删除都写入审计日志。
- 生产部署应通过 HTTPS、密钥服务和独立数据库运行，并为 `/healthz` 配置存活探针。
