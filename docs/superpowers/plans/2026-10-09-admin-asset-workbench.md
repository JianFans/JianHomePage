# 管理端素材工作台实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让编辑人员在管理端完成素材查询、文件直传、完成确认和 canonical 快照插入闭环。

**架构：** Go 服务为素材记录提供稳定的游标分页查询，内存和 PostgreSQL 仓储保持相同排序语义。管理端把文件约束、SHA-256、服务端签名上传和快照转换拆成纯函数、API 客户端和组合式函数；Vue 组件只负责表单、列表和可访问状态展示。

**技术栈：** Go 1.25、PostgreSQL、OpenAPI 3.1、TypeScript、Vue 3、Nuxt 4、Vitest、Vue Test Utils、@noble/hashes、Playwright。

---

## 文件职责

- `apps/server/internal/assets/service.go`：素材列表查询、状态过滤和游标编码。
- `apps/server/internal/store/memory/store.go`：内存素材列表实现。
- `apps/server/internal/store/postgres/repository.go`：PostgreSQL keyset 分页查询。
- `apps/server/internal/httpapi/handler.go`：素材列表 HTTP 路由与响应。
- `packages/schema/openapi/admin.yaml`：管理 API 规范源文件。
- `apps/admin/utils/admin-api.ts`：素材管理 API 和签名直传客户端。
- `apps/admin/utils/asset-workbench.ts`：文件预检、SHA-256 和快照插入纯函数。
- `apps/admin/composables/useAssetWorkspace.ts`：上传状态机、分页和筛选状态。
- `apps/admin/components/AssetWorkbench.vue`：素材上传与素材库界面。
- `apps/admin/pages/index.vue`：连接素材工作台与当前 JSON 编辑器。
- 对应测试文件：覆盖 Go 服务、仓储、HTTP、OpenAPI、TypeScript、Vue 与本地闭环。

## 任务 1：实现服务端素材分页查询

**文件：**

- 修改：`apps/server/internal/assets/service.go`
- 修改：`apps/server/internal/assets/service_test.go`
- 修改：`apps/server/internal/store/memory/store.go`
- 修改：`apps/server/internal/store/memory/store_test.go`
- 修改：`apps/server/internal/store/postgres/repository.go`
- 修改：`apps/server/internal/store/postgres/repository_test.go`

- [x] **步骤 1：编写失败的服务分页测试**

使用固定时间和 `asset_a`、`asset_b`、`asset_c` 测试默认排除删除素材、显式状态筛选、`CreatedAt DESC, ID DESC` 排序和下一页游标：

```go
page, err := service.List(context.Background(), editor(), ListOptions{Limit: 2})
if err != nil {
    t.Fatal(err)
}
if got := assetIDs(page.Items); !slices.Equal(got, []string{"asset_c", "asset_b"}) {
    t.Fatalf("unexpected first page %v", got)
}
if page.NextCursor == "" {
    t.Fatal("expected next cursor")
}
```

无效 Base64URL 和缺少 ID 的游标必须返回 `domain.ErrInvalidInput`。

- [x] **步骤 2：运行测试验证失败**

```bash
cd apps/server
go test ./internal/assets -run TestList -count=1
```

预期：FAIL，提示列表类型或方法不存在。

- [x] **步骤 3：实现查询类型与游标**

```go
type ListQuery struct {
    Statuses        []domain.AssetStatus
    BeforeCreatedAt *time.Time
    BeforeID        string
    Limit           int
}

type ListOptions struct {
    Status domain.AssetStatus
    Limit  int
    Cursor string
}

type ListPage struct {
    Items      []domain.AssetRecord
    NextCursor string
}
```

`Repository` 增加 `ListAssets(context.Context, ListQuery)`。服务默认查询 `pending` 与 `ready`，默认限制 50，最大 100。游标使用 `base64.RawURLEncoding` 编码 `{createdAt,id}`，仓储读取 `limit + 1` 条记录。

- [x] **步骤 4：实现内存与 PostgreSQL keyset 分页**

内存实现复制后排序。经性能修复后，PostgreSQL 默认未删除素材的深页查询使用：

```sql
SELECT id, blob_key, source_url, status, metadata, rights,
       created_by, created_at, deleted_at
FROM assets
WHERE status IN ('pending', 'ready')
  AND (created_at, id) < ($1, $2)
ORDER BY created_at DESC, id DESC
LIMIT $3
```

第一页不添加游标条件。单状态筛选使用 `status = $1`，其他仓储状态组合使用 `status = ANY($1)`，游标和 LIMIT 参数序号随之调整。查询不保留可空游标的 `OR` 分支；默认谓词匹配 `0005_asset_list_indexes.sql` 的部分索引，单状态查询使用状态复合索引。

