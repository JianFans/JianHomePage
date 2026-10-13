# 宣传位与首页编排实现计划

> 面向 AI 代理：按执行计划与 TDD 在功能分支逐任务实现，委派独立审查；用户已授权独立完成流程。

**目标：** 无需手改 JSON 即可维护宣传位和已有首页板块，并安全保存草稿。

**架构：** 首页临时草稿原子替换宣传位与编排；页面汇总多工作台未保存状态，JSON 继续是唯一内容源。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright、canonical Schema。

## 任务 1：事务与展示规则

- [ ] 新增 `apps/admin/test/unit/homepage-workbench.test.ts`，测试深复制、新增宣传位并加入、删除引用、板块顺序与身份保护、重复引用、素材与内部目标校验、过期文本及失败保留原文。
- [ ] 运行 `pnpm --filter @yujian/admin test test/unit/homepage-workbench.test.ts`，确认功能缺失失败。
- [ ] 新增 `apps/admin/utils/homepage-workbench.ts`：

```ts
interface HomepageDraft {
  heroSlides: HeroSlide[]
  sections: YujianContentSnapshot['homepage']['sections']
}
// createHomepageDraft(snapshot), createHeroSlide(), removeHomepageHero(draft, id)
// applyHomepageDraft(text, baseline, draft) -> { text, error, issues }
// homepageItemOptions(snapshot, draft, section) -> { id, title }[]
// homepageItemVisibility(snapshot, draft, section) -> ('visible' | 'disabled' | 'limit' | 'time-window')[]
```

- [ ] 展示状态测试对照公开站 `resolveHomepageSections` 的 generatedAt、过滤顺序及重复引用行为。
- [ ] 运行目标测试与 ESLint，提交 `feat(首页编辑): 添加宣传位与编排原子事务`。

## 任务 2：工作台状态与页面交接

- [ ] 新增 `apps/admin/test/unit/homepage-workspace.test.ts`，先验证打开、脏状态、应用、过期、失败保留、关闭／重载确认和 busy；扩展 `admin-shell.test.ts` 验证多表单保存、导出与替换保护。
- [ ] 运行对应测试确认失败，再新增 `apps/admin/composables/useHomepageWorkspace.ts`，复用音乐工作台状态模式但不重构音乐模块。
- [ ] 更新 `apps/admin/pages/index.vue` 的 unsaved、保存、载入／导入成功清理和保存成功刷新基线；两个草稿互不覆盖。
- [ ] 验证状态与页面测试、类型检查，提交 `feat(首页编辑): 保护多工作台草稿交接`。

## 任务 3：双语表单和编排

- [ ] 先新增 `apps/admin/test/unit/homepage-workbench-component.test.ts`，验证宣传位各字段、重复位置排序、数量限制、时间状态、双语和忙碌禁用。
- [ ] 创建 `HomepageWorkbench.vue`、`HeroSlideFields.vue`、`HomepageSectionFields.vue` 和 `apps/admin/utils/homepage-copy.ts`；复用 LocalizedTextFields 与 PlatformLinkFields。
- [ ] 页面接入 `HomepageWorkbench`：v-model:draft、snapshot、locale、busy、dirty、stale、error、issues、pending；事件 open/apply/close/keep/discard。busy 同时涵盖服务端操作，表单内部所有修改受 fieldset 禁用保护。
- [ ] 使用紧凑列表和原生控件，所有图标按钮提供无障碍名称，沿用 CSS 变量并添加窄屏布局。
- [ ] 运行组件、页面测试、类型检查与 lint，提交 `feat(首页工作台): 接入宣传位和首页编排表单`。

## 任务 4：浏览器、审查和文档

- [ ] 新增 `apps/admin/test/e2e/homepage-workbench.spec.ts`，通过实际导入和字段操作创建宣传位、加入板块、应用、导出再导入，并验证双工作台过期、替换、双语、键盘与窄屏。
- [ ] 运行 `pnpm --filter @yujian/admin test:e2e`，使用既有生产构建与桌面／手机项目。
- [ ] 运行管理端覆盖率、`pnpm verify`、`pnpm test:coverage`、`pnpm test:automation`、公开站 E2E；无 Go 改动时不把已有 Go 证据写成新验证。
- [ ] 独立审查设计合规性与完整分支代码质量，修复真实问题后重跑受影响门禁。
- [ ] 同步管理端与根 README、路线图和本计划验收记录；提交浏览器测试与文档各自职责。
- [ ] 检查全部 diff 和工作区，报告提交与真实环境缺口；不自动推送、合并或部署。
