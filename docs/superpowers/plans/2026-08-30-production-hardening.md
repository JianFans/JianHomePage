# 生产上线加固实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框跟踪进度。

**目标：** 修复 `79ed0c1..e23d321` 上线审查发现的生产装配、安全、幂等、一致性和发布门禁问题，使公开站与管理平台具备可部署的代码路径。

**架构：** 保持领域层只依赖 `Repository`、`BlobStore`、`BuildTrigger` 和 `IdentityProvider` 端口。生产组合根使用真实 PostgreSQL 驱动、S3 兼容对象存储适配器和现有 EdgeOne HTTPS webhook；浏览器和静态构建在各自边界 fail closed。

**技术栈：** Go 1.25、PostgreSQL/pgx、S3 兼容对象存储、Nuxt 4、Vue 3、Vitest、Playwright。

---

## 文件结构

- `apps/server/internal/config/*`：生产配置枚举、CORS 与提供商参数。
- `apps/server/internal/auth/oidc*`：OIDC discovery/JWKS 传输约束。
- `apps/server/internal/httpapi/*`：CORS、请求契约与本地上传路由。
- `apps/server/internal/publish/*`：发布幂等语义和可恢复状态机。
- `apps/server/internal/store/postgres/*`：真实事务迁移、错误映射和 SQL 驱动适配。
- `apps/server/internal/providers/s3/*`：S3 兼容 BlobStore。
- `apps/server/cmd/api/*`：生产组合根与资源关闭。
- `apps/admin/*`：稳定操作键与跨重试复用。
- `apps/web/utils/*`、`apps/web/plugins/*`：快照校验和存储异常降级。
- `edgeone.json`、`package.json`：部署前验证门禁。

### 任务 1：配置、认证和 HTTP 边界

- [ ] 在 `config_test.go` 添加未知 `APP_ENV`、缺失生产提供商配置和 CORS origin 校验用例。
- [ ] 运行 `go test ./internal/config`，确认测试因当前宽松配置失败。
- [ ] 在 `oidc_test.go` 添加生产 issuer discovery 指向 HTTP JWKS 时拒绝认证的用例。
- [ ] 运行 `go test ./internal/auth`，确认测试因 JWKS 未继承 HTTPS 约束失败。
- [ ] 在 `handler_test.go` 添加允许来源预检、拒绝未知来源和缺失 `rights` 返回 400 的用例。
- [ ] 运行 `go test ./internal/httpapi`，确认新增用例失败。
- [ ] 最小实现环境枚举、生产配置校验、JWKS HTTPS 和 allowlist CORS。
- [ ] 运行上述三个包测试并确认通过。
- [ ] 提交 `fix(安全边界): 收紧生产配置与跨域认证`。

### 任务 2：发布幂等与并发一致性

- [ ] 在管理端单测中添加相同版本重试复用操作键、成功后新操作生成新键的用例。
- [ ] 运行管理端窄测试，确认当前每次调用新建 UUID 导致失败。
- [ ] 在 Go 发布测试中添加同 key 不同版本、发布与回滚混用、并发唯一冲突恢复用例。
- [ ] 运行 `go test ./internal/publish ./internal/store/postgres`，确认新增用例失败。
- [ ] 为发布任务记录操作类型，校验幂等请求语义，并把 PostgreSQL 唯一约束映射为 `domain.ErrConflict`。
- [ ] 将 EdgeOne 触发拆为可恢复状态：持久化 `triggering` 后触发，保存 build ID 失败时刷新路径可按幂等 release ID 恢复。
- [ ] 运行管理端和 Go 发布相关测试并确认通过。
- [ ] 提交 `fix(发布): 修复幂等键与并发任务一致性`。

### 任务 3：生产基础设施装配

- [ ] 为 PostgreSQL 驱动包装器添加真实 `database/sql` 接口适配测试。
- [ ] 将迁移器测试改为验证 `BeginTx`、事务内 advisory lock、提交和回滚。
- [ ] 运行 PostgreSQL 包测试，确认旧迁移接口不能满足事务语义。
- [ ] 添加 pgx stdlib 驱动和 SQL executor/transaction wrapper，迁移只接受真实事务。
- [ ] 为 S3 兼容 BlobStore 添加签名上传、Put/Stat/Delete/Read URL 与错误映射测试。
- [ ] 实现不向领域层暴露 SDK 类型的 S3 适配器。
- [ ] 在 `cmd/api/main_test.go` 添加生产依赖构造成功、缺失配置 fail closed、关闭资源的用例。
- [ ] 装配 PostgreSQL、迁移、S3 BlobStore 和 EdgeOne Client；生产启动不再传空依赖。
- [ ] 运行 `go test ./...` 和生产配置启动冒烟。
- [ ] 提交 `feat(服务端): 接入可替换生产基础设施`。

### 任务 4：构建、浏览器与本地开发韧性

- [ ] 为静态快照加载器添加缺字段、错误引用和非法 URL 的失败用例。
- [ ] 运行 Web 窄测试，确认构建入口只检查 object 根导致失败。
- [ ] 复用 canonical JSON Schema 校验生成输入；非法快照在 Nuxt 配置加载阶段失败。
- [ ] 添加 localStorage 读写抛出异常时仍完成挂载和语言切换的用例。
- [ ] 使用安全存储辅助函数降级为内存偏好。
- [ ] 为本地上传 PUT 路由添加大小、MIME、checksum 和不存在路径测试，并接入开发 BlobStore。
- [ ] 更新 EdgeOne 构建命令，使部署前执行静态站 lint、类型检查、单测、生成和产物校验。
- [ ] 运行 Web、HTTP API 窄测试并确认通过。
- [ ] 提交 `fix(发布链路): 强化快照校验与开发降级`。

### 任务 5：全量验证与复审

- [ ] 运行 `pnpm verify`。
- [ ] 运行 `pnpm --filter @yujian/web test:e2e`。
- [ ] 运行 `pnpm verify:go`。
- [ ] 使用生产配置启动 API，确认通过依赖构造并在外部依赖不可用时给出安全错误。
- [ ] 运行 `git diff --check` 并清理空白错误。
- [ ] 对计划起点到 HEAD 做完整安全、正确性和上线复审。
- [ ] 修复复审中的 Critical/Important 问题并重新运行全量验证。
- [ ] 提交必要的文档和验证调整。
