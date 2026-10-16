# EdgeOne Pages 自动发布设计

> 历史方案：独立 Node 发布程序已由 [Go 单程序设计](2026-10-16-go-all-in-one-design.md) 替代。当前部署请使用 [部署指南](../../DEPLOYMENT.md)。下文保留原设计背景，不作为现行运行方式。

## 目标与边界

管理端沿用现有审核、发布和回滚操作。Go 服务冻结版本并写入 COS 不可变快照；新增 `apps/publisher` 执行静态构建和 Pages Direct Upload。首页不增加运行时内容 API，腾讯云凭据不进入浏览器或静态产物。

继续使用现有 `BuildTrigger` HTTP 契约，避免将官方 SDK 类型引入 Go 领域层。相比把 Node 构建器嵌入 API 容器，此方案隔离构建资源和部署权限；相比 GitHub Actions 触发器，不引入额外的 GitHub 凭据和调度依赖。

## 发布过程

1. Go 服务以 `Idempotency-Key` 提交 `releaseId`、`snapshotKey`、`snapshotChecksum`。
2. 发布进程将请求、Pages 项目、地域和代码版本写入 PostgreSQL。重复键返回同一任务，重复键携带不同请求返回冲突。
3. 单个持有 PostgreSQL advisory lock 的 worker 从受信任的固定代码目录构建。COS 读取限制在 `snapshots/<releaseId>/<checksum>.json`，核对字节 SHA-256 和快照 releaseId。
4. 向构建子进程仅传递必要环境变量和绝对快照路径；运行快照守卫、Schema 生成、Nuxt 静态生成和既有静态产物检查。每个构建独占代码输出目录，构建前清理旧产物。
5. 通过 `@edgeone/makers-sdk` 的 `deployments.deploy` 上传静态目录，指定 `Production`，持久化部署 ID，再通过 `deployments.get` 对账。
6. SDK `Success` 才返回 `succeeded`。Go 服务继续负责线上指针和审计；构建失败返回 `failed`。

## 持久化与故障恢复

新增 `publisher_builds` 表，由 Go 迁移统一创建。worker 使用单个专用数据库连接持有全局锁；连接丢失即终止进程，避免失去锁后继续上传。已接受请求不依赖 HTTP 连接存活。

阶段为 `queued`、`building`、`submitting`、`deployed`、`succeeded`、`failed`。进程重启后可以重建 `queued/building`；`deployed` 只查询已有部署。官方 API 没有明确的创建幂等保证，调用 SDK 前先持久化 `submitting`。该阶段中断或超时后保持 `building` 并提示人工核对，不盲目重新上传。经过认证的恢复接口仅允许为该阶段绑定已核实的 Production 部署 ID，或在核实没有创建部署后明确标记失败；绑定操作检查项目、环境和部署标识，并留存恢复记录。

代码版本和项目配置保存在任务中。重启后发现配置漂移时停止执行该任务并保留活动槽位，要求恢复原配置，不能拿新代码静默重建旧请求。

## 安全与运行

发布进程与 Go API 分开运行。Go 的 `EDGEONE_TOKEN` 是内部网关共享密钥；`PAGES_API_TOKEN` 才是腾讯云 Pages API Token，两者必须不同。内部接口经 HTTPS 反向代理访问，不公开到浏览器。HTTP 无 CORS，限制请求大小、超时，所有错误脱敏。

生产使用经过 CI 的固定代码镜像、固定依赖锁和非 root 用户。pnpm 作为发布进程的显式依赖安装，运行时直接执行其 CLI 并禁用自动安装，不依赖 Corepack 下载或 root 缓存。构建目录使用独占本地存储，不与本地开发站或其他容器共享；同一项目不并行运行其他发布入口。COS 账号只需快照读取权限。数据库和 COS、Pages 密钥只在发布进程中使用，构建子进程不继承这些变量。

## 验证

使用 Node 原生测试覆盖请求契约、重复键、冲突、重启恢复、提交结果不明、状态映射、配置漂移、快照大小及 SHA-256、子进程隔离和构建失败。PostgreSQL 集成测试验证真实唯一约束与 advisory lock；CI 提供独立 PostgreSQL 服务。现有 Go、前端和静态门禁继续运行。

真实云验收需要 Pages Direct Upload 项目 ID、Token、地域、COS 读取配置及 HTTPS 网关。没有云账号时仅声明本地契约、构建和测试结果。
