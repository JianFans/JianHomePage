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

- [x] 创建事务测试，运行 `pnpm --filter @yujian/admin test test/unit/moment-workbench.test.ts`；缺失模块后先建立最小接口骨架，确认行为断言失败再实现，不把编译错误当行为红灯。

```ts
const draft = createMomentDraft(snapshot)
delete draft.moments[0]!.caption
draft.moments[0]!.target = { kind: 'internal', contentId: 'release_01' }
const result = applyMomentDraft(text, text, draft)
expect(result.error).toBeNull()
expect(JSON.parse(result.text)).toEqual({ ...snapshot, moments: draft.moments })
expect(applyMomentDraft(text + '\n', text, draft).error).toBe('stale-snapshot')
```

- [x] 实现接口及完整校验；覆盖深复制、可选字段移除、图片／GIF、非法素材、空字段、重复／缺失 ID、HTTPS、安全目标、隐藏／缺失内部目标及失败原文保留。

```ts
interface MomentDraft { moments: Moment[] }
type MomentErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-moment' | 'invalid-draft'
function createMomentDraft(snapshot: YujianContentSnapshot): MomentDraft
function createMoment(): Moment
function applyMomentDraft(text: string, baseline: string, draft: MomentDraft): {
  text: string; error: MomentErrorCode | null; issues: readonly SnapshotEditorIssue[]
}
```

- [x] 先编写状态机失败测试，再实现 `useMomentWorkspace({ editorText, busy })`，返回 `draft/dirty/stale/error/issues/pending/open/apply/reset/close/keepEditing/discardPending`。验证同步原文基线、失败保留、确认取消、busy 与关闭重置。
- [x] 先写组件及页面失败测试，再实现 MomentWorkbench、MomentFields 和双语文案。通过显式 caption/label 开关添加 `{ 'zh-CN': '' }`，关闭使用 `delete`；目标切换创建唯一分支。选中编辑区按 ID 隔离，素材只列图片／GIF，使用最近有效快照提供选项，不加载媒体。诊断截断为 8 条。
- [x] 页面添加 moment.dirty、keepEditing/reset 和保存成功 open，在 EventWorkbench 后放置片段工作台。扩展页面测试验证五表单过期、保存／导出／审核／发布／回滚、离开提示、替换失败及成功、409 与成功基线刷新。
- [x] 新增桌面浏览器失败场景，先确认缺少 moment-open；实现后在桌面／手机验证新增、首页加入引用及真实 iframe、导出再导入、内部／外部链接、字段移除、五表单保护、双语键盘、布局和保存契约。媒体请求拦截；保存使用当前 revision 与 If-Match，冲突保留，延迟成功期间锁定。
- [x] 运行片段窄测试、管理端全量单测、类型检查及相关 ESLint，按事务／状态与界面／集成两个职责原子提交，各自测试同提交。
- [x] 全新只读子代理完成规格审查，问题修复复审通过后再做质量审查。

## 任务 2：整体验收与文档

- [x] 主代理核对实际 diff 与调用链；运行 `pnpm verify`、`pnpm test:coverage`、完整管理端 E2E，同一 Nuxt 工作区顺序执行。必要时仅进程设置 `VITEST_MAX_WORKERS=4`，不放宽断言、超时和门槛。
- [x] 使用浏览器连接实际编辑片段、校验拒绝、首页引用和 iframe，验证中英文、手机布局与控制台。
- [x] 更新根／管理端 README 和路线图，记录实测命令和真实联调边界，检查相对文档链接、生成文件和 diff。
- [x] 最终独立分支审查、复核暂存清单及 `git diff --cached --check`，以中文原子提交文档；保留干净功能分支。

## 验收记录

2026-10-13，基于主分支 `13fb70c` 完成本轮片段工作台与本地验收。

