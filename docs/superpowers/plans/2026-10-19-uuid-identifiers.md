# UUID 标识实现计划

> **面向 AI 代理的工作者：** 按任务顺序实现；每项行为先写失败测试并确认失败原因。

**目标：** 将本站生成的实体和请求标识统一为带类型前缀的标准 UUIDv4。

**架构：** Go 与 TypeScript 使用各自的密码学随机源生成 UUIDv4，Schema 负责约束带类型前缀的格式。PostgreSQL 审计主键升级为 UUID；迁移在任何不可迁移的旧业务数据存在时拒绝继续，不执行数据删除。

**技术栈：** Go `crypto/rand`、浏览器 Web Crypto、PostgreSQL、JSON Schema、Vitest 和 Go test。

---

### 任务 1：集中 UUIDv4 生成与请求 ID

**文件：**
- 创建：`apps/server/internal/identity/uuid.go`
- 测试：`apps/server/internal/identity/uuid_test.go`
- 修改：`apps/server/internal/httpapi/handler.go`
- 修改：`apps/server/internal/httpapi/handler_routes_test.go`
- 测试：`apps/server/internal/httpapi/handler_test.go`

- [ ] 添加 UUIDv4 生成、格式识别和前缀识别测试。
- [ ] 添加合法代理请求 ID 保留、非法请求 ID 替换、空生成器返回服务不可用的中间件测试。
- [ ] 实现标准格式并移除递增请求 ID 和 `req-unknown` 回退。
- [ ] 运行 `go test ./internal/identity ./internal/httpapi -count=1`。

### 任务 2：版本、素材和发布器任务

**文件：**
- 修改：`apps/server/internal/content/service.go`
- 修改：`apps/server/internal/content/versions.go`
- 修改：`apps/server/internal/assets/service.go`
- 修改：`apps/server/internal/publish/service.go`
- 修改：`apps/server/internal/store/postgres/publisher.go`
- 修改：`apps/server/internal/providers/local/build.go`
- 修改：相关服务、仓储及 provider 测试。

- [ ] 先覆盖各生成服务返回标准带前缀 UUIDv4 的行为。
- [ ] 用共享 Go 标识工具替换 96 位随机 ID、哈希构建 ID 和数字本地构建 ID。
- [ ] 运行对应 Go 包测试和 `go vet`。

### 任务 3：快照 Schema、fixture 与管理端

**文件：**
- 修改：`packages/schema/schema/content-snapshot.schema.json`
- 生成：`packages/schema/src/generated.ts`
- 修改：`packages/schema/test/snapshot.test.ts`
- 修改：`apps/server/internal/contract/schema.json`
- 修改：`apps/server/internal/contract/semantic.go` 及相关测试
- 修改：`content/fixtures/homepage.json`
- 修改：`apps/admin/utils/*-workbench.ts`、`apps/admin/composables/useToasts.ts` 及相关测试
- 修改：`apps/server/internal/publisher/publisher.go` 及相关测试
- 修改：`packages/schema/openapi/admin.yaml` 与 `apps/server/internal/httpapi/openapi_contract_test.go`

- [ ] 添加 UUIDv4 格式的正反例，先确认旧前缀数字 ID 被 Schema 拒绝。
- [ ] 更新各类型 Schema、内部跳转引用、fixture 和所有依赖固定 fixture ID 的测试。
- [ ] 为所有本站 API ID 字段和路径参数补齐 UUIDv4 pattern，并通过契约测试检查；保留外部平台 ID 的不透明字符串契约。
- [ ] 使用 Web Crypto 生成管理端实体与 Toast ID，不提供 `Math.random()` 回退。
- [ ] 运行 `pnpm schema:generate`、Schema／管理端测试、类型检查和 Go Schema 测试。

### 任务 4：数据库迁移和运维说明

**文件：**
- 创建：`apps/server/internal/store/postgres/migrations/0010_uuid_ids.sql`
- 修改：`apps/server/internal/store/postgres/migrate_test.go`
- 修改：`apps/server/internal/store/postgres/migrations` 相关契约测试
- 修改：`apps/server/README.md`、`docs/DEPLOYMENT.md`

- [ ] 测试空业务库迁移的 UUID 主键结果及默认值。
- [ ] 测试任意旧业务数据存在时迁移拒绝且事务回滚。
- [ ] 修改迁移将 `audit_log.id` 转成 UUID；删除递增默认值／序列。
- [ ] 文档说明首次升级需停掉旧实例和写入流量、使用空数据库；明确素材桶不会自动迁移、既有快照关联不能自动转换，回退需同时恢复数据库备份。
- [ ] 运行 `pnpm verify:go`、`pnpm test:coverage:go`、`pnpm typecheck`、`pnpm lint` 和 `pnpm test:coverage`。
