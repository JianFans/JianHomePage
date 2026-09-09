# 内容工作台基础增强设计规格

## 1. 背景

「遇健我」已经具备公开静态首页、canonical 内容 Schema、管理 API 和基础管理端。当前管理端仍以原始 JSON 文本框为主，只能判断输入能否解析为对象。Schema 字段错误、跨记录引用错误和素材类型错误要到服务端保存时才能发现，也缺少安全的本地导入、导出和内容概览。

本轮只完善内容维护的基础能力，不设计发布包、EdgeOne 工作流、真实云配置或部署流程。

## 2. 目标

编辑人员应能在不触发发布操作的情况下完成以下任务：

1. 在输入 JSON 时立即获得 canonical Schema 与语义校验结果。
2. 看到错误所属字段路径、问题类别和简短说明。
3. 仅在快照完整有效时保存草稿。
4. 从本地 JSON 文件载入编辑内容，并把当前有效草稿导出为 JSON 文件。
5. 快速确认快照包含的板块、作品、曲目、影像、现场、片段、素材和试听数量。

## 3. 方案选择

### 3.1 采用方案：共享诊断能力与管理端工作台

在 `@yujian/schema` 中增加结构化诊断 API，管理端直接复用同一套 Schema 与语义规则。管理端通过独立纯函数模块完成文本解析、诊断转换、摘要、导入限制和导出描述，Vue 页面只处理文件选择与下载等浏览器交互。

该方案能把错误提前到编辑阶段，同时保持服务端最终校验不变。后续增加内容类型或校验规则时，管理端可以自动获得一致结果。

### 3.2 未采用方案

- **新增公开站详情页：** 用户价值直观，但会提前固化尚未确认的正式内容结构，不解决内容录入效率问题。
- **只增加命令行检查器：** 适合开发者和 CI，但编辑人员仍需在 JSON 与终端错误之间来回定位。
- **完整可视化表单编辑器：** 长期价值高，但需要为每种内容类型设计表单、排序和关联选择器，本轮范围过大。

## 4. 范围

### 4.1 包含

- 结构化快照诊断，区分 Schema 与语义问题。
- 向后兼容现有 `validateContentSnapshot()` 与 `assertContentSnapshot()`。
- 管理端实时校验状态、错误列表和内容摘要。
- 最大 2 MiB 的本地 `.json` 文件导入。
- 合法快照导出，文件名优先使用安全化后的 `releaseId`。
- 中文和英文管理端界面文案。
- 单元测试、组件测试、类型检查、构建和仓库完整门禁。

### 4.2 不包含

- 修改内容 Schema 或生成类型。
- 新增首页板块、详情页或访客功能。
- 可视化字段表单、拖拽编排或素材上传。
- 自动保存、浏览器持久化草稿或多人协作。
- 发布、回滚、EdgeOne、对象存储或真实部署能力。

## 5. 共享诊断契约

`@yujian/schema` 新增以下公开类型和函数：

```ts
export type ContentSnapshotIssueSource = 'schema' | 'semantic'

export interface ContentSnapshotIssue {
  path: string
  source: ContentSnapshotIssueSource
  code: string
}

export function diagnoseContentSnapshot(value: unknown): readonly ContentSnapshotIssue[]
```

规则如下：

- `path` 使用 JSON Pointer，根节点为 `/`。
- Schema 问题的 `code` 使用稳定的 Ajv keyword，例如 `required`、`type`、`format`。
- 语义问题使用仓库定义的稳定代码：`duplicate-id`、`missing-reference`、`reference-mismatch`、`asset-kind`、`hidden-target`。
- 诊断顺序保持确定性，与 Schema 遍历和快照记录顺序一致。
- `validateContentSnapshot()` 继续返回路径数组，避免破坏已有调用方。
- `assertContentSnapshot()` 继续抛出 `ContentSnapshotValidationError`，其 `issues` 仍为路径数组。

