# 遇健我内容与发布服务

该服务为「遇健我」管理端提供内容版本、审核、素材和发布编排接口。业务层只依赖仓储、对象存储、构建触发和身份提供器接口，因此可以把 EdgeOne、对象存储和 OIDC 服务替换为其他实现。

## 当前边界

- `internal/content`：草稿、乐观锁、审核状态和审计。
- `internal/assets`：图片、音频、视频的 MIME、扩展名、大小和签名上传校验。
- `internal/publish`：不可变快照、SHA-256、发布幂等、构建状态、原子指针切换和回滚。
- `internal/store/postgres`：pgx 标准库驱动、嵌入式迁移、事务锁和可替换 SQL 执行器。
- `internal/httpapi`：标准库 `net/http` 管理 API，统一错误体、`ETag`、`If-Match` 和 `Idempotency-Key`。
- `internal/providers/edgeone`：可配置的 EdgeOne 构建触发 HTTP 适配器。
- `internal/providers/s3`：S3 兼容对象存储适配器，可配置腾讯云 COS 端点。

公开首页不依赖该服务在线。Nuxt 构建只读取已经审核的静态快照；服务停止时，已发布的 `apps/web/.output/public` 仍可独立分发。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` 时强制要求数据库和 OIDC 配置，并禁止开发身份。 |
| `HTTP_ADDRESS` | `127.0.0.1:8080` | HTTP 监听地址。 |
| `DATABASE_URL` | 空 | 生产环境必填；不得写入日志。 |
| `OIDC_ISSUER` | 空 | 生产环境必填。 |
| `OIDC_AUDIENCE` | 空 | 生产环境必填。 |
| `ADMIN_ALLOWED_ORIGINS` | 空 | 逗号分隔的管理端 Origin；生产环境至少一个且必须为 HTTPS。 |
| `ALLOW_DEV_IDENTITY` | `false` | 仅非生产环境显式设置为 `true` 时读取 `X-Dev-Subject` 和 `X-Dev-Roles`。 |
| `SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭等待时间。 |
| `S3_ENDPOINT` | 空 | S3 兼容 HTTPS 端点，例如腾讯云 COS 地域端点。 |
| `S3_REGION` | 空 | 对象存储地域，例如 `ap-singapore`。 |
| `S3_BUCKET` | 空 | 素材与不可变发布快照所在桶。 |
| `S3_ACCESS_KEY_ID` | 空 | 对象存储访问密钥 ID。 |
| `S3_SECRET_ACCESS_KEY` | 空 | 对象存储访问密钥；不得写入日志。 |
| `S3_SESSION_TOKEN` | 空 | 可选临时凭据 Token。 |
| `S3_USE_PATH_STYLE` | `false` | 服务商要求 path-style URL 时设置为 `true`。 |
| `MEDIA_PUBLIC_BASE_URL` | 空 | 生产环境必填；素材的稳定 HTTPS 公开基址，例如接入 EdgeOne 的 `https://media.yujian.me`。不得包含凭据、查询参数或片段。 |
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

服务入口提供 `GET /healthz`。开发模式的签名上传 URL 由同一进程的 `/local-upload/*` PUT 路由接收并校验 MIME、大小和 SHA-256；上传成功后可通过与内容契约一致的 `/media/*` 路径读取，便于本地 API 调试和内容快照引用。生产环境不会自动降级到内存仓储；启动时按顺序加载配置、连接并探测 PostgreSQL、构造并校验 S3 适配器、在同一事务中执行迁移、构造 EdgeOne 适配器、注册路由并启动 HTTP Server。任一步失败都会关闭已打开的数据库并拒绝启动。

## 容器运行

仓库根 `Dockerfile` 使用 Go 多阶段构建，只把静态 API 二进制复制到非 root 的 distroless 运行镜像。公开站和管理端不包含在该镜像中。

在仓库根目录构建并执行开发模式健康检查：

```bash
docker build --tag yujian-server:local .
docker run --detach --rm --name yujian-server-local --publish 127.0.0.1:8080:8080 yujian-server:local
curl --fail http://127.0.0.1:8080/healthz
docker rm --force yujian-server-local
```

生产部署应由编排平台注入下文列出的环境变量和密钥。不要在 Dockerfile、镜像层、构建参数或提交的 `.env` 文件中保存凭据。容器默认以 `nonroot` 用户运行并监听 `0.0.0.0:8080`；EdgeOne 回源和 TLS 在容器外配置。

生产运行示例：

