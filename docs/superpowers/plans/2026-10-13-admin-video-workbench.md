# 管理端影像工作台实现计划

> 面向 AI 代理：使用 subagent-driven-development 与 TDD 实现，依次执行规格与质量审查。用户已授权独立完成并原子提交；使用 `feat/admin-video-workbench`，不创建额外工作树，不推送、合并或部署。

**目标：** 通过结构化表单维护视频记录，与现有快照、首页编排和视觉预览闭环。

**架构：** 纯影像事务 + 临时工作区 + 双语表单；页面将其接入统一未应用保护和保存／替换交接。

**技术栈：** Nuxt、Vue、TypeScript、canonical Schema、Vitest、Playwright。

## 文件职责

| 文件 | 职责 |
| --- | --- |
| `apps/admin/utils/video-workbench.ts` | 创建独立草稿、新记录与原子应用 |
| `apps/admin/composables/useVideoWorkspace.ts` | 打开、重载、关闭、确认、过期和忙碌状态 |
| `apps/admin/utils/video-copy.ts` | 类型对齐的中英文界面与错误文案 |
| `apps/admin/components/VideoWorkbench.vue` | 记录选择、素材字段、双语字段和应用操作 |
| `apps/admin/pages/index.vue` | 第三个表单的脏状态、替换和保存交接 |
| `apps/admin/test/unit/video-*.test.ts` | 事务、状态与组件行为回归 |
| `apps/admin/test/unit/admin-shell.test.ts` | 多工作台与全局保护回归 |
| `apps/admin/test/e2e/video-workbench.spec.ts` | 桌面／手机的真实编辑闭环与保存契约 |

## 任务 1：影像维护闭环

- [x] 写纯事务失败测试并运行 `pnpm --filter @yujian/admin test test/unit/video-workbench.test.ts`。新工具不存在时先确认缺失模块；实现前保留能断言旧原文不被修改的失败行为。

```ts
const draft = createVideoDraft(snapshot)
draft.videos[0]!.title.en = 'Edited video'
const result = applyVideoDraft(text, text, draft)
expect(result.error).toBeNull()
expect(JSON.parse(result.text).videos[0].title.en).toBe('Edited video')
expect(JSON.parse(result.text).homepage).toEqual(snapshot.homepage)
expect(applyVideoDraft(`${text}\n`, text, draft).error).toBe('stale-snapshot')
```

- [x] 实现以下固定边界，全部失败返回输入原文；已有记录 ID 必须保留，重复 ID 与其他字段交由完整校验拒绝。

```ts
interface VideoDraft { videos: Video[] }
type VideoErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-video' | 'invalid-draft'
function createVideoDraft(snapshot: YujianContentSnapshot): VideoDraft
function createVideo(): Video
function applyVideoDraft(text: string, baseline: string, draft: VideoDraft): {
  text: string; error: VideoErrorCode | null; issues: readonly SnapshotEditorIssue[]
}
```

- [x] 添加工作区失败测试：当前原文打开、临时深副本、重载／关闭确认取消与放弃、其他表单应用后过期、错误保留、busy 期间不改变草稿或 JSON。
- [x] 实现 `useVideoWorkspace({ editorText, busy })`，对齐首页工作区的 `draft/dirty/stale/error/issues/pending/open/apply/reset/close/keepEditing/discardPending` 接口；应用成功同步当前草稿基线。
- [x] 添加组件与页面失败测试：双语、新增、切换保留输入、封面与视频筛选、清除可选字段、无效字段提示、锁定、保存／导出等门禁、替换失败保留和保存成功刷新基线。
- [x] 实现 VideoWorkbench，复用 LocalizedTextFields、PlatformLinkFields 与既有样式，不新增编辑预览播放器。已有 ID 只读显示，记录按位置选择，新记录使用 UUID。平台链接默认 bilibili、空 URL，时长默认 1；空标题／封面必须填写后才能通过应用校验。
- [x] 在管理首页接入工作区；统一 `formDirty`，替换前取消局部待确认，替换成功才 reset，保存成功只刷新已打开的干净工作区。
- [x] 编写并运行桌面 E2E 红灯：导入有效快照、打开影像编辑、新增标题和封面、至少一个合法平台链接、应用、首页重载加引用、预览检查标题、导出再导入保持快照。另覆盖三表单冲突、双语键盘／布局及拦截保存冲突与忙碌状态。
- [x] 运行新增窄测试、管理端全量单元测试与类型检查，修复后保持真实行为断言。

