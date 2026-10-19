# 遇健我内容与发布服务

该 Go 服务内嵌「遇健我」管理端，并提供内容版本、审核、素材、Vue 静态生成和 Pages 自动发布。业务层通过明确接口接入 PostgreSQL、对象存储、身份提供器和 Pages，运行时不需要 Node.js 或外部构建进程。

第一次部署先看 [部署操作指南](../../docs/DEPLOYMENT.md)：在 GitHub Actions 手动运行 `Release`，从 Releases 下载编译好的单程序包，再配置账号登录、数据库和云服务。本地电脑与服务器都无需编译。本文用于查完整参数及 API 行为。

## 当前边界

- `internal/content`：草稿、乐观锁、审核状态和审计。
- `internal/assets`：图片、动图、音频、视频的 MIME、扩展名、大小、签名上传校验与素材游标分页。
- `internal/publish`：审核版本的不可变快照、SHA-256、发布幂等、构建状态、原子指针切换和回滚。
- `internal/store/postgres`：pgx 标准库驱动、嵌入式迁移、事务锁和可替换 SQL 执行器。
- `internal/httpapi`：标准库 `net/http` 管理 API，统一错误体、`ETag`、`If-Match` 和 `Idempotency-Key`。
- `internal/publisher`：持久化发布 worker、固定目标、幂等和人工恢复。
- `internal/providers/pages`：Pages Direct Upload 与部署状态查询。
- `internal/sitegen`：Goja 执行内嵌 Vue SSR，生成 HTML、robots 和 sitemap。
- `internal/webassets`：内嵌管理端、渲染器和带内容哈希的公开浏览器资源。
- `internal/adminweb`：同源管理端 `/`、隔离预览 `/preview` 与静态资源服务。
- `internal/providers/s3`：只处理 `assets/` 媒体键的 S3 兼容存储适配器，可配置腾讯云 COS 端点。

公开首页不依赖该服务在线。发布时只读取已经审核的不可变快照；服务停止时，已上传 Pages 的 HTML 和浏览器资源仍可独立分发。

## 环境变量

