# 稳定素材公开 URL 修复计划

> **执行要求：** 按 TDD 顺序逐项实现；每个行为先证明当前实现失败，再补最小修复并运行窄测试。

**目标：** 让素材公开 URL 从上传创建、PostgreSQL 持久化、API 响应到发布快照校验保持同一个稳定值，并为已有素材提供安全迁移。

**架构：** `BlobStore.PublicURL` 只在素材创建和旧数据迁移时解析服务商配置，解析结果作为领域数据写入 `AssetRecord.SourceURL`。发布服务比较快照 `assets[].src` 与已持久化地址，不因后续更换对象存储适配器或配置而重算历史地址。

**技术栈：** Go、PostgreSQL、JSON-compatible OpenAPI、Node.js/pnpm 门禁。

---

## 文件结构

- `apps/server/internal/store/postgres/repository*`：读写 `source_url`。
- `apps/server/internal/store/postgres/migrate*`、`migrations/0004_asset_source_url.sql`：在事务中回填旧素材并添加扩展阶段检查。
- `apps/server/internal/assets/service*`：重试完成操作时保留已存地址。
- `apps/server/internal/publish/service*`：发布校验优先使用已存地址。
- `apps/server/cmd/api/main*`：使用对象存储端口为迁移解析旧素材地址。
- `packages/schema/openapi/admin.yaml`：明确 `Asset.src` 的稳定地址语义，再通过生成命令同步服务端副本。
- `README.md`、`apps/server/README.md`、`apps/web/EDGEONE.md`：记录地址稳定性和迁移前置条件。

### 任务 1：证明 PostgreSQL 丢失公开 URL

1. 在仓储测试中创建包含 `SourceURL` 的素材，断言 INSERT、SELECT、UPDATE 和行扫描都包含 `source_url`。
2. 运行 `go test ./internal/store/postgres -count=1`，确认测试因当前 SQL 未持久化字段而失败。
3. 修改仓储 SQL 与扫描逻辑，重新运行窄测试并确认通过。

### 任务 2：迁移已有素材

1. 添加迁移测试，断言 `0004` 在同一事务中锁定缺少地址的素材、经 `BlobStore.PublicURL` 解析后回填，并拒绝非 `NULL` 的空字符串。
2. 运行 PostgreSQL 迁移窄测试，确认迁移文件和回填行为缺失。
3. 添加 `0004_asset_source_url.sql` 与迁移回填钩子；解析失败或并发更新时回滚并拒绝启动。为兼容滚动升级和旧二进制回滚，本版本不设置 `NOT NULL`；旧实例退出后的后续 contract 迁移负责再次回填并收紧约束。
4. 在生产组合根先构造 `BlobStore`，再将其 `PublicURL` 能力注入迁移器。
5. 运行 PostgreSQL 与 `cmd/api` 窄测试并确认通过。

### 任务 3：固定服务和发布语义

1. 添加完成上传重试测试：提供商当前返回的新地址不能覆盖记录中已有的稳定地址。
2. 添加发布测试：快照地址与持久化地址一致时，即使当前提供商解析结果不同也应继续校验素材实体并发布。
3. 运行 `go test ./internal/assets ./internal/publish -count=1`，确认当前实现因重算地址而失败。
4. 最小修改服务逻辑，只有旧记录缺少地址时才兼容性解析；生产迁移完成后记录始终非空。
5. 重新运行窄测试并确认通过。

### 任务 4：同步契约与文档

1. 在 OpenAPI 契约测试中断言 `Asset.src` 必填，并保留 HTTPS 与本地 `/media/*` 两类格式。
2. 更新 canonical OpenAPI 描述，运行 `pnpm schema:generate` 和 `pnpm verify:go` 同步生成副本。
3. 更新部署文档，说明 `0004` 使用当前稳定媒体基址回填旧记录；迁移期间不得切换公开媒体域名。

### 任务 5：验证、复审与提交

1. 运行 `pnpm verify`、`pnpm test:coverage`、`pnpm test:coverage:go`、`pnpm verify:go`、Web E2E 和 `pnpm test:automation`。
2. 在 Docker 可用时构建镜像并检查 `/healthz`；不可用时记录准确外部阻塞。
3. 运行 `git diff --check`，对本次 diff 与上线基线执行 findings-first 复审。
4. 修复所有 Critical/Important 问题并重新运行受影响门禁。
5. 按实现、契约文档职责创建中文 Conventional Commits，确认 `master` 工作区干净。
