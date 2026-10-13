# 管理端现场工作台实现计划

> 面向 AI 代理：使用 subagent-driven-development 与 TDD，执行独立规格和质量审查。用户已授权独立完成及原子提交；使用现有检出的功能分支，不创建额外工作树，不推送、合并或部署。

**目标：** 无需手改 JSON 即可维护现场记录，并接入首页编排、真实预览和全局输入保护。

**架构：** 纯现场事务、临时集合草稿、双语表单，页面只协调四工作台交接。canonical Schema 为唯一校验源。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright。

## 文件职责

| 文件 | 职责 |
| --- | --- |
| `apps/admin/utils/event-workbench.ts` | 深复制、空记录与仅替换现场的原子应用 |
| `apps/admin/composables/useEventWorkspace.ts` | 临时输入、基线、确认、错误与忙碌状态 |
| `apps/admin/utils/event-copy.ts` | 中英文标签、状态与错误文案 |
| `apps/admin/components/EventWorkbench.vue` | 选择、新增、双语、时间、状态、链接与海报 |
| `apps/admin/pages/index.vue` | 全局门禁、替换和保存交接 |
| `apps/admin/test/unit/event-*.test.ts` | 事务、状态机与真实组件回归 |
| `apps/admin/test/unit/admin-shell.test.ts` | 四工作台与页面回归 |
| `apps/admin/test/e2e/event-workbench.spec.ts` | 真实编辑、预览、状态过滤与保存契约 |

## 任务 1：实现现场维护闭环

- [x] 编写事务失败测试并运行 `pnpm --filter @yujian/admin test test/unit/event-workbench.test.ts`。先确认缺失模块，再用最小契约骨架确认行为断言失败，不以编译错误代替行为红灯。

```ts
const draft = createEventDraft(snapshot)
draft.events[0]!.dateTime = '2030-01-02T20:00:00+08:00'
const result = applyEventDraft(text, text, draft)
expect(result.error).toBeNull()
expect(JSON.parse(result.text)).toEqual({ ...snapshot, events: draft.events })
expect(applyEventDraft(text + '\n', text, draft).error).toBe('stale-snapshot')
```

- [x] 实现固定接口，失败保持原文及精确诊断。新记录空标题、城市、场馆、时间、详情地址，状态 `scheduled`，无选填海报。

```ts
interface EventDraft { events: Event[] }
type EventErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-event' | 'invalid-draft'
function createEventDraft(snapshot: YujianContentSnapshot): EventDraft
function createEvent(): Event
function applyEventDraft(text: string, baseline: string, draft: EventDraft): {
  text: string; error: EventErrorCode | null; issues: readonly SnapshotEditorIssue[]
}
```

- [x] 事务测试覆盖保留既有 ID、重复 ID、合法偏移原文、无时区／无效时间、非法状态、空或危险链接、图片／GIF 海报、错误素材类型、悬空引用、可选字段移除、时间／状态导致内部跳转隐藏，以及非法草稿形状。
- [x] 编写状态机失败测试，随后实现 `useEventWorkspace({ editorText, busy })`：`draft/dirty/stale/error/issues/pending/open/apply/reset/close/keepEditing/discardPending`。同步打开和应用、重载／关闭确认、读取失败保留草稿与待办，busy 不执行变更。
- [x] 编写真实组件与页面失败测试，再实现字段及交接。选择保留整组输入，选中编辑区按 ID 隔离；可选海报清空使用 `delete`，海报只列图片／GIF。日期为原文 text 输入，附时区提示与可访问关联；状态标签双语。诊断最多展示 8 条并提示剩余数量。
- [x] 页面接入 `event.dirty`、`keepEditing/reset` 和保存成功 `open`，在 VideoWorkbench 后放置 EventWorkbench。四表单未应用保护、替换失败保留、保存成功刷新及 409 锁定必须有真实页面测试。
- [x] 编写桌面 E2E 红灯，确认缺少 `event-open` 的功能失败；实现后新增记录、应用、首页加引用并真实预览、导出再导入。另测售罄／取消／过去时间过滤、四表单冲突、双语键盘／窄屏和保存冲突／成功契约。媒体请求拦截，保存 API 不连接真实服务。
- [x] 运行新增窄测试、管理端全量测试与类型检查；提交按领域事务／状态和页面表单／浏览器回归拆为两个职责，测试与对应实现一同提交。
- [x] 独立规格审查通过后执行独立质量审查，验证问题后修复并复审。

## 任务 2：整体验收与说明

