# 管理端片段工作台实现计划

> 面向 AI 代理：使用 subagent-driven-development 与 TDD，先规格审查再质量审查。用户授权独立完成、分支开发与原子提交；复用当前检出目录，不创建额外工作树，不推送或部署。

**目标：** 不手改 JSON 即可维护片段记录，接入首页编排、真实预览和五工作台输入保护。

**架构：** 纯事务、集合草稿状态机、双语字段和页面协调。复用 canonical 校验、已有 LocalizedTextFields 和平台枚举，不引入通用工作台重构。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright。

## 文件职责

| 文件 | 职责 |
| --- | --- |
| `apps/admin/utils/moment-workbench.ts` | 深复制、新记录及仅替换 moments 的原子事务 |
| `apps/admin/composables/useMomentWorkspace.ts` | dirty、stale、确认、错误与忙碌状态 |
| `apps/admin/utils/moment-copy.ts` | 双语表单和稳定错误文案 |
| `apps/admin/components/MomentWorkbench.vue` | 片段列表、新增与生命周期操作 |
| `apps/admin/components/MomentFields.vue` | 素材、可选说明、目标及外链标签 |
| `apps/admin/pages/index.vue` | 五工作台全局门禁、替换及保存交接 |
| `apps/admin/test/unit/moment-*.test.ts` | 事务、状态机和真实组件回归 |
| `apps/admin/test/unit/admin-shell.test.ts` | 页面保存／替换与全部入口保护 |
| `apps/admin/test/e2e/moment-workbench.spec.ts` | 浏览器闭环与保存契约 |

## 任务 1：片段维护闭环

- [ ] 创建事务测试，运行 `pnpm --filter @yujian/admin test test/unit/moment-workbench.test.ts`；缺失模块后先建立最小接口骨架，确认行为断言失败再实现，不把编译错误当行为红灯。

```ts
const draft = createMomentDraft(snapshot)
delete draft.moments[0]!.caption
draft.moments[0]!.target = { kind: 'internal', contentId: 'release_01' }
const result = applyMomentDraft(text, text, draft)
expect(result.error).toBeNull()
expect(JSON.parse(result.text)).toEqual({ ...snapshot, moments: draft.moments })
expect(applyMomentDraft(text + '\n', text, draft).error).toBe('stale-snapshot')
```

- [ ] 实现接口及完整校验；覆盖深复制、可选字段移除、图片／GIF、非法素材、空字段、重复／缺失 ID、HTTPS、安全目标、隐藏／缺失内部目标及失败原文保留。

```ts
interface MomentDraft { moments: Moment[] }
type MomentErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-moment' | 'invalid-draft'
function createMomentDraft(snapshot: YujianContentSnapshot): MomentDraft
function createMoment(): Moment
function applyMomentDraft(text: string, baseline: string, draft: MomentDraft): {
  text: string; error: MomentErrorCode | null; issues: readonly SnapshotEditorIssue[]
}
```

- [ ] 先编写状态机失败测试，再实现 `useMomentWorkspace({ editorText, busy })`，返回 `draft/dirty/stale/error/issues/pending/open/apply/reset/close/keepEditing/discardPending`。验证同步原文基线、失败保留、确认取消、busy 与关闭重置。
- [ ] 先写组件及页面失败测试，再实现 MomentWorkbench、MomentFields 和双语文案。通过显式 caption/label 开关添加 `{ 'zh-CN': '' }`，关闭使用 `delete`；目标切换创建唯一分支。选中编辑区按 ID 隔离，素材只列图片／GIF，使用最近有效快照提供选项，不加载媒体。诊断截断为 8 条。
- [ ] 页面添加 moment.dirty、keepEditing/reset 和保存成功 open，在 EventWorkbench 后放置片段工作台。扩展页面测试验证五表单过期、保存／导出／审核／发布／回滚、离开提示、替换失败及成功、409 与成功基线刷新。
- [ ] 新增桌面浏览器失败场景，先确认缺少 moment-open；实现后在桌面／手机验证新增、首页加入引用及真实 iframe、导出再导入、内部／外部链接、字段移除、五表单保护、双语键盘、布局和保存契约。媒体请求拦截；保存使用当前 revision 与 If-Match，冲突保留，延迟成功期间锁定。
- [ ] 运行片段窄测试、管理端全量单测、类型检查及相关 ESLint，按事务／状态与界面／集成两个职责原子提交，各自测试同提交。
- [ ] 全新只读子代理完成规格审查，问题修复复审通过后再做质量审查。

## 任务 2：整体验收与文档

- [ ] 主代理核对实际 diff 与调用链；运行 `pnpm verify`、`pnpm test:coverage`、完整管理端 E2E，同一 Nuxt 工作区顺序执行。必要时仅进程设置 `VITEST_MAX_WORKERS=4`，不放宽断言、超时和门槛。
- [ ] 使用浏览器连接实际编辑片段、校验拒绝、首页引用和 iframe，验证中英文、手机布局与控制台。
- [ ] 更新根／管理端 README 和路线图，记录实测命令和真实联调边界，检查相对文档链接、生成文件和 diff。
- [ ] 最终独立分支审查、复核暂存清单及 `git diff --cached --check`，以中文原子提交文档；保留干净功能分支。
