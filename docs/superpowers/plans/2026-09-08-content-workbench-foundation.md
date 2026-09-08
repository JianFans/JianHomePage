# 内容工作台基础增强实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 让管理端在保存前复用 canonical 内容契约完成即时诊断，并提供本地导入、导出和内容摘要。

**架构：** `@yujian/schema` 输出稳定的结构化诊断，同时保留现有路径数组 API。管理端以纯函数模块把编辑文本转换为有效快照、诊断和摘要，组合式函数管理状态，Vue 组件只处理展示及浏览器文件交互。

**技术栈：** TypeScript、Ajv、Vue 3、Nuxt 4、Vitest、`@lucide/vue`、pnpm workspace。

---

## 文件结构

- 修改 `packages/schema/src/validate.ts`：新增结构化 Schema 与语义诊断。
- 修改 `packages/schema/test/snapshot.test.ts`：验证诊断代码、路径和旧 API 兼容性。
- 修改 `apps/admin/package.json`、`pnpm-lock.yaml`：声明共享 Schema 与图标依赖。
- 创建 `apps/admin/utils/snapshot-workbench.ts`：解析、摘要、导入和导出纯函数。
- 创建 `apps/admin/test/unit/snapshot-workbench.test.ts`：覆盖工作台纯函数边界。
- 修改 `apps/admin/composables/useAdminWorkspace.ts`：用完整分析结果控制保存及文件操作。
- 修改 `apps/admin/test/unit/admin-workspace.test.ts`：覆盖有效快照、导入和导出状态。
- 创建 `apps/admin/components/SnapshotInsights.vue`：展示摘要或诊断。
- 创建 `apps/admin/test/unit/snapshot-insights.test.ts`：覆盖双语摘要和诊断界面。
- 修改 `apps/admin/pages/index.vue`：接入工具栏、文件选择、下载与 Insights 组件。
- 修改 `apps/admin/test/unit/admin-shell.test.ts`：覆盖页面入口和浏览器交互。
- 修改 `apps/admin/README.md`、`README.md`：记录内容工作台能力与边界。

### 任务 1：共享结构化诊断

**文件：**
- 修改：`packages/schema/test/snapshot.test.ts`
- 修改：`packages/schema/src/validate.ts`

- [x] **步骤 1：编写失败的结构化诊断测试**

在 Schema 测试中导入 `diagnoseContentSnapshot`，添加以下行为：

```ts
it('为 Schema 和语义错误返回稳定的结构化诊断', () => {
  const missing = structuredClone(fixture)
  delete missing.site.brand
  expect(diagnoseContentSnapshot(missing)).toContainEqual({
    path: '/site/brand',
    source: 'schema',
    code: 'required',
  })

  const brokenReference = structuredClone(fixture)
  brokenReference.releases[0].coverAssetId = 'asset_missing'
  expect(diagnoseContentSnapshot(brokenReference)).toContainEqual({
    path: '/releases/0/coverAssetId',
    source: 'semantic',
    code: 'asset-kind',
  })
})

it('保留路径数组校验 API', () => {
  const invalid = structuredClone(fixture)
  invalid.releases[0].coverAssetId = 'asset_missing'
  expect(validateContentSnapshot(invalid)).toContain('/releases/0/coverAssetId')
})
```

- [x] **步骤 2：运行测试并确认正确失败**

运行：

```bash
pnpm --filter @yujian/schema test -- snapshot.test.ts
```

预期：FAIL，提示 `diagnoseContentSnapshot` 尚未导出。

- [x] **步骤 3：实现最少结构化诊断**

在 `validate.ts` 中定义：

```ts
export type ContentSnapshotIssueSource = 'schema' | 'semantic'

export interface ContentSnapshotIssue {
  path: string
  source: ContentSnapshotIssueSource
  code: string
}

export function diagnoseContentSnapshot(value: unknown): readonly ContentSnapshotIssue[] {
  if (!validateSchema(value)) {
    return (validateSchema.errors ?? []).map(error => ({
      path: formatSchemaIssue(error),
      source: 'schema',
      code: error.keyword,
    }))
  }
  return diagnoseSemantics(value as YujianContentSnapshot)
}

export function validateContentSnapshot(value: unknown): readonly string[] {
  return diagnoseContentSnapshot(value).map(issue => issue.path)
}
```

把语义校验的内部累加值改为 `ContentSnapshotIssue[]`，并通过统一辅助函数加入 `duplicate-id`、`missing-reference`、`asset-kind` 和 `hidden-target`。不要改变原有遍历顺序和校验条件。

- [x] **步骤 4：运行 Schema 测试和类型检查**

```bash
pnpm --filter @yujian/schema test
pnpm --filter @yujian/schema typecheck
```

预期：全部通过，0 失败。

- [x] **步骤 5：提交任务 1**

```bash
git add packages/schema/src/validate.ts packages/schema/test/snapshot.test.ts
git diff --cached --check
git commit --no-gpg-sign -m "feat(内容契约): 添加结构化快照诊断"
```

### 任务 2：编辑文本、摘要和文件边界