- [x] 新表单失败测试暴露 TypeScript／Go 日期差异：补充跨端日历回归，确认 TypeScript 的 8 个行为红灯后修复共享校验，34 项 Schema 测试与 `pnpm verify:go` 通过；不增加表单专用校验或修改 Schema 定义。

- [x] 运行 `pnpm verify`、`pnpm test:coverage`、`pnpm --filter @yujian/admin test:e2e`，同一 Nuxt 目录顺序执行；公开站 E2E 验证共享预览无回归。
- [x] 使用浏览器连接实际填写现场、应用、首页引用与 iframe 预览，检查中英文、手机布局及控制台。
- [x] 更新根／管理端 README 与内容路线图，记录命令实测结果、已知联调边界；不引入部署承诺。
- [x] 最终独立审查整个分支，检查 `git diff --check`、暂存文件清单和暂存 diff，原子提交说明；保留干净功能分支，不推送或创建 PR。

## 验收记录

2026-10-13，基于主分支 `058df2d` 完成本轮实现与本地验证。

- TDD：事务、状态机、真实组件和页面先确认缺失行为；桌面 E2E 在生产构建成功后因缺少 `event-open` 超时。构建中测试元组缺少 `as const` 的类型错误已在行为红灯前修正。
- 现场失败用例暴露共享日期校验自动进位。先确认 TypeScript 的 8 个行为失败，再修复真实日历和小时检查，补充合法偏移、闰年与低年份回归；34 项 Schema 测试通过，Go 的 13 项现场日期子用例确认原有拒绝行为。
- `pnpm verify`：退出码 0；Schema 34、管理端 280、公开站 105、脚本 21，共 440 项测试通过。ESLint、类型检查、Schema 生成、两站静态生成与产物检查通过，公开站包含 18 个本地资源引用，首屏 JavaScript 为 287 KiB。
- 首次直接执行完整门禁时，两个 Nuxt 工作区并发初始化触发 hook 超时及连锁页面失败；隔离运行页面 22 项和管理端 280 项均通过。随后仅在验证进程设置 `VITEST_MAX_WORKERS=4`，完整门禁与覆盖率全部通过。未修改超时、断言、仓库测试配置或门槛。
- `pnpm test:coverage`：退出码 0，各工作区语句／分支／函数／行覆盖率见下表。现场状态机四项覆盖率均为 100%；现场事务语句 95.65%、分支与函数 100%、行 93.75%。
- `pnpm verify:go` 与 `pnpm test:coverage:go`：退出码均为 0；gofmt、Go generate、全包测试与 Go vet 通过，Go 全包语句覆盖率为 82.48%，门槛为 80%。
- 管理端完整 E2E：桌面与手机共 50 项通过，其中新增现场 10 项；公开站 E2E 与 axe 检查共 11 项通过。
- 独立规格与质量审查均未发现需修复问题；共享日期修复另经独立审查。主代理复核应用边界、四表单交接与测试断言，文档相对链接目标检查通过。

| 工作区 | 语句 | 分支 | 函数 | 行 |
| --- | --- | --- | --- | --- |
| Schema | 90.52% | 77.27% | 95.91% | 94.54% |
| 管理端 | 90.71% | 85.28% | 87.87% | 92.95% |
| 公开站 | 89.35% | 79.09% | 91.75% | 89.42% |

浏览器连接使用独立上下文和本地生产产物，实际新增现场并填写全部字段。输入 2 月 30 日时拒绝应用，显示 `/events/2/dateTime`，保留原文与临时输入；修正后保留 `+08:00` 原文。通过首页表单添加引用并提高上限后，真实 iframe 展示新增现场。管理端与预览切换英文后内容保留；390 px 视口下页面 `scrollWidth` 为 377，现场面板约 350.67 px，预览手机视口为 390，无横向溢出，现场编辑表单不加载图片或视频。

控制台仅观察到测试媒体地址连接拒绝、证书不匹配及既有同源 iframe sandbox 警告，未观察到脚本异常。未绕过证书错误，不能据此证明真实媒体可达性。E2E 保存使用请求拦截，只证明修订与交接契约；未串联真实 Go、OIDC、COS 或 EdgeOne。PostgreSQL 集成测试未配置真实数据库，本轮未执行容器构建、正式快照 EdgeOne 门禁或部署。

提交按职责拆为 `7c28007`（现场事务与状态机）、`cb4277b`（共享日期修复与跨端回归）、`afc023b`（现场日期及同步基线回归）和 `5a72c42`（双语表单、页面与浏览器回归）。规格与初始计划为 `2cd744a`，使用说明与本验收记录独立提交；保留功能分支，不推送或创建 PR。