- [x] **步骤 5：运行窄测试**

```bash
cd apps/server
go test ./internal/assets ./internal/store/memory ./internal/store/postgres -count=1
```

- [x] **步骤 6：原子提交**

```text
feat(素材服务): 添加素材分页查询
```

## 任务 2：暴露素材列表 HTTP 与 OpenAPI 契约

**文件：**

- 修改：`apps/server/internal/httpapi/handler.go`
- 修改：`apps/server/internal/httpapi/handler_test.go`
- 修改：`apps/server/internal/httpapi/handler_routes_test.go`
- 修改：`apps/server/internal/httpapi/openapi_contract_test.go`
- 修改：`packages/schema/openapi/admin.yaml`
- 生成：`apps/server/internal/httpapi/openapi.yaml`

- [x] **步骤 1：编写失败的路由与契约测试**

扩展 `assetServiceStub`：

```go
listFn func(context.Context, domain.Principal, assets.ListOptions) (assets.ListPage, error)
```

请求 `GET /api/v1/assets?status=ready&limit=25&cursor=cursor-value`，断言服务收到规范化参数。无效状态、`limit=0` 和 `limit=101` 返回 `400 invalid_request`。OpenAPI 测试断言新路径与 3 个查询参数存在。

- [x] **步骤 2：运行测试验证失败**

```bash
cd apps/server
go test ./internal/httpapi -run 'TestListAssets|TestOpenAPI' -count=1
```

- [x] **步骤 3：实现端点和响应结构**

注册：

```go
register("GET /api/v1/assets", auth.PermissionCreateAsset, handler.listAssets)
```

响应：

```go
type assetListResponse struct {
    Items      []assetResponse `json:"items"`
    NextCursor string          `json:"nextCursor,omitempty"`
}
```

- [x] **步骤 4：更新规范并生成副本**

更新 `packages/schema/openapi/admin.yaml` 后运行：

```bash
cd apps/server
go generate ./...
```

不得手工编辑生成副本。

- [x] **步骤 5：运行验证并提交**

```bash
cd apps/server
go test ./internal/httpapi -count=1
cd ../..
pnpm verify:go
```

提交：

```text
feat(管理接口): 添加素材列表端点
```

## 任务 3：实现管理端素材纯函数与 API 客户端

**文件：**

- 创建：`apps/admin/utils/asset-workbench.ts`
- 创建：`apps/admin/test/unit/asset-workbench.test.ts`
- 修改：`apps/admin/utils/admin-api.ts`
- 修改：`apps/admin/test/unit/admin-api.test.ts`

- [x] **步骤 1：编写失败的文件和快照测试**

测试允许的 MIME、扩展名、大小上限、空文件、标准 SHA-256 和重复素材 ID。合法 fixture 插入后必须继续通过 `analyzeSnapshotText()`。

```ts
expect(validateAssetFile(webpFile)).toMatchObject({
  kind: 'image',
  contentType: 'image/webp',
  maxBytes: 20 * 1024 * 1024,
})
```

- [x] **步骤 2：运行测试验证失败**

```bash
pnpm --filter @yujian/admin test -- asset-workbench.test.ts
```

- [x] **步骤 3：实现纯函数**

```ts
export function validateAssetFile(file: Pick<File, 'name' | 'size' | 'type'>): AssetFileRule
export async function sha256File(file: Blob): Promise<string>
export function toSnapshotAsset(asset: AdminAsset, alt: LocalizedDraft): Asset
export function insertSnapshotAsset(text: string, asset: Asset): SnapshotInsertResult
```

`Asset` 来自 `@yujian/schema`。`sha256File()` 使用 `@noble/hashes` 按 4 MiB 分块读取并输出小写 `sha256:<hex>`，避免 Web Crypto 整文件摘要的内存开销。服务端 metadata 的 `duration` 按纳秒转换为 `durationSeconds`。

- [x] **步骤 4：编写失败的 API 客户端测试**

覆盖列表参数、创建上传 Bearer Token、签名 `PUT` 不携带 `Authorization`、完成确认 URL 编码。

- [x] **步骤 5：实现 API 客户端并验证**

新增：

```ts
listAssets(options?: AssetListOptions): Promise<AdminAssetPage>
createAssetUpload(input: AdminAssetUploadRequest): Promise<AdminAssetUpload>
uploadAssetBlob(upload: AdminAssetUpload, file: Blob): Promise<void>
completeAssetUpload(assetId: string): Promise<AdminAsset>
```

运行：

```bash
pnpm --filter @yujian/admin test -- asset-workbench.test.ts admin-api.test.ts
pnpm --filter @yujian/admin typecheck
```