```powershell
$env:APP_ENV = "production"
$env:HTTP_ADDRESS = "0.0.0.0:8080"
$env:DATABASE_URL = "postgres://..."
$env:OIDC_ISSUER = "https://id.example.com"
$env:OIDC_AUDIENCE = "yujian-admin"
$env:ADMIN_ALLOWED_ORIGINS = "https://admin.yujian.me"
$env:S3_ENDPOINT = "https://cos.ap-singapore.myqcloud.com"
$env:S3_REGION = "ap-singapore"
$env:S3_BUCKET = "yujian-media"
$env:S3_ACCESS_KEY_ID = "..."
$env:S3_SECRET_ACCESS_KEY = "..."
$env:MEDIA_PUBLIC_BASE_URL = "https://media.yujian.me"
$env:EDGEONE_TRIGGER_URL = "https://gateway.example.com/builds"
$env:EDGEONE_STATUS_URL = "https://gateway.example.com/builds"
$env:EDGEONE_TOKEN = "..."
go run ./cmd/api
```

EdgeOne 可将 `/api/*` 回源至该进程。管理端跨域请求只接受 `ADMIN_ALLOWED_ORIGINS` 中的精确 Origin；未知来源和非 HTTPS 生产来源会被拒绝。

COS 桶还需单独配置浏览器直传 CORS：Origin 与 `ADMIN_ALLOWED_ORIGINS` 保持一致，允许 `PUT`、`HEAD`，并放行 `Content-Type`、`X-Amz-Checksum-Sha256` 请求头。公开读取建议通过 `MEDIA_PUBLIC_BASE_URL` 对应的 EdgeOne 域名，不直接暴露带凭据或签名参数的存储端点。

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

创建素材上传时必须提交 `sha256:<64 位十六进制摘要>`。客户端上传至签名 URL 时还需原样携带响应中的校验头；素材响应的 `src` 是可直接写入内容快照的稳定公开地址。服务会在创建素材时持久化该地址，完成上传重试和发布校验不会根据当前提供商配置覆盖它。

发布和回滚必须携带长度至少为 8 的 `Idempotency-Key`。重复使用同一个键只返回同一个逻辑任务，不重复写快照或触发构建。同一时刻只允许一个生产发布或回滚任务处于 `pending`、`building` 状态。

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

服务启动后每 15 秒自动对账一次活动任务：`pending` 任务会安全重试同一个构建幂等键，`building` 任务会自动查询 EdgeOne 并完成指针切换。管理端的手动刷新接口仅用于即时查看，不是发布完成的必要条件。

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

触发请求同时携带 `Idempotency-Key` HTTP 头，值与当前发布任务的键一致。网关或工作流必须按该键去重，并为同一键返回同一个逻辑构建，以便服务端在网络结果不确定时安全恢复。

状态端点返回 `pending`、`building`、`succeeded` 或 `failed`。端点和 Token 可替换，便于接 EdgeOne API 网关、Webhook 或内部发布工作流。

## 安全与运维

- 所有管理路径使用 Bearer 鉴权，写操作由 RBAC 再次校验。
- 请求正文启用未知字段拒绝和大小上限；外部链接及媒体资源由内容契约校验。
- 上传使用 15 分钟签名 URL，签名和完成确认都校验 SHA-256、实际大小与 MIME；不信任客户端自定义 metadata 中的摘要。
- `MEDIA_PUBLIC_BASE_URL` 应指向 EdgeOne 加速的 COS 公开读取域名。内容快照只保存该稳定地址，不保存短期签名 URL 或服务商内部端点。更换 COS 或其他对象存储源站时应保留该公开域名，避免历史快照失效。
- 首次部署包含 `0003_publish_target_freeze` 的版本前，应等待 `pending`、`building` 发布任务结束。迁移会自动冻结 checksum 一致的单个活跃任务；如果检测到多个任务或历史数字格式导致 checksum 不一致，服务会拒绝启动。此时应先用旧版本确认并结束活跃任务，再重新部署；不得手改 checksum 或跳过迁移。
- `0004_asset_source_url` 会在同一迁移事务中锁定缺少公开地址的旧素材，使用当前对象存储适配器的 `PublicURL` 回填，并拒绝非 `NULL` 的空字符串。升级期间必须保持 `MEDIA_PUBLIC_BASE_URL` 为旧内容正在使用的稳定域名；解析或并发回填失败时服务会回滚迁移并拒绝启动。为兼容滚动升级中的旧实例和旧二进制回滚，本版本允许 `source_url` 为 `NULL`；旧实例全部退出并度过回滚窗口后，后续迁移必须再次回填期间产生的 `NULL`，再设置 `NOT NULL`。
- 审核、发布、回滚、资源删除都写入审计日志。
- 生产部署应通过 HTTPS、密钥服务和独立数据库运行，并为 `/healthz` 配置存活探针。
