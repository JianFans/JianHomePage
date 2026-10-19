# 部署

运行单个 Go 二进制，内含管理端、API、静态生成器和 Pages 发布器；服务器无需 Node.js。公开站由 EdgeOne Pages 分发。

## 程序与启动

[Release 工作流](https://github.com/JianFans/JianHomePage/actions/workflows/release.yml) 在最新 `master` 手动触发，`bump` 为 `patch`、`minor` 或 `major`。它同步 Node／Go 版本，原子推送版本 commit 和 tag，检查并构建该提交，再发布到 [Releases](https://github.com/JianFans/JianHomePage/releases)。Actions 需有默认分支、标签、Release 和 GHCR 的写权限。

下载对应架构的裸二进制 `yujian-server-X.Y.Z-linux-amd64` 或 `yujian-server-X.Y.Z-linux-arm64`，以及 `SHA256SUMS.txt`。以下以 `0.1.0`、amd64 为例：

将源码中的 [.env.example](../.env.example) 复制为服务根目录的 `.env` 并填写下表。程序只读取**当前工作目录**的 `.env`，进程环境变量优先（包括显式空值）；不搜索父目录。缺失文件时使用环境变量，文件不可读或语法错误时拒绝启动。进程托管工具需将工作目录设为服务根目录，`.env` 仅对服务账号可读。值包含 `$` 或 `#` 时用单引号包裹。

在服务根目录校验并启动：

```bash
sha256sum --check --ignore-missing SHA256SUMS.txt
install -m 755 yujian-server-0.1.0-linux-amd64 ./yujian-server
./yujian-server --version
./yujian-server
```

容器可使用 `ghcr.io/jianfans/jianhomepage:vX.Y.Z`（目前仅 linux/amd64），通过环境变量或在容器工作目录挂载 `.env` 配置。容器监听地址应为 `0.0.0.0:8080`；私有 GHCR 包需有拉取权限。

## 环境变量

下表以 `admin.yujian.me` 为管理域名，`media.yujian.me` 为素材域名；按实际域名修改。标记「必填」表示完整生产后台所需。

| 变量 | 要求 | 值／获取方式 |
| --- | --- | --- |
| `APP_ENV` | 必填 | `production` |
| `HTTP_ADDRESS` | 默认 `127.0.0.1:8080` | HTTP 监听地址 |
| `DATABASE_URL` | 必填 | PostgreSQL 连接串，由数据库服务提供；密码中的保留字符需 URL 编码；使用直连或会话级连接池 |
| `OIDC_ISSUER` | 必填 | OIDC 服务的 issuer，须与 discovery 文档一致 |
| `OIDC_AUDIENCE` | 必填 | 身份服务为本 API 配置的 access token audience |
| `OIDC_ROLE_EDITOR` | 默认 `editor` | 身份服务中对应本站内容编辑权限的角色名 |
| `OIDC_ROLE_REVIEWER` | 默认 `reviewer` | 身份服务中对应本站审核权限的角色名 |
| `OIDC_ROLE_PUBLISHER` | 默认 `publisher` | 身份服务中对应本站发布与回滚权限的角色名 |
| `OIDC_ROLE_ADMIN` | 默认 `admin` | 身份服务中对应本站管理员权限的角色名 |
| `ADMIN_OIDC_CLIENT_ID` | 必填 | 身份服务中新建 OIDC 客户端的 Client ID，启用授权码流程和 S256 PKCE |
| `ADMIN_OIDC_CLIENT_SECRET` | 按客户端类型 | confidential client 的 Client Secret，从身份服务客户端配置获取；public client 留空 |
| `ADMIN_OIDC_REDIRECT_URL` | 必填 | `https://admin.yujian.me/api/v1/auth/callback`，同时注册到 OIDC 回调白名单 |
| `ADMIN_RETURN_URL` | 必填 | `https://admin.yujian.me/` |
| `ADMIN_ALLOWED_ORIGINS` | 必填 | `https://admin.yujian.me`，多个 Origin 用逗号分隔，不带末尾 `/` |
| `ADMIN_SESSION_KEY` | 必填 | 本机生成：`openssl rand -base64 32`；不是腾讯云密钥 |
| `S3_ENDPOINT` | 必填 | COS 地域端点，如 `https://cos.ap-singapore.myqcloud.com`，不含桶名；见 [地域与域名](https://cloud.tencent.com/document/product/436/6224) |
| `S3_REGION` | 必填 | COS 桶所在地域，如 `ap-singapore`，从桶概览获取 |
| `S3_BUCKET` | 必填 | COS 完整桶名，含 APPID 后缀，从桶概览获取 |
| `S3_ACCESS_KEY_ID` | 必填 | 腾讯云 [CAM API 密钥](https://console.cloud.tencent.com/cam/capi) 中的 SecretId |
| `S3_SECRET_ACCESS_KEY` | 必填 | 同一对凭据的 SecretKey；授权访问所用 COS 桶 |
| `S3_SESSION_TOKEN` | 临时凭据时 | STS 返回的 sessionToken，与临时 SecretId／SecretKey 配套；到期前需更新并重启 |
| `S3_USE_PATH_STYLE` | 默认 `false` | COS 保持 `false`；其他 S3 兼容服务按需修改 |
| `MEDIA_PUBLIC_BASE_URL` | 必填 | 稳定 HTTPS 素材域名，如 `https://media.yujian.me`，经 EdgeOne 回源 COS |
| `PAGES_API_TOKEN` | 必填 | EdgeOne Pages 控制台的 API Token 页面创建，见 [官方说明](https://edgeone.cloud.tencent.com/pages/document/176921267881082880)；不是 CAM SecretKey |
| `PAGES_PROJECT_ID` | 必填 | Pages **Direct Upload** 项目的 ID，从项目详情获取；不是项目名称 |
| `PAGES_REGION` | 必填 | 中国站 `china`，国际站 `global`；项目、Token 与站点一致，不是 COS 地域 |
| `PAGES_PUBLIC_URL` | 必填 | Pages 绑定的公开站地址，如 `https://yujian.me`，须与内容快照 canonical URL 一致 |
| `SHUTDOWN_TIMEOUT` | 默认 `10s` | 优雅停止超时 |
| `ALLOW_DEV_IDENTITY` | 默认 `false` | 生产环境必须为 `false` |

OIDC 可使用标准身份服务；令牌要求及 Casdoor 示例见下节。完整认证契约见 [浏览器登录与会话](../apps/server/README.md#浏览器登录与会话)。

COS 的 SecretId／SecretKey 从 [CAM](https://console.cloud.tencent.com/cam/capi) 创建或获取，使用限制到目标桶的子账号权限；Pages Token 在 [Pages 控制台](https://console.cloud.tencent.com/edgeone/pages) 单独创建。两组凭据都只配置在服务器，不需要放入 Actions。

## OIDC 登录与权限（以 Casdoor 为例）

`aud` 表示令牌发给哪个应用，`roles` 表示用户在本站的权限，两者均由身份服务签发：

| 使用方式 | 本站检查 |
| --- | --- |
| 后台网页登录 | ID token 的 `aud` 包含 `ADMIN_OIDC_CLIENT_ID`；验证后创建 Cookie 会话，之后无需手动提供令牌 |
| 程序直接调用 API | `Authorization: Bearer <access_token>` 中令牌的 `aud` 包含 `OIDC_AUDIENCE` |
| 两种方式的权限 | 顶层 `roles` 至少包含一个配置的角色名，例如默认配置下的 `roles: ["admin"]`；其他角色名忽略，多个匹配角色的权限合并 |

`aud` 不是密码，不需要手动修改令牌。两种方式都校验签名、issuer 和有效期。`roles` 支持字符串数组或空白分隔的字符串；完全无匹配角色、声明缺失或数组含非字符串项时拒绝认证。Casdoor 的 `isAdmin` 或角色显示名称不替代本站 `roles`。

角色名区分大小写，可用上表四个 `OIDC_ROLE_*` 变量配置。留空或未设置时使用默认名；自定义后只匹配新名称，不再接受原默认名。名称不能包含空白或控制字符，四个名称不能重复。例如 `OIDC_ROLE_ADMIN=yujian-admin` 时，`roles: ["other-app-user", "yujian-admin"]` 可获得本站管理员权限，只有 `roles: ["admin"]` 则不能。修改后重启服务；已有会话保留原权限至退出或过期，需重新登录使新映射生效。API 和管理端会话内仍使用标准角色名。

Casdoor 普通、非共享应用可按以下方式配置，其他 OIDC 服务遵循相同令牌契约：

1. 在应用详情复制 **Client ID**、**Client Secret**，分别填写 `ADMIN_OIDC_CLIENT_ID`、`ADMIN_OIDC_CLIENT_SECRET`。`OIDC_ISSUER` 填 Casdoor 服务根地址，例如 `https://auth.example.com`，须与 `/.well-known/openid-configuration` 返回的 issuer 一致。此示例中 `OIDC_AUDIENCE` 填同一 Client ID；共享应用或另设 API audience 时须按实际 `aud` 调整，ID token 仍须匹配后台 Client ID。
2. **Redirect URLs** 添加 `https://admin.yujian.me/api/v1/auth/callback`，与 `ADMIN_OIDC_REDIRECT_URL` 完全一致。应用需支持授权码和 S256 PKCE。**Token signing method** 使用 `RS256`，签名证书的公钥须出现在根 discovery 返回的 `jwks_uri` 中；仅应用专属 JWKS 可见的证书不适用于本站当前配置。
3. 在 Casdoor **Roles** 中创建名称为 `admin` 的角色，启用并在 **Users** 中加入实际管理员账号。其他人员按需分配 `editor`、`reviewer`、`publisher`；已有角色名称不同时配置对应的 `OIDC_ROLE_*`。用户可以同时持有其他应用的角色；不要给所有登录用户统一授予管理员权限。
4. 应用的 **Token format** 选择 `JWT-Custom`，在 **Token attributes** 新增以下映射。它提取用户已分配角色的名称，而非输出默认角色对象；仅在 **Token fields** 勾选 `Roles` 不等价于此映射。

   | Name | Category | Value | Type |
   | --- | --- | --- | --- |
   | `roles` | `Existing Field` | `Roles` | `Array` |

5. 保存后重新登录，使新令牌包含更新后的角色。管理员令牌的相关字段应类似以下片段；`aud` 可为字符串或包含该 Client ID 的数组，实际令牌还须包含有效的 `sub`、`exp`，登录 ID token 须保留 `nonce`。不要将完整令牌上传到公共解析网站。

   ```json
   {
     "iss": "https://auth.example.com",
     "aud": ["0123456789abcdef0123"],
     "roles": ["admin"]
   }
   ```

此映射要求 Casdoor 版本提供 `Existing Field` 类别及 `Roles` 字段；缺少时先升级，不能用静态 `admin` 代替。配置参考：[OIDC 客户端](https://casdoor.org/docs/how-to-connect/oidc-client/)、[自定义令牌属性](https://casdoor.org/docs/token/overview/#custom-token-attributes)、[用户角色](https://casdoor.org/docs/user/roles/)。

侧栏资料来自已验证 ID token：显示名使用非空 `displayName`，否则 `name`；头像使用 `picture`，缺失时使用 `avatar`。身份服务需签发这些字段，生产头像必须是无凭据的 HTTPS 地址。修改资料声明后重新登录；没有资料时显示账号标识及占位头像。

## 接入要求

- 管理域名使用 HTTPS，所有路径回源同一 Go 服务，包括 `/`、`/preview`、`/_nuxt/*`、`/api/*`；HTML、认证和 API 禁止缓存，哈希静态资源可长期缓存。
- COS 直传 CORS：Origin 与 `ADMIN_ALLOWED_ORIGINS` 一致，允许 `PUT`、`HEAD`，放行 `Content-Type`、`X-Amz-Checksum-Sha256`。仅公开素材读取，`snapshots/*` 保持私有。
- Pages 使用 [Direct Upload](https://edgeone.ai/document/173731777508691968) 项目并绑定公开站域名。素材必须是有使用权的稳定 HTTPS 地址；fixture 不用于生产。
- 数据库迁移在启动时自动执行。健康检查为 `GET /healthz`。首次部署验证 OIDC 登录、素材直传、预览、发布与回滚，以及公开站的媒体、`robots.txt`、`sitemap.xml`。

## 更新与故障恢复

- 首次应用 `0009_uuid_ids` 是不兼容的停服升级：先停止所有旧版服务实例和写入流量，再启动新版；迁移成功后不得重启旧二进制。迁移只检查执行时业务表是否为空，不会阻止旧实例之后写入旧格式 ID。`content_versions`、`assets`、`publish_jobs`、`publish_pointer` 或 `publisher_builds` 任一表有记录时，迁移会回滚并拒绝启动，不会删除数据。预发布环境应改用空数据库；迁移会保留审计行并将其主键转为 UUID。该迁移不检查或清理 COS 对象；旧素材对象不会自动关联到新素材记录。升级失败时保持服务停止；回退必须同时恢复升级前的数据库备份和旧二进制。
- 发布请求失败时，用响应中的 `requestId` 查服务日志 `publish request failed`。`stage` 区分版本查询、素材校验、快照读取／写入和发布器提交；外部故障另含白名单 `provider_error_code`、`provider_http_status`，如 `NoSuchBucket`／404、`AccessDenied`／403。`Unknown`／0 表示没有可用分类；日志不记录服务商原文或凭据。版本确实不存在时返回 404，对象存储的桶或上传故障返回服务错误。
- 内容更新由后台保存、审核和发布；程序升级前结束活动发布任务，备份数据库及旧二进制。迁移前置条件见 [安全与运维](../apps/server/README.md#安全与运维)。
- 发布任务固定二进制 SHA-256；旧任务未结束时保留并使用旧程序恢复。云端创建结果不明时先核实 Pages 部署，再使用 [发布恢复接口](../apps/server/README.md#内置-pages-发布与恢复)，不要重复提交。
- Release 准备阶段成功后，版本 commit／tag 保留。后续临时失败用 **Re-run failed jobs**；代码修复后发下一版本，不移动已有标签。准备阶段推送响应丢失时，先核实远端是否已写入。Release 草稿可续传相同文件，已公开文件不会被覆盖。
