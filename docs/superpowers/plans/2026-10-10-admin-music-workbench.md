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

- [ ] 先写新增作品与曲目、更新和排序、重复 ID、跨作品曲目、移除被引用曲目、素材类型及首页隐藏目标测试。
- [ ] 运行 `pnpm --filter @yujian/admin test test/unit/music-workbench.test.ts`，确认功能缺失导致失败。
- [ ] 实现 `createMusicDraft(snapshot, releaseId)` 和 `applyMusicDraft(text, baseline, draft)`。应用结果包含原文或新文本、稳定错误代码及共享诊断；所有失败保持原文。
- [ ] 同步 `release.trackIds` 与曲目归属；只修改当前作品、其原有曲目及显式编辑的音乐板块条目，不变更其他内容。
- [ ] 重新运行目标测试、管理端类型检查与目标文件 lint，再提交 `feat(音乐编辑): 添加快照事务与引用校验`。

## 任务 2：状态与未保存保护

文件：`apps/admin/composables/useMusicWorkspace.ts`、`apps/admin/composables/useAdminWorkspace.ts` 及对应 `test/unit` 测试。

- [ ] 先测试未应用输入、取消、过期基线、忙碌门禁，以及保存冲突保持文本和修订号。
- [ ] 运行目标测试确认失败，再实现打开、应用、丢弃和切换请求。切换前通过内联继续编辑／放弃提示保留决定权。
- [ ] 音乐列表复用工作区防抖分析；打开与应用直接校验最新文本，保存直接分析当前文本。
- [ ] 记录服务端文本基线，区分本地未保存快照与未应用表单。保存期间阻止重入、载入、导入、素材插入和表单改动。
- [ ] 重新运行 `admin-workspace.test.ts`、`music-workspace.test.ts` 与类型检查，再提交 `feat(音乐编辑): 保护临时表单和未保存快照`。

## 任务 3：界面

文件：`apps/admin/components/MusicWorkbench.vue`、`LocalizedTextFields.vue`、`PlatformLinkFields.vue`、`MusicTrackFields.vue`、`apps/admin/utils/music-copy.ts`、`apps/admin/pages/index.vue`、`apps/admin/components/AssetWorkbench.vue` 及对应组件测试。

- [ ] 先测试作品选择、新建、应用、取消、双语标签、素材过滤和禁用状态，再运行组件测试确认缺少行为。
- [ ] 实现作品列表与表单，独立小组件复用双语字段、平台链接和曲目编辑，避免单文件承担全部字段。
- [ ] 曲目排序与音乐板块排序采用可访问的上下移动按钮。保留原 ID 和选填文案；隐藏值不得因切换界面语言被清除。
- [ ] 接入页面未保存提示、载入／导入保护和 `beforeunload`。素材插入遵守保存忙碌状态。
- [ ] 重新运行管理端测试、类型检查、lint，再提交 `feat(音乐工作台): 接入双语作品曲目表单`。

## 任务 4：浏览器与 CI

文件：`apps/admin/playwright.config.ts`、`apps/admin/test/e2e/music-workbench.spec.ts`、`apps/admin/package.json`、`pnpm-lock.yaml`、`.github/workflows/code-check.yml`、`scripts/automation-config.test.mjs`、`apps/admin/README.md`。

- [ ] 先写浏览器测试：导入 fixture、新增作品与曲目、绑定封面／音频、排序、应用、导出并重新校验；另测无效与过期编辑、保存请求修订与冲突。
- [ ] 新增管理端 E2E 脚本，Playwright 版本与公开站一致；Vitest include 收窄到单元测试以避免加载 Playwright 测试。
- [ ] 先扩展自动化契约测试要求管理端 E2E 命令与结果归档，确认失败，再接入现有 E2E job；不把依赖版本约束写进工作流。
- [ ] 桌面与窄屏浏览器运行验证布局、键盘入口和实际字段行为。拦截 API 的测试明确标注为契约测试，不冒充真实服务端或云集成。
- [ ] 同步 README 的操作路径、未保存行为、支持范围和验证命令，再提交 `test(音乐工作台): 补齐浏览器回归和持续验证`。

## 最终门禁

- [ ] `pnpm --filter @yujian/admin build`
- [ ] `pnpm test:coverage`
- [ ] `pnpm test:automation`
- [ ] `pnpm verify`
- [ ] `pnpm verify:go`
- [ ] `pnpm --filter @yujian/admin test:e2e`
- [ ] `pnpm --filter @yujian/web test:e2e`
- [ ] 对比 `master...HEAD` 完整审查，修复实际问题后重新验证受影响范围。
- [ ] 每次提交前检查暂存文件和 `git diff --cached --check`；结束时报告提交、验证与真实环境缺口。
