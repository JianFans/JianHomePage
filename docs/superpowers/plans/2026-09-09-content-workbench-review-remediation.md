# 内容工作台审查问题修复计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 消除异步快照导入竞态，补齐诊断与管理端英文模式的契约和无障碍细节。

**架构：** 管理工作区用单调递增的导入代次保证只有最后一次文件读取可以提交结果，并把导入状态纳入现有 `busy` 门禁。页面只根据组合式函数暴露的状态禁用冲突操作。诊断和语言修复保持在现有文案映射与 Nuxt Head 边界内。

**技术栈：** TypeScript、Vue 3、Nuxt 4、Vitest、Vue Test Utils、Ajv。

---

## 文件职责

- `apps/admin/composables/useAdminWorkspace.ts`：导入代次、导入状态与保存门禁。
- `apps/admin/pages/index.vue`：导入期间的交互禁用、动态页面语言和本地化编辑器名称。
- `apps/admin/components/SnapshotInsights.vue`：补齐 Schema 关键字的双语说明。
- `apps/admin/test/unit/admin-workspace.test.ts`：复现并发导入与导入期间保存门禁。
- `apps/admin/test/unit/admin-shell.test.ts`：验证页面语言、编辑器名称和导入期间禁用状态。
- `apps/admin/test/unit/snapshot-insights.test.ts`：验证 `exclusiveMinimum` 双语诊断。
- `docs/superpowers/specs/2026-09-08-content-workbench-foundation-design.md`：同步稳定语义诊断代码。

### 任务 1：阻止异步导入覆盖新操作

- [x] **步骤 1：编写失败的组合式函数测试**

使用两个可控 Promise 启动连续导入，先完成后发导入，再完成先发导入：

```ts
const first = deferred<string>()
const second = deferred<string>()
const firstImport = workspace.importSnapshot(importFile(first.promise))
const secondImport = workspace.importSnapshot(importFile(second.promise))

second.resolve(secondContents)
await secondImport
first.resolve(firstContents)
await firstImport

expect(workspace.editorText).toBe(secondContents)
```

另一个测试在待处理导入期间断言：

```ts
expect(workspace.importing).toBe(true)
expect(workspace.canSave).toBe(false)
```

- [x] **步骤 2：运行测试验证失败**

运行：

```bash
pnpm --filter @yujian/admin test -- admin-workspace.test.ts
```

预期：旧导入覆盖新内容，且 `importing` 尚不存在。

- [x] **步骤 3：实现导入代次和状态门禁**

在 `useAdminWorkspace()` 内维护单调代次：

```ts
const importing = ref(false)
let importSequence = 0

async function importSnapshot(file: SnapshotImportFile, locale: AdminLocale = 'zh-CN') {
  const sequence = ++importSequence
  importing.value = true
  try {
    const contents = await readSnapshotImport(file)
    if (sequence !== importSequence) return
    editorText.value = contents
    workflow.value = workflowSuccess(locale === 'en' ? 'Snapshot imported' : '已导入快照')
  } finally {
    if (sequence === importSequence) importing.value = false
  }
}
```

错误状态也只允许最新代次写入。`busy` 包含 `importing`，页面在导入期间禁用导入按钮和编辑器。

- [x] **步骤 4：运行窄测试验证通过**

```bash
pnpm --filter @yujian/admin test -- admin-workspace.test.ts admin-shell.test.ts
pnpm --filter @yujian/admin typecheck
```

- [x] **步骤 5：原子提交**

```text
fix(内容工作台): 防止异步导入覆盖新内容
```

### 任务 2：补齐诊断和英文无障碍状态

- [x] **步骤 1：编写失败的组件与页面测试**

诊断组件使用 `exclusiveMinimum`，分别断言中文和英文标签。页面切换英文后断言：

```ts
expect(document.documentElement.lang).toBe('en')
expect(wrapper.get('.json-editor').attributes('aria-label')).toBe('JSON snapshot editor')
```

- [x] **步骤 2：运行测试验证失败**

```bash
pnpm --filter @yujian/admin test -- snapshot-insights.test.ts admin-shell.test.ts
```

预期：诊断回退为通用文案，文档语言和编辑器名称仍为中文。

- [x] **步骤 3：实现最小双语修复**

在诊断映射中加入：

```ts
exclusiveMinimum: 'Value must be above minimum'
exclusiveMinimum: '数值必须大于下限'
```

页面文案加入 `editorLabel`，并通过响应式 `useHead()` 同步 `<html lang>`。

- [x] **步骤 4：运行管理端完整验证**

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```

- [x] **步骤 5：同步契约文档并原子提交**

把 `reference-mismatch` 加入稳定语义代码清单，并标记本计划完成。

```text
fix(内容工作台): 完善双语诊断与语言状态
```

### 任务 3：完整门禁与分支复审

- [x] **步骤 1：运行完整验证**

```bash
pnpm verify
pnpm test:coverage
pnpm --filter @yujian/web test:e2e
pnpm verify:go
pnpm test:coverage:go
pnpm test:automation
```

- [x] **步骤 2：检查提交和工作树**

```bash
git diff --check master..HEAD
git status --short --branch
```

- [x] **步骤 3：复审 `master..HEAD`**

按 findings-first 重新检查正确性、安全边界、契约兼容、i18n、无障碍和真实云环境剩余风险。