**文件：**
- 修改：`apps/admin/package.json`
- 修改：`pnpm-lock.yaml`
- 创建：`apps/admin/utils/snapshot-workbench.ts`
- 创建：`apps/admin/test/unit/snapshot-workbench.test.ts`

- [x] **步骤 1：声明工作区依赖**

在管理端依赖中加入：

```json
{
  "@lucide/vue": "1.41.0",
  "@yujian/schema": "workspace:*"
}
```

运行 `pnpm install --lockfile-only` 更新锁文件。

- [x] **步骤 2：编写失败的工作台纯函数测试**

测试必须读取 `content/fixtures/homepage.json`，并验证：

```ts
const valid = analyzeSnapshotText(JSON.stringify(fixture))
expect(valid.snapshot?.releaseId).toBe('rel_fixture_20260829')
expect(valid.summary).toMatchObject({
  sections: 6,
  releases: 5,
  tracks: 5,
  videos: 3,
  events: 2,
  moments: 3,
  assets: 15,
  previews: 3,
})

expect(analyzeSnapshotText('{').issues[0]?.code).toBe('invalid-json')
expect(analyzeSnapshotText('[]').issues[0]?.code).toBe('object-root')

await expect(readSnapshotImport({
  name: 'snapshot.txt',
  size: 2,
  text: async () => '{}',
})).rejects.toThrow('JSON')

const exported = createSnapshotExport(valid.snapshot!)
expect(exported.filename).toBe('rel_fixture_20260829.json')
expect(analyzeSnapshotText(exported.contents).issues).toHaveLength(0)
```

同时覆盖空文件、超过 `2 * 1024 * 1024` 字节、文件读取失败和不安全 `releaseId` 文件名回退。

- [x] **步骤 3：运行测试并确认正确失败**

```bash
pnpm --filter @yujian/admin test -- snapshot-workbench.test.ts
```

预期：FAIL，提示 `snapshot-workbench.ts` 不存在。

- [x] **步骤 4：实现工作台纯函数**

实现以下稳定接口：

```ts
export interface SnapshotEditorIssue {
  path: string
  source: 'editor' | ContentSnapshotIssueSource
  code: string
}

export interface SnapshotSummary {
  sections: number
  releases: number
  tracks: number
  videos: number
  events: number
  moments: number
  assets: number
  previews: number
}

export interface SnapshotImportFile {
  name: string
  size: number
  text(): Promise<string>
}

export interface SnapshotExport {
  filename: string
  contents: string
  mimeType: 'application/json'
}
```

`analyzeSnapshotText()` 先解析 JSON，再调用 `diagnoseContentSnapshot()`；只有 0 个问题时才返回类型化快照和摘要。`readSnapshotImport()` 只接受大小合规的 `.json` 文件。`createSnapshotExport()` 输出格式化 JSON 和末尾换行。

- [x] **步骤 5：运行工作台测试和管理端类型检查**

```bash
pnpm --filter @yujian/admin test -- snapshot-workbench.test.ts
pnpm --filter @yujian/admin typecheck
```

预期：全部通过，0 失败。

- [x] **步骤 6：提交任务 2**

```bash
git add apps/admin/package.json apps/admin/utils/snapshot-workbench.ts apps/admin/test/unit/snapshot-workbench.test.ts pnpm-lock.yaml
git diff --cached --check
git commit --no-gpg-sign -m "feat(内容工作台): 添加快照分析与文件工具"
```

### 任务 3：接入管理工作区状态

**文件：**
- 修改：`apps/admin/composables/useAdminWorkspace.ts`
- 修改：`apps/admin/test/unit/admin-workspace.test.ts`

- [x] **步骤 1：编写失败的组合式函数测试**

使用完整 fixture 作为有效快照，验证：

```ts
workspace.editorText = JSON.stringify(fixture)
expect(workspace.editorAnalysis.issues).toHaveLength(0)
expect(workspace.canSave).toBe(true)
expect(workspace.exportSnapshot()?.filename).toBe('rel_fixture_20260829.json')

await workspace.importSnapshot({
  name: 'draft.json',
  size: JSON.stringify(fixture).length,
  text: async () => JSON.stringify(fixture),
})
expect(workspace.editorAnalysis.snapshot?.releaseId).toBe('rel_fixture_20260829')

workspace.editorText = '{}'
expect(workspace.canSave).toBe(false)
await workspace.saveDraft()
expect(fetcher).not.toHaveBeenCalled()
```

更新现有草稿流程测试，不再用不完整的 `{ schemaVersion }` 伪快照。

- [x] **步骤 2：运行测试并确认正确失败**

```bash
pnpm --filter @yujian/admin test -- admin-workspace.test.ts
```

预期：FAIL，提示 `editorAnalysis`、`importSnapshot` 或 `exportSnapshot` 不存在。

- [x] **步骤 3：实现状态接入**

用 `analyzeSnapshotText(editorText.value)` 替换仅解析 JSON 的状态。`canSave` 必须依赖完整有效快照；`saveDraft()` 使用分析后的快照。新增：

```ts
async function importSnapshot(file: SnapshotImportFile): Promise<void>
function exportSnapshot(): SnapshotExport | null
```