- [x] **步骤 6：原子提交**

```text
feat(素材工作台): 添加文件校验与上传客户端
```

## 任务 4：实现上传与分页状态机

**文件：**

- 创建：`apps/admin/composables/useAssetWorkspace.ts`
- 创建：`apps/admin/test/unit/asset-workspace.test.ts`

- [x] **步骤 1：编写失败的状态机测试**

依赖注入 API 与摘要函数，断言阶段顺序：

```ts
expect(stages).toEqual([
  'hashing',
  'creating',
  'uploading',
  'completing',
  'succeeded',
])
```

覆盖上传失败保留表单、完成失败可单独重试、分页追加按 ID 去重、切换状态筛选重置游标。

- [x] **步骤 2：运行测试验证失败**

```bash
pnpm --filter @yujian/admin test -- asset-workspace.test.ts
```

- [x] **步骤 3：实现组合式函数**

状态包括上传草稿、阶段、素材数组、下一页游标、服务端状态筛选和本地文本筛选。异步操作使用单调序号，`onScopeDispose()` 后不得写入 Vue 状态。

- [x] **步骤 4：运行测试并提交**

```bash
pnpm --filter @yujian/admin test -- asset-workspace.test.ts
pnpm --filter @yujian/admin typecheck
```

提交：

```text
feat(素材工作台): 添加上传与分页状态机
```

## 任务 5：实现响应式素材工作台界面

**文件：**

- 创建：`apps/admin/components/AssetWorkbench.vue`
- 创建：`apps/admin/test/unit/asset-workbench-component.test.ts`
- 修改：`apps/admin/pages/index.vue`
- 修改：`apps/admin/test/unit/admin-shell.test.ts`
- 修改：`apps/admin/assets/css/main.css`

- [x] **步骤 1：读取 OpenDesign 原型或记录外部阻塞**

优先从 OpenDesign 原型提取布局、间距、层级和状态表达。若 OpenDesign 因账户额度等外部原因未生成产物，则记录失败证据，并以现有 `apps/admin/pages/index.vue` 与 `apps/admin/assets/css/main.css` 为视觉基线继续实现。两种情况下都继续使用现有颜色变量与 `@lucide/vue`，不复制额外框架、字体或依赖。

- [x] **步骤 2：编写失败的组件测试**

覆盖双语字段、文件信息、上传阶段播报、`ready` 插入门禁、已存在素材禁用和加载更多条件。

- [x] **步骤 3：运行测试验证失败**

```bash
pnpm --filter @yujian/admin test -- asset-workbench-component.test.ts admin-shell.test.ts
```

- [x] **步骤 4：实现组件与页面连接**

组件接收 `locale`、`editorText`、`apiBaseUrl`、`token`，并发出：

```ts
const emit = defineEmits<{
  'update:editorText': [value: string]
}>()
```

插入只更新 JSON 文本，不自动保存。桌面两列，`860 px` 以下单列；图片和 GIF 使用固定比例预览，音视频只显示图标和元数据。

