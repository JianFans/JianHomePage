# Pages 自动发布进程

`apps/publisher` 将 Go 内容服务的发布任务转换为腾讯云 EdgeOne Pages Direct Upload。它从 COS/S3 读取不可变快照，在固定代码版本上生成公开站，再使用官方 `@edgeone/makers-sdk` 上传。管理端无需手动打包；原有发布、刷新与回滚按钮沿用 Go API。

## 部署结构

```text
管理端 → Go API → 私有 HTTPS 发布网关 → PostgreSQL 任务记录
                                      → COS 快照 → Nuxt 构建
                                      → Pages API → yujian.me
```

Go API 保留审核、RBAC、版本冻结、不可变快照、发布指针和业务审计。发布进程保留上传任务、代码身份和异常恢复记录。两个服务使用同一个 PostgreSQL 数据库与 schema；公开站仍是纯静态页面。发布进程的连接必须直连 PostgreSQL 或使用会话池，不能使用会切换底层连接的事务池，否则会话级 advisory lock 无法保证互斥。

Pages 官方 SDK 使用 **Pages API Token**，不是腾讯云通用 SecretId/SecretKey 签名。COS 读取使用独立 S3 兼容凭据。内部网关 Token、Pages Token 和 COS 密钥各有职责，不能混用。

## 首次部署

1. 在 Pages 控制台创建 **Direct Upload** 项目，记录项目 ID、账号地域和 API Token。Git 集成项目不能直接替代 Direct Upload 项目。绑定 `yujian.me`、配置 TLS、关闭 SPA fallback，确认域名已完成服务商要求的接入条件。
2. 先部署本分支 Go API，让启动迁移创建 `0008_publisher_builds`。该迁移只增加独立任务表，不改已有版本。为发布进程授予该表的查询、插入和更新权限，不需要给它 DDL 权限。
3. 为发布进程准备 COS 账号，只允许目标桶 `snapshots/*` 的 `GetObject`。与 Go API 使用相同桶和地域，但可以使用权限更小的独立密钥。
4. 在经过 CI 的干净提交上构建发布镜像。代码身份写入镜像，不能在运行中修改源码、安装依赖或挂载另一个源码目录。

```powershell
$publisherRevision = git rev-parse HEAD
docker build --file apps/publisher/Dockerfile --build-arg "CODE_REVISION=$publisherRevision" --tag yujian-publisher:local .
```

5. 由服务器密钥管理或运维配置生成 `/etc/yujian/publisher.env`，仅在容器运行时注入下表变量。该文件不得进入仓库、镜像或备份中的公开目录。

```bash
docker run --detach --name yujian-publisher --restart unless-stopped \
  --env-file /etc/yujian/publisher.env \
  --publish 127.0.0.1:8091:8091 yujian-publisher:local
```

6. 在服务器反向代理上提供发布网关的 HTTPS 地址，将请求转发至 `127.0.0.1:8091`。只允许 Go API 与运维来源访问；不要将这些接口转发到公开站 `/api/*` 或浏览器。关闭代理对发布响应的缓存，限制请求正文至 16 KiB，保留 `Authorization` 和 `Idempotency-Key`。
7. Go API 设置 `EDGEONE_TRIGGER_URL` 和 `EDGEONE_STATUS_URL` 为同一个 HTTPS `/builds` 地址，`EDGEONE_TOKEN` 与发布进程的 `PUBLISHER_TOKEN` 一致。重启 Go API 后从管理端选择已审核版本并发布。

发布镜像包含构建工具和开发依赖，独立于精简的 Go API 镜像。应为 Nuxt 生成预留足够 CPU、内存和临时磁盘。数据库保存任务，不依赖容器本地文件恢复；每个容器使用独立可写源码层，**不要共享构建输出卷**。整个项目只保留这一条自动生产发布入口，避免手工上传或 Git 自动部署绕过发布编排。

## 环境变量