导入失败写入现有 `workflowError` 状态；导出无效内容时返回 `null`，不触发网络请求。

- [x] **步骤 4：运行管理端单元测试和类型检查**

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
```

预期：全部通过，0 失败。

- [x] **步骤 5：提交任务 3**

```bash
git add apps/admin/composables/useAdminWorkspace.ts apps/admin/test/unit/admin-workspace.test.ts
git diff --cached --check
git commit --no-gpg-sign -m "feat(内容工作台): 接入实时校验状态"
```

### 任务 4：实现摘要与诊断界面

**文件：**
- 创建：`apps/admin/components/SnapshotInsights.vue`
- 创建：`apps/admin/test/unit/snapshot-insights.test.ts`
- 修改：`apps/admin/pages/index.vue`
- 修改：`apps/admin/test/unit/admin-shell.test.ts`

- [x] **步骤 1：编写失败的 Insights 组件测试**

测试合法摘要和错误诊断两种状态：

```ts
expect(validWrapper.get('[data-testid="snapshot-summary"]').text()).toContain('15')
expect(invalidWrapper.get('[data-testid="snapshot-issues"]').text()).toContain('/site/brand')
expect(invalidWrapper.findAll('[data-testid="snapshot-issue"]')).toHaveLength(8)
```

组件 props 使用 `analysis` 和 `locale`，诊断超过 8 项时显示剩余数量。

- [x] **步骤 2：编写失败的页面交互测试**

更新管理端页面测试，验证导入输入、导出按钮、可访问名称和状态标记存在。为下载测试 stub `URL.createObjectURL`、`URL.revokeObjectURL` 和锚点点击，并确认创建的 Blob MIME 为 `application/json`。

- [x] **步骤 3：运行测试并确认正确失败**

```bash
pnpm --filter @yujian/admin test -- snapshot-insights.test.ts admin-shell.test.ts
```

预期：FAIL，提示组件和工具栏尚不存在。

- [x] **步骤 4：实现组件与页面交互**

`SnapshotInsights.vue` 使用数字网格展示摘要，用有序列表展示最多 8 条诊断。页面从 `@lucide/vue` 使用 `FileUp`、`Download` 和 `CircleCheck` 图标，并提供可访问文本。

页面导入流程读取第一个文件并调用 `workspace.importSnapshot()`，随后清空 input value，允许重复选择同一文件。导出流程：

```ts
const exported = workspace.exportSnapshot()
if (!exported) return
const url = URL.createObjectURL(new Blob([exported.contents], { type: exported.mimeType }))
const anchor = document.createElement('a')
anchor.href = url
anchor.download = exported.filename
anchor.click()
URL.revokeObjectURL(url)
```

样式保持低饱和深色；工具栏可换行，图标按钮最小高度为 44 px，诊断路径允许换行且不撑破窄屏。

- [x] **步骤 5：运行管理端验证**

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```

预期：全部通过，0 失败。

- [x] **步骤 6：提交任务 4**

```bash
git add apps/admin/components/SnapshotInsights.vue apps/admin/pages/index.vue apps/admin/test/unit/snapshot-insights.test.ts apps/admin/test/unit/admin-shell.test.ts
git diff --cached --check
git commit --no-gpg-sign -m "feat(内容工作台): 展示摘要与校验诊断"
```

### 任务 5：文档、完整验证与审查

**文件：**
- 修改：`apps/admin/README.md`
- 修改：`README.md`

- [x] **步骤 1：更新文档**

记录即时校验、本地导入导出、摘要字段、2 MiB 限制，以及“客户端检查不能替代服务端最终校验”。根 README 只增加稳定能力入口，详细行为放在管理端 README。

- [x] **步骤 2：运行文档自检**

```bash
rg -n "TODO|待定|稍后补充" README.md apps/admin/README.md docs/superpowers/specs/2026-09-08-content-workbench-foundation-design.md docs/superpowers/plans/2026-09-08-content-workbench-foundation.md
git diff --check
```

预期：没有新增占位符，`git diff --check` 退出码为 0。

- [x] **步骤 3：提交文档**

```bash
git add README.md apps/admin/README.md
git diff --cached --check
git commit --no-gpg-sign -m "docs(内容工作台): 补充快照维护说明"
```

- [x] **步骤 4：运行完整门禁**

```bash
pnpm lint
pnpm typecheck
pnpm test
pnpm test:coverage
pnpm generate
pnpm verify
pnpm verify:go
pnpm test:coverage:go
pnpm --filter @yujian/web test:e2e
pnpm test:automation
git diff --check
```

预期：所有命令退出码为 0，测试 0 失败，生成命令不留下未提交文件。

- [x] **步骤 5：执行 findings-first 自审**

比较 `master...HEAD`，按架构、正确性、安全、性能、可访问性、测试和范围逐项审查。任何 Critical 或 Important 问题必须先修复、补充失败测试并重新运行相关门禁。

- [x] **步骤 6：确认分支状态**

```bash
git status --short --branch
git log --oneline --decorate master..HEAD
git diff --stat master...HEAD
```

预期：工作树干净；提交均为单一职责；不包含发布链路或真实部署改动。