- TDD：事务接口骨架 21 项行为失败、状态机骨架 6 项失败后实现；组件空模板与页面缺少入口先确认行为红灯，随后组件与页面共 31 项通过。生产构建成功后，新增桌面 E2E 因缺少 `moment-open` 失败，完成界面后桌面／手机共 10 项通过。
- 独立规格审查通过后进行独立质量审查，均未发现阻塞问题。主代理手动验收发现内部目标选择框误用素材提示；补充中英文两项组件失败测试，再修正为「选择内容 / Choose content」，素材提示保持原文。
- 修正后的 `pnpm verify`：退出码 0；Schema 34、管理端 318、公开站 105、脚本 21，共 478 项测试通过。ESLint、类型检查、Schema 生成、两站静态生成与静态产物检查通过；公开站 18 个本地资源引用，首屏 JavaScript 为 287 KiB。
- 修正后的 `pnpm test:coverage`：退出码 0，覆盖率见下表。验证进程设置 `VITEST_MAX_WORKERS=4`，未修改仓库并发、超时、断言或门槛。
- `pnpm verify:go` 与 `pnpm test:coverage:go`：退出码均为 0，gofmt、Go generate、全包测试与 Go vet 通过；Go 全包语句覆盖率为 82.48%，门槛为 80%。本轮没有 Go 源码或生成文件变更。
- 管理端完整 Playwright：桌面与手机共 60 项通过，其中新增片段 10 项。文案修正后重新构建生产产物，再跑受影响的片段场景，10 项全部通过。保存场景使用请求拦截，只验证修订、冲突与交接契约。
- 最终独立复核覆盖文案修正、全分支代码与同步文档，未发现新增问题；25 个本地文档链接目标检查通过，生成文件没有残留改动。

| 工作区 | 语句 | 分支 | 函数 | 行 |
| --- | --- | --- | --- | --- |
| Schema | 90.52% | 77.27% | 95.91% | 94.54% |
| 管理端 | 91.17% | 86.11% | 88.27% | 93.18% |
| 公开站 | 89.35% | 79.09% | 91.75% | 89.42% |

浏览器连接使用独立上下文与本地生产产物。通过 JSON 编辑器载入开发快照，实际新增片段、绑定封面、开启双语说明并设置目标。非 HTTPS 链接返回 `/moments/3/target/link/url` 诊断；新记录指向尚未编排的自身返回 `hidden-target`，均保留 JSON 和输入。改为可见的 `release_01` 后应用成功，首页编排保持不变；通过首页表单加入引用、将上限调为 4 并应用后，真实 iframe 存在新增 `figure`，链接为 `#release_01`，图片使用素材 alt，说明使用隐藏的 `figcaption`。

管理端与预览分别切换英文后保留双语输入；手机视口页面 `innerWidth` 与 `scrollWidth` 均为 390，预览手机宽度为 390，无横向溢出。片段编辑表单未加载图片、音频或视频。切换手机模拟时触发标准未保存提示，选择保留后输入仍在。

手动浏览器控制台仅观察到开发媒体连接拒绝、`yujian.me` 测试媒体证书不匹配与既有同源 iframe sandbox 警告，未观察到脚本异常；未绕过证书检查，不据此确认真实媒体可达性。Playwright 所需 Chromium 下载因网络超时失败，验证使用本机 Chrome，仅在临时配置覆盖 `use.channel`，验收后已删除该配置，未改变正式配置、媒体测试或断言。依赖安装和 Go 下载分别使用进程级 npm 官方 registry 与 Go 镜像，锁文件及仓库配置不变。

真实 Go、OIDC、COS、EdgeOne 和 PostgreSQL 集成尚未串联；本轮未执行公开站 E2E、容器构建、正式快照 EdgeOne 门禁或部署。公开站与共享渲染源码未改，公开站单测、类型检查及静态生成通过。

提交按职责拆为 `07e7f8b`（片段事务与状态机）、`f45cf11`（双语表单、页面与浏览器回归）和 `1930cc6`（内部目标提示修复与双语回归）。规格与初始计划为 `cf86a19`，使用说明与验收记录独立提交；保留功能分支，不推送或创建 PR。