## 任务 2：审查与验收

- [x] 独立规格审查确认任务 1 完整实现，再独立质量审查；对发现逐项验证、修复和复审。
- [x] 运行 `pnpm verify`、`pnpm test:coverage`、管理端 `test:e2e` 与公开站 `test:e2e`；同一 Nuxt 目录的命令顺序运行。
- [x] 用浏览器连接实际操作新增、应用、预览、语言和手机布局，检查控制台错误。
- [x] 更新管理端／根 README 与路线图，记录当前实测门禁和真实云未验证的边界。
- [x] 检查 diff 和暂存范围，以中文原子提交保存实现与测试，再独立提交验收文档。恢复本任务 Nuxt 自动元数据，保留功能分支与干净工作区。

## 验收记录

2026-10-13，基于 `39c813e` 完成本地验证；以下结果来自本轮命令，不沿用上一阶段数字。

- TDD 红灯：新模块缺失后补齐契约骨架，确认新 ID 为空、工作区无法打开与页面缺少影像入口的行为断言失败；桌面 E2E 在构建成功后因缺少 `video-open` 超时。初轮构建中的缺失测试依赖与元组类型错误已在行为红灯前修正。
- 新增窄测试及页面集成：4 个文件、45 项通过；影像 E2E 桌面与手机共 8 项通过。
- `pnpm verify`：退出码 0；Schema 19、管理端 240、公开站 105、脚本 21，共 385 项测试通过。ESLint、类型检查、Schema 生成、两站静态生成及产物校验通过，公开站包含 18 个本地资源引用，首屏 JavaScript 为 287 KiB。
- `pnpm test:coverage`：退出码 0。各工作区语句／分支／函数／行覆盖率见下表；未降低阈值。
- 管理端完整 E2E：桌面与手机共 40 项通过；公开站 E2E 与 axe 检查共 11 项通过。
- 独立规格与质量审查均未发现需修复问题；主代理复核应用边界、页面交接、测试与文档。文档相对链接目标检查通过。

| 工作区 | 语句 | 分支 | 函数 | 行 |
| --- | --- | --- | --- | --- |
| Schema | 90.47% | 76.92% | 95.91% | 94.51% |
| 管理端 | 90.24% | 84.19% | 87.52% | 92.70% |
| 公开站 | 90.37% | 80.85% | 91.75% | 90.76% |

影像工作区状态机四项覆盖率均为 100%；影像事务语句 95.65%、分支与函数 100%、行 93.75%。浏览器连接使用隔离上下文和本地产物，实际填写新增记录、应用、在首页加入引用、打开真实 iframe、切换管理端与预览语言并检查手机布局。390 px 视口下页面 `scrollWidth` 为 390，影像面板宽 366，预览手机视口为 390；编辑表单没有视频元素。切换手机模拟时触发标准离开确认，取消后内容保留。

浏览器控制台出现测试媒体地址连接拒绝、证书不匹配及既有同源 iframe sandbox 警告，没有脚本异常。真实媒体可达性不由此次 fixture 验证证明；E2E 保存使用请求拦截，未联调真实 Go API、OIDC、COS 或 EdgeOne。本轮不修改 Go、数据库、Docker 或发布配置，未重跑 Go／容器／正式快照门禁，未部署。

实现按职责拆为 `42a4791`（影像事务与状态机）和 `ba8aa75`（表单、页面集成与浏览器回归），测试随对应实现提交。设计为 `55abad4`，使用说明与本验收记录独立提交；保留功能分支，不推送或创建 PR。