| 变量 | 要求与用途 |
| --- | --- |
| `PUBLISHER_DATABASE_URL` | 必填，已有内容服务的 PostgreSQL 数据库与 schema。公网连接必须按数据库提供商要求配置 TLS。 |
| `PUBLISHER_TOKEN` | 必填，至少 32 字符的随机内部 Bearer 密钥，不得与 Pages Token 相同。 |
| `PAGES_API_TOKEN` | 必填，腾讯云 Pages API Token，仅发布进程可见。 |
| `PAGES_PROJECT_ID` | 必填，Direct Upload 项目 ID。 |
| `PAGES_REGION` | 必填，`china` 或 `global`，必须匹配 Token 所属账号站点。 |
| `PAGES_PUBLIC_URL` | 必填，稳定公开 HTTPS 地址，例如 `https://yujian.me/`，不含凭据、查询或片段。成功后向 Go 返回该地址，不返回带签名的预览地址。 |
| `PUBLISHER_CODE_REVISION` | 容器已内置，完整 40 位 Git SHA；启动时必须与镜像代码标记匹配。 |
| `S3_ENDPOINT` | 必填，COS/S3 的 HTTPS 端点。 |
| `S3_REGION` | 必填，对象存储地域。 |
| `S3_BUCKET` | 必填，与 Go API 写入审核快照的桶相同。 |
| `S3_ACCESS_KEY_ID` | 必填，快照读取账号的密钥 ID。 |
| `S3_SECRET_ACCESS_KEY` | 必填，快照读取密钥。 |
| `S3_SESSION_TOKEN` | 临时凭据需要时提供。 |
| `S3_USE_PATH_STYLE` | 默认 `false`，只接受 `true` / `false`。 |
| `PUBLISHER_HOST` | 本地默认 `127.0.0.1`，镜像内默认 `0.0.0.0`。 |
| `PUBLISHER_PORT` | 默认 `8091`。 |
| `PUBLISHER_SOURCE_ROOT` | 默认仓库根目录，容器内为 `/workspace`。生产不要覆盖。 |
| `PUBLISHER_WORK_ROOT` | 默认系统临时目录中的 `yujian-publisher`，必须位于源码目录之外。 |
| `PUBLISHER_BUILD_TIMEOUT_MS` | 默认 `600000`，读取快照、只读产物扫描与构建命令共用总预算，可配置范围为 10000–1800000 ms。 |

发布镜像使用 Node.js 25。Node.js 25 不再内置 Corepack，镜像构建时通过 npm 将根 `package.json` 的 `packageManager` 指定版本安装到独立的 `/opt/pnpm`，再执行 `pnpm install --frozen-lockfile`。发布进程直接执行工作区安装的固定版本 pnpm JavaScript CLI，禁用脚本执行前的自动依赖安装；不依赖 root 用户的 Corepack 缓存，也不会在发布时下载构建工具。运行时只执行快照路径守卫、Schema 类型生成、公开站生成和静态产物检查；完整 lint、类型、测试和覆盖率由 CI 在镜像发布前执行。上传目录不携带根 `edgeone.json` 的安装命令，Pages 不再次构建。

快照最多 8 MiB，逐字节核对 SHA-256 和内嵌 `releaseId`。开发 fixture 路径及已知样例媒体引用会被拒绝；已知样例文件也会从上传产物移除。上线前需在管理端换成已登记权利与稳定公开地址的真实素材。

截止时间到达后停止后续构建步骤，但已经开始的文件写入、删除与快照清理必须结束后才能释放构建锁，以免下一次构建遭到延迟写入。只支持独占本地文件系统，不支持网络共享构建目录。

## 网关契约

所有响应 `Cache-Control: no-store`，错误不返回 SDK、数据库或构建日志。除 `GET /healthz` 外均需 `Authorization: Bearer`。浏览器 CORS 不开放。

| 接口 | 行为 |
| --- | --- |
| `POST /builds` | JSON 为 `releaseId`、`snapshotKey`、`snapshotChecksum`；携带 8–128 字节的 `Idempotency-Key`，返回 202 与稳定构建 ID。 |
| `GET /builds/{id}` | 返回 Go 所需 `id/status/previewUrl/error`，以及运维用 `phase/deploymentId`。 |
| `POST /builds/{id}/reconcile` | 只用于结果不明的提交，需核对备注 `note`，绑定 `deploymentId` 或明确 `confirmedAbsent: true`。 |
| `GET /healthz` | 检查数据库及任务表可读；不验证 COS、Pages、域名或真实发布成功。 |

幂等请求固定原来的快照和项目配置；同一键换快照返回 409。另一个活动任务存在时也返回 409。任务状态查询不会触发构建，worker 每 5 秒尝试对账；Go API 自己每 15 秒对账并在成功后切换版本指针。

`Idempotency-Key` 按 UTF-8 字节传输。网关严格解码后，与 Go 一样去除首尾 Unicode White_Space，校验 8–128 个 UTF-8 字节并拒绝控制字符；BOM（U+FEFF）保留，不改变不透明键的身份。非法编码或非法键返回 400，不创建任务。升级不会改写已有任务的键或状态；历史 `pending` 任务若已受旧校验差异影响，需先核对网关与服务商是否受理，不能直接换键重试或释放活动槽位。