默认从当前工作目录读取 `.env`，不搜索父目录；进程环境变量优先，包括显式空值。缺失 `.env` 时继续读取环境变量，文件读取或解析失败则拒绝启动。模板见根目录 [.env.example](../../.env.example)，凭据来源见 [部署说明](../../docs/DEPLOYMENT.md#环境变量)。`--help`、`--version` 和 `--export-site` 不加载该文件。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` 时强制要求数据库和 OIDC 配置，并禁止开发身份。 |
| `HTTP_ADDRESS` | `127.0.0.1:8080` | HTTP 监听地址。 |
| `DATABASE_URL` | 空 | 生产环境必填；不得写入日志。 |
| `OIDC_ISSUER` | 空 | 生产环境必填。 |
| `OIDC_AUDIENCE` | 空 | 生产环境必填。 |
| `OIDC_ROLE_EDITOR` | `editor` | 身份服务中的内容编辑角色名。 |
| `OIDC_ROLE_REVIEWER` | `reviewer` | 身份服务中的内容审核角色名。 |
| `OIDC_ROLE_PUBLISHER` | `publisher` | 身份服务中的发布与回滚角色名。 |
| `OIDC_ROLE_ADMIN` | `admin` | 身份服务中的管理员角色名。 |
| `ADMIN_OIDC_CLIENT_ID` | 空 | 浏览器授权码登录的 OIDC client ID；ID token 的 audience 使用该值。 |
| `ADMIN_OIDC_CLIENT_SECRET` | 空 | 可选 confidential client 密钥，通过 token endpoint 的 HTTP Basic 发送。 |
| `ADMIN_OIDC_REDIRECT_URL` | 空 | 身份提供商注册的固定回调 URL，路径为 `/api/v1/auth/callback`。 |
| `ADMIN_RETURN_URL` | 空 | 登录完成后的固定管理端 URL；Origin 必须位于允许列表。 |
| `ADMIN_SESSION_KEY` | 空 | Base64 编码的 32 字节随机密钥，用于 AEAD 加密短期登录 Cookie。不得提交或记录。 |
| `ADMIN_ALLOWED_ORIGINS` | 空 | 逗号分隔的管理端 Origin；生产环境至少一个且必须为 HTTPS。 |
| `ALLOW_DEV_IDENTITY` | `false` | 仅非生产环境显式设置为 `true` 时读取 `X-Dev-Subject` 和 `X-Dev-Roles`。 |
| `SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭等待时间。 |
| `S3_ENDPOINT` | 空 | S3 兼容 HTTPS 端点，例如腾讯云 COS 地域端点。 |
| `S3_REGION` | 空 | 对象存储地域，例如 `ap-singapore`。 |
| `S3_BUCKET` | 空 | 媒体素材桶；不存储网站快照或构建产物。 |
| `S3_ACCESS_KEY_ID` | 空 | 对象存储访问密钥 ID。 |
| `S3_SECRET_ACCESS_KEY` | 空 | 对象存储访问密钥；不得写入日志。 |
| `S3_SESSION_TOKEN` | 空 | 可选临时凭据 Token。 |
| `S3_USE_PATH_STYLE` | `false` | 服务商要求 path-style URL 时设置为 `true`。 |
| `MEDIA_PUBLIC_BASE_URL` | 空 | 生产环境必填；素材的稳定 HTTPS 公开基址，例如接入 EdgeOne 的 `https://media.yujian.me`。不得包含凭据、查询参数或片段。 |
| `PAGES_API_TOKEN` | 空 | 生产环境必填；Pages API Token，从 `.env`、环境变量或密钥服务注入。 |
| `PAGES_PROJECT_ID` | 空 | 生产环境必填；Direct Upload 项目的 ID。 |
| `PAGES_REGION` | 空 | 生产环境必填；`china` 或 `global`。 |
| `PAGES_PUBLIC_URL` | 空 | 生产环境必填；公开站 HTTPS 基址，不得包含凭据、查询参数或片段。 |

生产环境禁止把开发身份头当作认证信息。OIDC 实现只信任发行方签发的 `sub`、`email`、`name` 和已知角色。

### 浏览器登录与会话

浏览器登录使用身份提供商托管的标准登录页、授权码和 S256 PKCE。管理端不接收密码，也不使用 password grant。配置 `ADMIN_OIDC_CLIENT_ID`、`ADMIN_OIDC_REDIRECT_URL`、`ADMIN_RETURN_URL`、`ADMIN_SESSION_KEY` 时须同时配置 `OIDC_ISSUER` 和 `ADMIN_ALLOWED_ORIGINS`；任何部分配置都会阻止启动。生产环境的 issuer、回调和返回 URL 必须为 HTTPS。`ADMIN_OIDC_CLIENT_SECRET` 按身份提供商的客户端类型选填；需为 ID token 配置顶层 `roles` 声明。

ID token 和 API Bearer token 共用 `OIDC_ROLE_*` 映射：外部名称精确匹配后转为内部 `editor`、`reviewer`、`publisher`、`admin`，合并权限并去重。默认使用标准名称；自定义值替换默认名，空值回退默认。角色名区分大小写，不能含空白、控制字符或与其他映射重复。`roles` 支持字符串数组或空白分隔的字符串，忽略无关角色；无匹配角色、缺失声明或数组含非字符串项时拒绝认证。持久会话和开发身份仍严格校验内部标准角色。更改映射需重启服务，已有会话保持原权限至退出或过期，重新登录后应用新映射。

`GET /api/v1/auth/session` 无需登录，返回 `authenticated` 和 `loginAvailable`。已认证时另含 `principal`、`csrfToken`、`expiresAt`；登录未配置时返回两个 `false`，不会生成开发身份或演示会话。`GET /api/v1/auth/login` 跳转至身份提供商；回调校验一次性 state、nonce、签名、issuer、client audience、有效期和授权方后设置会话。回调只跳转至配置的固定 URL，忽略用户提供的返回目标。

`principal.name` 和可选 `principal.picture` 取自已验证 ID token 的显示名称与头像声明。标准字段为 `name`、`picture`；兼容 Casdoor 的 `displayName`（优先于登录名）和 `avatar`。头像须为无凭据的绝对 HTTP(S) URL，生产仅接受 HTTPS。缺少资料时显示账号标识和占位头像，不额外调用 UserInfo；身份服务须将资料声明加入 ID token。既有会话需退出并重新登录才能更新资料。

会话 Cookie 为不透明随机值，设置 `HttpOnly`、`SameSite=Lax`、主机作用域和 `/api/` 路径，生产环境还设置 `Secure`。PostgreSQL 仅保存 bearer 的 SHA-256 摘要；会话在 ID token 过期或 8 小时后失效（取较早值），每次请求都检查有效期。生产环境使用迁移 `0006_admin_sessions.sql` 创建持久会话及已消费 state 表，服务重启和多实例不会恢复已撤销会话。开发模式显式选择内存依赖，会话随进程消失。

管理 API 同时支持既有 Bearer JWT 和会话 Cookie。Cookie 写操作必须发送允许列表中的精确 `Origin` 和当前会话的 `X-CSRF-Token`；CORS 只对允许来源启用 credentials。`POST /api/v1/auth/logout` 使用同样校验，成功后撤销数据库会话并清除 Cookie，旧 Cookie 不能再次访问。所有认证响应禁止缓存。签名素材 URL 上传独立于会话认证，浏览器直传必须使用 `credentials: 'omit'`。

会话缺失、过期或已撤销时，管理 API 与退出接口返回 401；Cookie 写操作的 CSRF 校验失败返回 403。会话查询或撤销遇到数据库故障时返回 503，不清除 Cookie，也不把有效会话标记为失效；数据库恢复后可重试原操作。错误响应不包含底层存储信息。

数据库运维可定期删除 `admin_sessions` 和 `admin_login_flows` 中 `expires_at < now()` 的记录，以控制过期记录数量；清理延迟不影响认证有效期或撤销校验。生产接入仍需在真实身份提供商验证回调白名单、RS256 JWKS、角色声明和同站 HTTPS 域名部署；本地测试不能替代这些配置。

## 本地运行

先在仓库根目录生成 Go 内嵌资源，再运行测试；缺少完整资源时生产启动会失败：

```powershell
pnpm build:server:assets
cd apps/server
go test ./...
go vet ./...
```

开发环境显式启用开发身份后，可运行完整的草稿、审核和发布请求：

```powershell
$env:APP_ENV = "development"
$env:ALLOW_DEV_IDENTITY = "true"
go run ./cmd/api
```

服务入口提供管理页面 `/`、隔离预览 `/preview` 和 `GET /healthz`。开发模式的签名上传 URL 由同一进程的 `/local-upload/*` PUT 路由接收并校验 MIME、大小和 SHA-256；上传成功后可通过与内容契约一致的 `/media/*` 路径读取，便于本地 API 调试和内容快照引用。生产环境不会自动降级到内存仓储；启动时加载配置、探测 PostgreSQL、校验 S3、事务执行迁移，并校验 Pages、发布仓储和内嵌资源后注册路由。任一步失败都会关闭已打开的数据库并拒绝启动。

正式部署使用 Releases 中的 Linux amd64／arm64 裸二进制及 `SHA256SUMS.txt`，`--version` 查看应用版本。`Check Code` 的二进制制品保留 7 天；`Release` 先同步 Node／Go 版本并原子推送 commit 和 tag，再检查与构建该提交。工具链版本与内容 Schema 版本独立；配置、启动和失败恢复见 [部署说明](../../docs/DEPLOYMENT.md)。

仅开发时需要自行编译：在仓库根目录运行 `pnpm build:server`，输出 `dist/yujian-server`，Windows 为 `dist/yujian-server.exe`。此命令生成前端资源并编译 Go，不自动运行测试。交付二进制已包含管理端、SSR 渲染器和公开浏览器资源，目标机器无需安装 Node.js。`GOOS`、`GOARCH` 可用于指定编译目标。

仅导出本地预览时可使用以下命令，输出目录必须为空：

```powershell
.\dist\yujian-server.exe --export-site content/fixtures/homepage.json --output dist/preview
```

该模式使用同一内嵌渲染器和浏览器资源，允许 fixture 与开发相对素材地址，不连接数据库或 Pages。导出目录包含 HTML 与浏览器资源；预览相对 `/media/*` 时，静态服务器还需提供对应开发媒体文件。正式发布 worker 仍要求审核快照和稳定 HTTPS 素材。

## 容器运行

仓库根 `Dockerfile` 先在 Node.js 阶段生成内嵌前端资源，再编译 Go，只把包含管理端和发布资源的二进制复制到非 root 的 distroless 运行镜像。运行镜像不包含 Node.js。

Go 标准库会编入 API 二进制。修复标准库漏洞时，需要更新 `Dockerfile` 构建阶段的 Go 补丁版本并重新构建；仅更新 distroless 运行镜像不能替换二进制中的标准库。CI 会扫描最终镜像，并阻止包含 HIGH 或 CRITICAL 漏洞的镜像通过。

生产部署可使用 Actions 发布的 `ghcr.io/jianfans/jianhomepage:vX.Y.Z`。镜像目前支持 Linux amd64，ARM64 使用 Releases 裸二进制。私有包需要拉取权限，生产固定版本标签。

以下命令仅用于开发时自行构建镜像并执行开发模式健康检查：

```bash
docker build --tag yujian-server:local .
docker run --detach --rm --name yujian-server-local --publish 127.0.0.1:8080:8080 yujian-server:local
curl --fail http://127.0.0.1:8080/healthz
docker rm --force yujian-server-local
```

生产通过环境变量或容器工作目录下只读挂载的 `.env` 提供配置，使用根 [.env.example](../../.env.example) 中的完整字段。不要把凭据写入镜像。容器以 `nonroot` 用户运行，默认监听 `0.0.0.0:8080`。管理域名 HTTPS 回源覆盖 `/`、`/preview`、`/_nuxt/*` 和 `/api/*`；公开域名由 Pages 分发。

COS 桶还需单独配置浏览器直传 CORS：Origin 与 `ADMIN_ALLOWED_ORIGINS` 保持一致，允许 `PUT`、`HEAD`，并放行 `Content-Type`、`X-Amz-Checksum-Sha256` 请求头。公开读取建议通过 `MEDIA_PUBLIC_BASE_URL` 对应的 EdgeOne 域名，不直接暴露带凭据或签名参数的存储端点。

## 管理 API 示例

### 版本库与克隆

`GET /api/v1/versions?limit=20&cursor=...` 允许 `editor`、`reviewer`、`publisher` 和 `admin`，返回 `{items, nextCursor?}`。每项包含 `id`、`name`、可选 `sourceVersionId`、`status`、`revision`、`checksum`、`createdAt`、`updatedAt` 和可选 `reviewApproved`，不读取或返回内容快照。默认每页 20 项，上限 100 项，按 `(created_at, id)` 倒序稳定分页；非法、超长或重复分页参数返回 400。客户端须原样传递返回的游标。

`POST /api/v1/versions` 兼容已有 `{snapshot}` 请求，并接受可选 `name`。显示名称会去除首尾空白，最多 120 个字符，不允许控制字符；省略或空名称回退为版本 ID。`name` 和 `sourceVersionId` 是版本元数据，不进入 canonical 内容快照及其 checksum。旧版本无名称时也显示 ID。

`POST /api/v1/versions/{versionId}/clone` 接受 `{name}`，仅允许 `editor` 和 `admin`。服务在同一事务中读取源版本、校验 canonical JSON、checksum 和内容语义，生成新 ID、独立草稿和审计记录，再返回完整版本与 `ETag: "1"`。副本保留 `sourceVersionId`，固定 `status: "draft"`、`revision: 1`、无审核批准；源版本及线上发布指针不变。源不存在、角色不允许或快照损坏时不会创建副本。请求不能携带客户端 `snapshot` 覆盖服务端源内容。

迁移 `0007_version_library.sql` 以可重复执行的方式新增名称和来源字段及分页索引，既有版本和内容快照保持兼容。来源字段是 `content_versions` 自引用外键，不复制发布任务或发布指针。

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

### 素材列表

```http
GET /api/v1/assets?status=ready&limit=50 HTTP/1.1
Authorization: Bearer <session-token>
```

接口使用 `create_asset` 权限，允许 `editor` 和 `admin`。参数如下：

| 参数 | 省略时 | 约束 |
| --- | --- | --- |
| `status` | 返回 `pending` 和 `ready` | 显式值只允许 `pending`、`ready`、`deleted` |
| `limit` | 50 | 整数，范围 1–100 |
| `cursor` | 第一页 | 原样传入上一响应的 `nextCursor`，不要自行生成或修改 |

响应为 `{"items": [...], "nextCursor": "..."}`，最后一页省略 `nextCursor`，空列表的 `items` 为 `[]`。记录按 `createdAt DESC, id DESC` 排序，游标是分页边界，不是整个素材库的冻结快照。切换状态应从第一页开始；类型和文件名搜索由管理端在已加载记录中执行，API 不提供这两种筛选参数。

已知参数重复、显式空值、越界 `limit`、非法 URL 编码及无效游标返回 `400 invalid_request`。游标校验拒绝缺失时间、空白 ID、NUL 字符、未知 JSON 字段和尾随数据。列表返回稳定 `src`、状态、metadata、权利及时间，不返回存储对象键或创建者身份。

列表不产生业务审计；为兼容旧实例，会原子补全返回页中尚未持久化的素材地址，失败时整个请求失败。完整契约见 [管理 API 规范](../../packages/schema/openapi/admin.yaml)。

### 发布与错误响应

发布和回滚必须携带有效 UTF-8 的 `Idempotency-Key`。去除首尾 Unicode White_Space 后，长度必须为 8–128 个 UTF-8 字节，且不能包含控制字符；BOM（U+FEFF）作为键的一部分保留。非法键返回 400，不冻结版本、写快照或创建任务。重复使用同一个归一化键只返回同一个逻辑任务，不重复写快照或触发构建。同一时刻只允许一个生产发布或回滚任务处于 `pending`、`building` 状态。

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
3. 将快照原文写入 PostgreSQL `publish_snapshots`，按 release ID 与 SHA-256 不可变寻址。
4. 通过进程内 `BuildTrigger` 创建持久化 worker 任务，发布任务进入 `building`。
5. 构建成功后，在同一事务内归档旧版本、发布新版本、切换 `production` 指针并写审计。
6. 构建失败只标记任务失败，保留上一成功指针。
7. 回滚从保留的内容版本重新校验并写入同一不可变快照仓库，不读取或写入 COS 快照。

服务启动后每 15 秒先运行发布 worker，再对账内容发布任务：`pending` 任务按原幂等键关联同一构建，`building` 任务自动更新部署结果并完成指针切换。管理端的手动刷新接口仅用于即时查看，不是发布完成的必要条件。

通知器失败只产生告警，不回滚已经成功切换的发布指针。

## 内置 Pages 发布与恢复

`internal/publisher` 与 API 在同一个 Go 进程运行。任务沿用 `0008_publisher_builds` 创建的 `publisher_builds` 表，阶段为 `queued`、`building`、`submitting`、`deployed`、`succeeded` 或 `failed`。固定 PostgreSQL 连接持有全局 worker 锁，网络请求期间不持有数据库事务；服务重启后继续读取持久化任务。

每个任务冻结项目 ID、地域、公开 URL 和完整 Go 二进制的 SHA-256。身份包含 Go 逻辑与内嵌资源，无需额外代码版本环境变量。worker 从 PostgreSQL 读取快照，并校验 checksum、release ID、canonical 契约及生产素材地址，使用 Goja 渲染页面，再通过 [Makers Direct Upload](https://pages.edgeone.ai/zh/document/direct-upload) 创建 Production 部署。构建包交给 Makers 的临时上传目标由 Makers API 提供；`S3_*` 配置只供媒体管理使用。目标漂移时不会用新配置继续旧任务。

调用云端创建前持久化 `submitting`。超时、断连或响应无法确认时保留该阶段，不自动重试创建；先在腾讯云核实部署，再由 `admin` 角色恢复：

- `GET /api/v1/publisher/builds/{buildId}`：读取任务阶段、冻结目标与已绑定部署。
- `POST /api/v1/publisher/builds/{buildId}/recovery`：仅恢复 `submitting`，且必须匹配当前冻结目标。

已确认创建部署时，提交部署 ID 和核实说明：

```json
{"deploymentId":"deployment-id","note":"已在 Pages 核实对应部署"}
```

服务重新查询部署并验证项目及 Production 环境，绑定后由正常对账完成状态更新。只有确认云端未创建部署时才使用：

```json
{"confirmedAbsent":true,"note":"已核实该次提交未创建部署"}
```

这会将构建标记为失败，再由正常发布对账释放活动状态。两种恢复选择互斥，均必须提供 `note`。会话 Cookie 写操作需要精确 `Origin` 和 `X-CSRF-Token`，也支持已验证的 Bearer JWT。审计操作者由服务端认证身份记录，不接受客户端指定身份。

升级前应先结束旧独立发布程序的活动任务，保留历史记录。存在历史 `submitting` 时应使用与冻结身份匹配的旧程序核对恢复；不要直接修改数据库、替换冻结身份或重复提交云端创建。切换完成后停用旧程序，避免同时调度。

## 安全与运维

- 管理 API 使用已验证的会话或 Bearer 鉴权，写操作由 RBAC 再次校验；可公开读取的管理端静态文件不授予 API 权限。
- 请求正文启用未知字段拒绝和大小上限；外部链接及媒体资源由内容契约校验。
- 上传使用 15 分钟签名 URL，签名和完成确认都校验 SHA-256、实际大小与 MIME；不信任客户端自定义 metadata 中的摘要。
- 普通 API 保持 15 秒读写期限。本地 `/local-upload/*` 只有在签名、有效期、MIME、声明大小和校验头通过后，才将本次连接的读写期限延长为 15 分钟；仍按声明大小流式校验，不全量缓存文件。生产素材直接上传到对象存储，不经过该本地路由。
- 开发素材保存在临时文件中，正常关闭会等待活动写入并清理文件；超过 `SHUTDOWN_TIMEOUT` 时先断开在途连接，再释放依赖。Windows 媒体读取句柄允许共享删除，删除后新的读取失败，已有下载可继续读完；Range 请求仍可用。开发上传不作为持久化生产存储。
- `MEDIA_PUBLIC_BASE_URL` 应指向 EdgeOne 加速的 COS 公开读取域名。内容快照只保存该稳定地址，不保存短期签名 URL 或服务商内部端点。更换 COS 或其他对象存储源站时应保留该公开域名，避免历史快照失效。
- `0009_publish_snapshots` 在 PostgreSQL 创建有大小上限的不可变构建快照表；不从旧 COS 复制快照。回滚输入来自保留的 `content_versions` 快照。升级时先让旧二进制完成活动发布，再切换到新版本；无需为媒体桶授予 `snapshots/` 权限。
- 首次部署包含 `0003_publish_target_freeze` 的版本前，应等待 `pending`、`building` 发布任务结束。迁移会自动冻结 checksum 一致的单个活跃任务；如果检测到多个任务或历史数字格式导致 checksum 不一致，服务会拒绝启动。此时应先用旧版本确认并结束活跃任务，再重新部署；不得手改 checksum 或跳过迁移。
- `0004_asset_source_url` 会在同一迁移事务中锁定缺少公开地址的旧素材，使用当前对象存储适配器的 `PublicURL` 回填，并拒绝非 `NULL` 的空字符串。升级期间必须保持 `MEDIA_PUBLIC_BASE_URL` 为旧内容正在使用的稳定域名；解析或并发回填失败时服务会回滚迁移并拒绝启动。为兼容滚动升级中的旧实例和旧二进制回滚，本版本允许 `source_url` 为 `NULL`；旧实例全部退出并度过回滚窗口后，后续迁移必须再次回填期间产生的 `NULL`，再设置 `NOT NULL`。
- 滚动升级期间，旧实例仍可能写入没有 `source_url` 的素材。新实例读取素材列表时只补全实际返回页中缺少的地址，并原子持久化首次胜出的地址；不改写状态、元数据或已存在的地址。解析或持久化失败会使该次列表请求失败，不返回空 `src`。旧实例退出前应保持稳定公开域名，不能借滚动升级更换历史素材的域名。
- `0005_asset_list_indexes` 为单状态筛选添加状态、创建时间和 ID 的复合索引，为未删除列表添加部分索引。迁移在启动事务中执行，首次构建索引会暂时阻塞素材表写入；应在维护窗口执行，并为大表预留启动时间。索引不改变旧实例读写契约，也无需重写素材数据。
- 审核、发布、回滚、资源删除都写入审计日志。
- 生产部署应通过 HTTPS、密钥服务和独立数据库运行，并为 `/healthz` 配置存活探针。

## PostgreSQL 集成验证

先在仓库根目录运行 `pnpm build:server:assets`。指定 `YUJIAN_TEST_POSTGRES_URL` 后，可在 `apps/server` 中运行真实迁移、素材地址并发补全、分页查询计划和发布恢复检查：

```bash
go test ./internal/store/postgres -run TestPostgresAsset -count=1 -v
go test ./internal/store/postgres -run TestPostgresPublisher -count=1 -v
```

测试账号需要创建和删除 Schema 的权限。每个测试创建独立 Schema，执行两次迁移检查可重复性，结束后删除测试 Schema；不会访问现有应用表。分页测试创建 100,000 条临时素材，对仓储实际执行的 SQL 运行 `EXPLAIN (ANALYZE, BUFFERS)`，检查首、深页排序和游标、索引使用及扫描行数。未提供测试数据库时，这些集成测试明确跳过，其他单元测试仍正常执行。

发布集成测试检查历史迁移兼容、单活动任务竞争、固定连接锁、服务重建及结果不明后的人工恢复。云端适配器使用受控测试实现，不证明真实 Pages 账号、权限或网络已验证。
