# 发布幂等键交接修复计划

> 执行方式：在现有隔离工作区内联执行，遵循 TDD 与完成前验证流程。

**目标：** 消除 Go 与发布网关的幂等键校验差异，避免非法键冻结版本并永久占用发布槽。

**架构：** Go 在业务副作用之前校验；网关从 Node 的 Latin-1 HTTP 头表示还原 UTF-8 字节。两端保留 UTF-8 键、首尾 Unicode White_Space 归一化和 8–128 字节限制，拒绝控制字符与非法 UTF-8。BOM 不属于 White_Space，作为不透明键的一部分保留。

**技术栈：** Go 标准库、Node HTTP、Node 原生测试。

## 任务 1：复现契约差异

- [x] 在 `apps/server/internal/domain/domain_test.go` 增加长度边界、控制字符、非法 UTF-8、Unicode 空白与 BOM 用例。
- [x] 在 `apps/server/internal/publish/service_test.go` 验证发布与回滚遇到非法键时不修改版本、写快照、创建任务或触发构建。
- [x] 在 `apps/publisher/test/http.test.mjs` 使用真实 HTTP 字节验证 UTF-8 键、128 字节边界、Unicode 空白、BOM 和无效编码。

```powershell
cd apps/server
go test ./internal/domain ./internal/publish -run 'Idempotency|Unsafe' -count=1
cd ../..
node --test apps/publisher/test/http.test.mjs
```

预期：Go 拒绝非法键的断言失败；网关接受合法 UTF-8 边界键的断言失败。

## 任务 2：最小修复与契约同步

- [x] 为 `NormalizeIdempotencyKey` 增加 `utf8.ValidString` 和 `unicode.IsControl` 检查，保留现有字节长度与空白归一化。
- [x] 网关先严格解码 HTTP 头，再按 Unicode White_Space 归一化并校验；不得使用自动移除 BOM 的解码或 trim 行为。

```javascript
const decoded = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
  .decode(Buffer.from(value, 'latin1'))
const key = decoded.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '')
```

- [x] 同步 `packages/schema/openapi/admin.yaml`、Go 生成副本和服务端、发布器 README：限制按 UTF-8 字节计算，不按字符数计算。
- [x] 重新执行任务 1 的窄测试，全部通过后执行完整门禁。

## 任务 3：验证、审查与原子提交

- [x] `pnpm lint`、`pnpm verify:go`、`pnpm --filter @yujian/publisher test:coverage` 通过。
- [x] 使用隔离 PostgreSQL 执行发布器集成测试。
- [x] 对照完整 diff 复核，无用户内容混入；生成副本一致，`git diff --check` 通过。
- [x] 将实现、回归测试与相关契约文档放入同一个职责明确的修复提交。

## 验证记录

- TDD：Go 原实现误接受控制字符与非法 UTF-8；网关原实现未保留 UTF-8 键身份。窄回归测试先失败，最小修复后通过。
- `pnpm lint`、`pnpm verify:go` 通过，后者包含真实 PostgreSQL 测试、生成副本检查和 Go vet。
- 发布器覆盖率门禁：83 项通过，1 项 POSIX 专用测试在 Windows 跳过；行覆盖率 97.11%，函数覆盖率 92.47%。
- 隔离 PostgreSQL 集成测试：2 项通过，包含持久化、锁、HTTP 交接与真实进程健康检查；验证后停止测试实例。
- 独立代码审查及升级边界复核通过；OpenAPI 源与 Go 生成副本一致，diff 无空白错误。
- 本次未执行真实腾讯云发布或 Docker 镜像验证，不据本地门禁推断云环境上线成功。

## 交付边界

不修改已存在任务的键或状态，不重试不明确的云端创建。已经受影响的 pending 任务仍需核对服务商是否受理；不得直接释放版本或修改数据库来跳过该核对。真实云集成和容器执行结论独立于本次本地验证。

升级前暂停新发布与回滚，在旧镜像下确认 Go 与网关两端都无活动任务，并核对原构建关联。旧网关把 UTF-8 头按 Latin-1 字符持久化；若旧构建已经受理而 Go 仍 pending，跨版本自动重试可能产生第二个逻辑构建。未完成核对不得升级，误升级后先停止 Go 自动对账，再恢复旧镜像核对；不增加存在身份歧义的自动键别名或迁移逻辑。