## 超时、恢复与升级

`queued/building` 可以从原快照重建。上传前先持久化 `submitting`，得到云端 ID 后转为 `deployed`；该阶段以后只查询原部署。Pages `Success` 才是 `succeeded`；其他明确终态为 `failed`，查询暂时失败或出现未知状态则继续保留活动任务。

官方创建接口没有可依赖的幂等保证。`submitting` 中断、超时或上传出错后，网关会显示 `Deployment outcome unknown; reconciliation required`，**不会自动重复上传或释放活动槽位**。

运维先在 Pages 控制台核对项目、Production 环境、创建时间和构建内容。确定部署 ID 后，在受保护的网关调用恢复接口。将示例中的 `deployment_example` 替换为核实的真实 ID：

```json
{
  "deploymentId": "deployment_example",
  "note": "已核对项目、环境、时间与快照内容"
}
```

网关会再次查询并验证项目、环境和 ID，保留备注和时间，恢复自动对账。由于云端不提供本系统幂等键，内容对应关系仍需运维确认，不能仅挑选列表中最新部署。

只有确认没有任何部署被创建、服务商也不会继续处理旧请求后，才能提交：

```json
{
  "confirmedAbsent": true,
  "note": "已核实旧请求没有创建且不会再创建部署"
}
```

该操作标记构建失败，Go 下次对账释放版本锁；再从管理端以新的幂等键发布。请求仍在执行时恢复接口返回 409，应等待当前操作退出后再核对。不要直接改 SQL 状态、删任务或重复使用失败任务的键。

项目、地域、公开地址和代码 SHA 都冻结在任务中。升级镜像前暂停新的发布和回滚操作，并在旧镜像与原配置下确认 Go 侧没有 `pending/building` 任务，网关也没有活动任务。仅确认网关已终态不够：Go 若因响应丢失仍为 `pending`，必须先核对并恢复原构建关联，不能在新镜像启动后依赖自动重试。未完成核对时不得升级；已升级的实例应先停止 Go API 的自动对账，再恢复旧镜像和配置核对旧任务。

尤其是从旧版 Latin-1 幂等键存储升级到 UTF-8 校验的实例，已有非 ASCII 键不能直接跨版本重试，否则可能创建第二个逻辑构建。本次修复不自动迁移已有键或状态；遇到无法完成的历史任务应保留记录，先核对网关与服务商受理结果。回滚操作重新构建历史快照，默认使用当前经过验证的代码版本；如果需要回滚代码，先部署对应旧镜像再发起内容回滚。

## 验证与真实云验收

```bash
pnpm --filter @yujian/publisher test
pnpm --filter @yujian/publisher test:coverage
docker compose -f apps/publisher/compose.test.yaml up --detach --wait
```

为测试进程配置 `YUJIAN_TEST_POSTGRES_URL` 指向上述测试数据库，再运行 `pnpm --filter @yujian/publisher test:integration`。Compose 账号密码均为 `publisher_test`，数据库同名，端口 `15432`；只监听本机。测试创建随机临时 schema，验证真实约束、锁、HTTP 到 worker 的交接和实际发布进程健康检查，不调用腾讯云写 API。

CI 自动启动独立 PostgreSQL 运行集成测试，并以非 root 用户、禁用网络运行发布镜像的真实静态构建演练。该演练只使用测试快照与测试媒体 URL，不上传、不代表内容审核通过。`test/build.integration.mjs` 仅可对显式 `YUJIAN_TEST_BUILD_SOURCE` 指定的隔离源码执行，会替换该目录的 Nuxt 输出；不要指向正在运行的开发工作区。

普通单测覆盖数据库断线、云请求不明、配置漂移、流取消、子进程失败和 SDK 状态。发布进程生产代码行与函数覆盖率门槛为 80%，启动装配代码通过真实进程健康检查验证。Linux 构建使用进程组终止异常子进程；Windows 本地验证使用 `taskkill` 尽力清理，生产部署使用上述 Linux 镜像。

真实上线还需确认：Pages Token 权限与地域、COS 读取、内部 HTTPS、域名/TLS、首页和媒体访问、缓存、robots/sitemap、首次发布与一次内容回滚。尚未完成这些云环境验收时，不能把本地测试通过视为已经上线。

官方参考：[SDK 概览](https://pages.edgeone.ai/document/sdk-overview)、[部署接口](https://pages.edgeone.ai/document/deployment)、[Direct Upload](https://pages.edgeone.ai/document/direct-upload)。