语义代码用于帮助编辑人员理解错误类型，不取代服务端校验，也不写入内容快照。

## 6. 管理端工作台模型

新增 `apps/admin/utils/snapshot-workbench.ts`，提供以下纯函数边界：

```ts
export interface SnapshotAnalysis {
  snapshot: YujianContentSnapshot | null
  issues: readonly SnapshotEditorIssue[]
  summary: SnapshotSummary | null
}

export function analyzeSnapshotText(text: string): SnapshotAnalysis
export function readSnapshotImport(file: SnapshotImportFile): Promise<string>
export function createSnapshotExport(snapshot: YujianContentSnapshot): SnapshotExport
```

行为约束：

- JSON 语法错误、非对象根节点和契约错误都返回结构化问题，不抛出到 Vue 页面。
- 只有完整通过共享诊断的值才作为 `snapshot` 返回。
- 摘要只从有效快照生成，避免在不可信结构上访问字段。
- 导入拒绝非 `.json` 文件、空文件和超过 2 MiB 的文件。
- 导出内容使用 2 空格缩进并以换行结尾，不包含 Token、API 地址、版本状态或发布任务。
- 导出文件名为 `<releaseId>.json`；无法安全使用时回退 `yujian-snapshot.json`。

## 7. 页面体验

管理端编辑区增加紧凑工具栏：

- 导入按钮打开隐藏文件选择器。
- 导出按钮仅在快照有效时启用。
- 状态标记显示“有效”或错误数量。

右侧预览区调整为两层：

1. 有效快照显示内容数量摘要，使用紧凑数字网格，避免新增说明性长文。
2. 无效快照显示最多 8 条诊断，包含字段路径和本地化问题类别；剩余数量以汇总显示。

原始 JSON 预览继续保留在可滚动区域，便于审核实际内容。窄屏下工具栏允许换行，按钮触控高度不小于 44 px。状态不只依赖颜色表达，键盘焦点沿用现有全局样式。

## 8. 数据流

```text
文本输入或本地 JSON 文件
  -> analyzeSnapshotText
  -> JSON 解析与对象根校验
  -> @yujian/schema diagnoseContentSnapshot
  -> 有效快照 + 内容摘要，或结构化诊断
  -> Vue 响应式状态
  -> 保存草稿 / 本地导出
```

保存仍调用现有 Go API。服务端继续执行最终 Schema、语义、权限和乐观锁校验。

## 9. 错误处理与安全

- 文件读取失败转换为管理端可展示的安全错误，不暴露本地路径。
- 导入内容只作为文本解析，不执行 HTML、脚本或 CSS。
- 导出文件只包含已验证的快照对象。
- 对未知诊断代码显示通用“字段不符合内容契约”，不直接向用户展示 Ajv 内部对象。
- 页面创建的 Object URL 在触发下载后立即释放。
- 不把编辑内容自动写入 `localStorage`，避免共享设备残留未发布资料。

## 10. 测试策略

- Schema 单元测试覆盖结构化 Schema 诊断、语义诊断和旧 API 兼容性。
- 工作台纯函数测试覆盖合法快照、语法错误、非对象根、契约错误、摘要、导入限制和导出内容。
- 工作区组合测试覆盖无效快照禁用保存、导入更新编辑器和导出描述。
- 页面组件测试覆盖工具栏、状态、摘要与诊断渲染。
- 完成前运行管理端测试与类型检查，再运行仓库完整前端、Go、覆盖率、E2E 和自动化门禁。

## 11. 验收标准

- 管理端与服务端使用同一份 canonical Schema 语义规则。
- 任一无效快照都不能从管理端发起保存。
- 错误至少包含准确 JSON Pointer 路径和稳定问题类别。
- 合法 fixture 的摘要数量准确，导入后可编辑，导出后可再次通过校验。
- 本轮没有新增或修改发布、回滚、EdgeOne 和部署行为。
- 所有受影响测试、构建和仓库门禁通过。
