# 宣传位与首页编排实现计划

> 面向 AI 代理：按执行计划与 TDD 在功能分支逐任务实现，委派独立审查；用户已授权独立完成流程。

**目标：** 无需手改 JSON 即可维护宣传位和已有首页板块，并安全保存草稿。

**架构：** 首页临时草稿原子替换宣传位与编排；页面汇总多工作台未保存状态，JSON 继续是唯一内容源。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright、canonical Schema。

## 任务 1：事务与展示规则

- [x] 新增 `apps/admin/test/unit/homepage-workbench.test.ts`，测试深复制、新增宣传位并加入、删除引用、板块顺序与身份保护、重复引用、素材与内部目标校验、过期文本及失败保留原文。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/homepage-workbench.test.ts`，确认功能缺失失败。
- [x] 新增 `apps/admin/utils/homepage-workbench.ts`：

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

- [x] 展示状态测试对照公开站 `resolveHomepageSections` 的 generatedAt、过滤顺序及重复引用行为。
- [x] 运行目标测试与 ESLint，提交 `feat(首页编辑): 添加宣传位与编排原子事务`。

## 任务 2：工作台状态与页面交接

- [x] 新增 `apps/admin/test/unit/homepage-workspace.test.ts`，先验证打开、脏状态、应用、过期、失败保留、关闭／重载确认和 busy；扩展 `admin-shell.test.ts` 验证多表单保存、导出与替换保护。
- [x] 运行对应测试确认失败，再新增 `apps/admin/composables/useHomepageWorkspace.ts`，复用音乐工作台状态模式但不重构音乐模块。
- [x] 更新 `apps/admin/pages/index.vue` 的 unsaved、保存、载入／导入成功清理和保存成功刷新基线；两个草稿互不覆盖。
- [x] 验证状态与页面测试、类型检查；状态层提交 `feat(首页编辑): 保护临时草稿与过期基线`，页面交接与任务 3 的界面一起提交，保持每次提交可独立编译。

## 任务 3：双语表单和编排

- [x] 先新增 `apps/admin/test/unit/homepage-workbench-component.test.ts`，验证宣传位各字段、重复位置排序、数量限制、时间状态、双语和忙碌禁用。
- [x] 创建 `HomepageWorkbench.vue`、`HeroSlideFields.vue`、`HomepageSectionFields.vue` 和 `apps/admin/utils/homepage-copy.ts`；复用 LocalizedTextFields 和平台字段规则，单链接直接绑定稳定对象。
- [x] 页面接入 `HomepageWorkbench`：v-model:draft、snapshot、locale、busy、dirty、stale、error、issues、pending；事件 open/apply/close/keep/discard。busy 同时涵盖服务端操作，表单内部所有修改受 fieldset 禁用保护。
- [x] 使用紧凑列表和原生控件，所有图标按钮提供无障碍名称，沿用 CSS 变量并添加窄屏布局。
- [x] 运行组件、页面测试、类型检查与 lint，提交 `feat(首页工作台): 接入宣传位和首页编排表单`。

## 任务 4：浏览器、审查和文档

- [x] 新增 `apps/admin/test/e2e/homepage-workbench.spec.ts`，通过实际导入和字段操作创建宣传位、加入板块、应用、导出再导入，并验证双工作台过期、替换、双语、键盘与窄屏。
- [x] 运行 `pnpm --filter @yujian/admin test:e2e`，使用既有生产构建与桌面／手机项目。
- [x] 运行管理端覆盖率、`pnpm verify`、`pnpm test:coverage`、`pnpm test:automation`、公开站 E2E；无 Go 改动时不把已有 Go 证据写成新验证。
- [x] 独立审查设计合规性与完整分支代码质量，修复真实问题后重跑受影响门禁。
- [x] 同步管理端与根 README、路线图和本计划验收记录；提交浏览器测试与文档各自职责。
- [x] 检查全部 diff 和工作区，报告提交与真实环境缺口；不自动推送、合并或部署。

## 验收记录

- 事务测试 43 项、状态测试 6 项、首页组件测试 15 项；管理端共 193 项单元测试通过。
- `pnpm verify` 通过：lint、工作区类型检查、全部单元测试、Schema 生成、Nuxt 静态生成和静态产物校验；公开站 18 个资源引用，首屏 JavaScript 286 KiB。
- `pnpm test:coverage` 通过全部工作区门禁；管理端语句 87.62%、分支 81.59%、函数 84.56%、行 90.43%。`pnpm test:automation` 的 3 项配置契约测试通过，公开站 E2E 的 11 项测试通过。
- `pnpm --filter @yujian/admin test:e2e` 的 16 项测试通过，涵盖桌面与 Pixel 7 手机尺寸；其中首页工作台新增 3 个场景，在两个项目各运行一次。
- 独立审查确认 canonical 允许重复板块 ID，身份保护改为多重集合并补充回归；重排板块时清除待添加选项，避免组件按位置复用后残留选择。两项修正均先复现失败，再验证修复。
- Chrome 真实交互验证了导入 JSON、双语字段编辑、应用前原文保持、导出保护及应用结果；桌面和窄屏无横向溢出。手机尺寸和真实文件输入另由 Playwright 覆盖。
- 本轮不修改 Schema、Go API、数据库或公开站行为；未执行 Go、Docker 或真实云环境验证。浏览器保存测试拦截 API，仅证明请求与状态契约；OIDC、Go 服务、COS 和 EdgeOne 的真实集成仍待后续验证。实际首页视觉预览属于阶段 B，当前预览仍为 JSON。