- [x] **步骤 5：运行管理端完整验证并提交**

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```

提交：

```text
feat(素材工作台): 添加素材上传与快照插入界面
```

## 任务 6：补充本地闭环与文档

**文件：**

- 修改或创建：现有测试布局下的管理端 E2E／Go HTTP 集成测试。
- 修改：`apps/admin/README.md`
- 修改：`README.md`

- [x] **步骤 1：读取现有 E2E 启动方式**

复用现有端口管理、fixture 和清理流程，不依赖真实 COS、OIDC 或 EdgeOne。

- [x] **步骤 2：先写本地上传闭环测试**

使用小型 WebP fixture 验证：创建签名上传、`PUT /local-upload/*`、完成确认、插入 fixture 快照并保持 canonical 校验。若现有浏览器架构不能安全注入开发身份，则拆成 Go HTTP 集成测试与管理端 API 单元测试，并在文档中明确边界。

- [x] **步骤 3：实现最小测试适配并验证**

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm test:automation
```

- [x] **步骤 4：更新文档并分开提交**

记录支持的 MIME、大小限制、Token 与签名请求头边界、本地上传流程、COS CORS 要求，以及“上传成功不等于发布”。

```text
test(素材工作台): 覆盖本地上传闭环
docs(素材工作台): 补充上传与安全说明
```

## 任务 7：完整门禁与分支复审

**文件：**

- 修改：`docs/superpowers/plans/2026-10-09-admin-asset-workbench.md`

- [x] **步骤 1：运行完整门禁**

```bash
pnpm lint
pnpm typecheck
pnpm test
pnpm test:coverage
pnpm verify:go
pnpm test:coverage:go
pnpm test:automation
pnpm --filter @yujian/web test:e2e
```

Windows 本地 `pnpm generate` 若仍复现已知 Nuxt `ssr: false` renderer 平台问题，必须保留完整错误证据，并以 Linux GitHub CI 的 `Frontend Verify` 验证生成；不得修改生产渲染模式绕过。

- [x] **步骤 2：检查工作树和提交原子性**

```bash
git diff --check master..HEAD
git status --short --branch
git log --oneline master..HEAD
```

- [x] **步骤 3：完整复审分支**

按 findings-first 检查分页稳定性、权限、Token 泄漏、上传重试、快照契约、i18n、键盘、触控、响应式和大文件内存风险。

- [x] **步骤 4：修复 Critical 与 Important 问题并重新验证**

所有修复先添加回归测试，再运行窄测试和完整门禁。

- [x] **步骤 5：记录验证结果并提交**

```text
docs(素材工作台): 记录实现与验证结果
```

## 执行结果

计划于 2026 年 10 月 9 日完成。实现包含素材稳定分页、管理 API、签名直传客户端、上传状态机、响应式素材工作台、快照插入以及本地 HTTP 上传闭环。

### 设计与环境说明

- OpenDesign 在生成阶段返回 `AMR_INSUFFICIENT_BALANCE`，未产生可用原型。本次沿用现有管理端的颜色变量、间距、面板层级和 Lucide 图标，并通过实际浏览器检查桌面、760 px 和 390 px 布局。
- 本机未安装 Playwright revision 1243，官方 Chromium 下载在 CDN 阶段无法取得进度。E2E 使用本机 Google Chrome 通过一次性配置运行，临时配置未提交。
- Nuxt 4.6.0 在 Windows 路径下未命中默认 renderer 内联规则。管理端与公开站显式内联 `nuxt/dist`，配置测试同时覆盖 Windows 和 POSIX 路径；修复后两端静态生成均成功。

### 验证结果

以下命令在首次实现完成时通过，统计对应当时的代码；后续修复记录见文末链接：

```text
pnpm lint
pnpm typecheck
pnpm test
pnpm test:coverage
pnpm verify:go
pnpm test:coverage:go
pnpm test:automation
pnpm --filter @yujian/admin build
pnpm --filter @yujian/admin generate
pnpm --filter @yujian/web generate
pnpm verify:static
```

验证统计：Schema 19 个测试、管理端 68 个测试、公开站 82 个测试、仓库脚本 21 个测试，Go 全包测试与 `go vet` 通过；Go 语句覆盖率为 82.28%，门槛为 80%。使用系统 Chrome 运行的 Playwright 与 axe 测试为 11/11 通过。公开站静态产物包含 18 个有效资源引用，首屏 JavaScript 为 286 KiB。

### 复审结论

分支按权限与 Token 隔离、游标分页、大文件内存、上传失败恢复、完成确认重试、canonical 快照、双语与无障碍、响应式布局和 Nuxt 跨平台构建进行复审。复审发现并修复了 2 个 Important 问题：完成确认重试会丢失替代文本；本地上传适配器会将最大 2 GiB 文件重复读入内存。对应回归测试覆盖重试字段保留、64 KiB 分块写入、大小与 SHA-256 流式校验、取消信号、幂等写入和临时文件清理。最终复审未留下 Critical 或 Important 级别问题。

生产 PostgreSQL 迁移、真实 OIDC、COS CORS、稳定媒体域名和 EdgeOne 路由仍需在部署阶段验证；本计划不据此宣称已经上线。本地 PostgreSQL 后续已完成独立 Schema 的迁移、并发与查询计划验证，不能替代生产迁移冒烟。

### 后续修复与验证边界

- [素材工作台审查修复](2026-10-09-asset-review-remediation.md)：记录上传期限、预览地址、筛选并发、替代文本、稳定地址和分页索引修复。
- [交互与关闭边界修复](2026-10-09-asset-workbench-interaction-remediation.md)：记录表单冻结、API 切换隔离、超时测试、关闭和查询解析修复。
- 后续共享样式、Windows 下载期间删除及 NUL 游标修复分别见提交 `39cdf2a`、`d3141c1` 和 `8f0af60`；测试与实现位于同一职责提交。

本阶段素材上传采用 Go HTTP 集成测试与管理端单元测试分别验证。首次记录中的 11 项 Playwright 与 axe 测试属于公开站，不是管理端完整浏览器上传 E2E。完整管理端上传 E2E、Go race detector 和真实云集成仍是独立验证项，不因本地分层测试通过而视为完成。
