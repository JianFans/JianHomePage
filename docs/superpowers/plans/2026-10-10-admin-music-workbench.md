# 音乐内容结构化编辑实现计划

> 面向 AI 代理：按 executing-plans 在当前功能分支逐任务执行；行为修改遵循 TDD。

**目标：** 从有效快照创建、编辑作品与曲目，绑定已有素材并编排音乐板块，安全保存草稿。

**架构：** `editorText` 保持唯一可保存内容源。音乐表单持有临时副本，应用时检查文本基线，事务式修改副本并执行完整共享校验。保存沿用现有版本 API 和乐观修订。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright、`@yujian/schema`。

## 任务 1：快照事务

文件：`apps/admin/utils/music-workbench.ts`、`apps/admin/test/unit/music-workbench.test.ts`。

接口使用以下结构，字段类型复用生成契约：

```ts
interface MusicDraft {
  originalId: string | null
  release: Omit<Release, 'trackIds'>
  tracks: Track[]
  sections: { id: string; itemIds: string[] }[]
}
```

- [x] 先写新增作品与曲目、更新和排序、记录 ID 冲突、跨作品曲目、移除被引用曲目、素材类型及首页隐藏目标测试。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/music-workbench.test.ts`，确认功能缺失导致失败。
- [x] 实现 `createMusicDraft(snapshot, releaseId)` 和 `applyMusicDraft(text, baseline, draft)`。应用结果包含原文或新文本、稳定错误代码及共享诊断；所有失败保持原文。
- [x] 同步 `release.trackIds` 与曲目归属；只修改当前作品、其原有曲目及显式编辑的音乐板块条目，不变更其他内容。
- [x] 重新运行目标测试、管理端类型检查与目标文件 lint，再提交 `feat(音乐编辑): 添加快照事务与引用校验`。

## 任务 2：状态与未保存保护

文件：`apps/admin/composables/useMusicWorkspace.ts`、`apps/admin/composables/useAdminWorkspace.ts` 及对应 `test/unit` 测试。

- [x] 先测试未应用输入、取消、过期基线、忙碌门禁，以及保存冲突保持文本和修订号。
- [x] 运行目标测试确认失败，再实现打开、应用、丢弃和切换请求。切换前通过内联继续编辑／放弃提示保留决定权。
- [x] 音乐列表复用工作区防抖分析；打开与应用直接校验最新文本，保存直接分析当前文本。
- [x] 记录服务端文本基线，区分本地未保存快照与未应用表单。保存期间阻止重入、载入、导入、素材插入和表单改动。
- [x] 重新运行 `admin-workspace.test.ts`、`music-workspace.test.ts` 与类型检查，再提交 `feat(音乐编辑): 保护临时表单和未保存快照`。

## 任务 3：界面

文件：`apps/admin/components/MusicWorkbench.vue`、`LocalizedTextFields.vue`、`PlatformLinkFields.vue`、`MusicTrackFields.vue`、`apps/admin/utils/music-copy.ts`、`apps/admin/pages/index.vue`、`apps/admin/components/AssetWorkbench.vue` 及对应组件测试。

- [x] 先测试作品选择、新建、应用、取消、双语标签、素材过滤和禁用状态，再运行组件测试确认缺少行为。
- [x] 实现作品列表与表单，独立小组件复用双语字段、平台链接和曲目编辑，避免单文件承担全部字段。
- [x] 曲目排序与音乐板块排序采用可访问的上下移动按钮。保留原 ID 和选填文案；隐藏值不得因切换界面语言被清除。
- [x] 接入页面未保存提示、载入／导入保护和 `beforeunload`。素材插入遵守保存忙碌状态。
- [x] 重新运行管理端测试、类型检查、lint，再提交 `feat(音乐工作台): 接入双语作品曲目表单`。

## 任务 4：浏览器与 CI

文件：`apps/admin/playwright.config.ts`、`apps/admin/test/e2e/music-workbench.spec.ts`、`apps/admin/package.json`、`pnpm-lock.yaml`、`.github/workflows/code-check.yml`、`scripts/automation-config.test.mjs`、`apps/admin/README.md`。

- [x] 先写浏览器测试：导入 fixture、新增作品与曲目、绑定封面／音频、排序、应用、导出并重新校验；另测无效与过期编辑、保存请求修订与冲突。
- [x] 新增管理端 E2E 脚本，Playwright 版本与公开站一致；Vitest include 收窄到单元测试以避免加载 Playwright 测试。
- [x] 先扩展自动化契约测试要求管理端 E2E 命令与结果归档，确认失败，再接入现有 E2E job；不把依赖版本约束写进工作流。
- [x] 桌面与窄屏浏览器运行验证布局、键盘入口和实际字段行为。拦截 API 的测试明确标注为契约测试，不冒充真实服务端或云集成。
- [x] 同步 README 的操作路径、未保存行为、支持范围和验证命令；浏览器／CI 验证与使用文档分别原子提交。

## 最终门禁

- [x] `pnpm --filter @yujian/admin build`
- [x] `pnpm test:coverage`
- [x] `pnpm test:automation`
- [x] `pnpm verify`
- [x] `pnpm verify:go`
- [x] `pnpm --filter @yujian/admin test:e2e`
- [x] `pnpm --filter @yujian/web test:e2e`
- [x] 对比 `master...HEAD` 完整审查，修复实际问题后重新验证受影响范围。
- [x] 每次提交前检查暂存文件和 `git diff --cached --check`；结束时报告提交、验证与真实环境缺口。

## 验收记录

本轮已完成音乐工作台，未修改公开内容 Schema、公开站内容加载方式或服务端 API。

首版已运行完整仓库门禁。最终代码复核基于 `master...d51b1cc`，重新运行管理端覆盖率、类型检查、生产构建 Playwright、lint 和自动化配置测试；下表区分首版记录与最终复核结果。

| 验证 | 结果 |
| --- | --- |
| `pnpm verify`（首版） | lint、全部类型检查、220 项单元测试、21 项脚本测试、静态生成和产物检查通过；首屏 JavaScript 286 KiB |
| `pnpm test:coverage`（首版） | Schema、管理端、公开站均达标；最终管理端覆盖率见下一行 |
| `pnpm --filter @yujian/admin test:coverage` | 最终复核 15 个文件、126 项测试通过；语句 86.40%、分支 78.92%、函数 81.90%、行 89.63%，门槛均达标 |
| `pnpm --filter @yujian/admin typecheck`、`pnpm lint` | 最终复核通过 |
| `pnpm test:automation` | 3 项通过，包含管理端 E2E 入口、依赖版本一致性与结果归档 |
| 管理端生产构建 | 通过；Playwright 使用生产构建预览，避免开发服务器首次编译影响页面就绪 |
| 管理端 Playwright | 最终复核桌面与手机共 10 项通过，覆盖创建、编辑、素材、重复引用与排序、导出再导入、过期保护、内部引用与保存契约 |
| 公开站 Playwright（首版） | 11 项通过，包含 axe、键盘、响应式、试听、语言和 SEO |
| `pnpm verify:go`（首版） | 格式、生成、全包测试和 vet 通过 |
| `pnpm test:coverage:go`（首版） | 全包语句覆盖率 82.45%，达到 80% 门槛 |

审查期间已补齐列表作品类型，修复合法重复引用被误拒绝及相邻重复作品阻塞排序的问题，并增加单元、组件和浏览器回归。最终完整审查与独立复核未发现新的可确认缺陷。已检查浏览器窄屏截图与横向溢出断言，`git diff --check master...HEAD` 通过。

管理端保存 E2E 使用请求拦截，只验证浏览器与 API 的契约。未配置 `YUJIAN_TEST_POSTGRES_URL`，数据库集成测试按既有规则跳过；真实 OIDC、Go API、COS、EdgeOne 与域名联调尚未验证。本轮未执行部署。
