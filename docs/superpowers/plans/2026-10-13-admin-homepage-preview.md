# 管理端首页视觉预览实现计划

> 面向 AI 代理：使用 subagent-driven-development 与 TDD 实现，分阶段执行规格和质量审查。用户已授权独立完成并原子提交；使用当前功能分支，不创建额外工作树或远端发布。

**目标：** 在管理端安全预览已应用快照的真实首页，复用公开站展示逻辑。

**架构：** 共享 HomepageView，管理端 iframe 专用页面独立加载公开样式，通过严格的同源窗口消息传递有效快照和语言。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright、canonical Schema。

## 任务 1：共享渲染

文件：`apps/web/components/home/HomepageView.vue`、`apps/web/pages/index.vue`、`apps/web/components/home/HeroShowcase.vue`、`apps/web/test/unit/homepage-view.test.ts`。如控制器需要脱离 Nuxt，提取到 `apps/web/utils/audio-player.ts`，原 composable 保留公开导出与插件取用兼容。

- [ ] 添加失败测试：显式传入快照与语言渲染六类板块；使用 generatedAt；发出语言切换及音乐试听意图；预览禁用视频自动播放，公开站默认行为不变。
- [ ] 运行 `pnpm --filter @yujian/web test test/unit/homepage-view.test.ts`，确认功能缺失失败。
- [ ] 抽取现有首页模板与纯计算，接口如下，不复制展示组件或契约。

```ts
defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
  player: AudioPlayerController
  autoplayEnabled?: boolean
}>()
defineEmits<{ toggleLocale: [] }>()
```

- [ ] 页面保留 useHead、SEO、构建快照、语言偏好和 LocaleNotice；播放器 Dock 继续由公开站 app.vue 承担。
- [ ] 窄测试、公开站单元测试与类型检查通过，检查 diff 后提交 `refactor(首页渲染): 提取显式快照展示边界`。

## 任务 2：预览消息与管理端交互

文件：`apps/admin/utils/homepage-preview.ts`、`apps/admin/components/HomepagePreview.vue`、`apps/admin/pages/preview.vue`、`apps/admin/pages/index.vue`、`apps/admin/nuxt.config.ts`、`apps/admin/composables/useAdminWorkspace.ts`；对应管理端 unit 测试。

- [ ] 先写消息与生命周期失败测试，覆盖精确 origin/source、协议与语言拒绝、canonical 拒绝、只传快照和语言、过期画面撤下、关闭清理、未应用提示、加载失败可重试。
- [ ] `useAdminWorkspace` 暴露只读的分析原文，供预览比较 `editorText === analyzedText`，复用既有 250 ms 分析。
- [ ] 实现最小固定协议；接收方只能从预期窗口接收消息，构造时列举允许字段。

```ts
type PreviewLocale = 'zh-CN' | 'en'
type PreviewMessage =
  | { channel: 'yujian-homepage-preview-v1'; type: 'ready' }
  | { channel: 'yujian-homepage-preview-v1'; type: 'snapshot'; snapshot: YujianContentSnapshot; locale: PreviewLocale }
  | { channel: 'yujian-homepage-preview-v1'; type: 'locale'; locale: PreviewLocale }
```

- [ ] 预览初始关闭；源变化立即撤下旧 iframe。有效且分析追上后挂载，握手成功才传快照。语言变化只传明确允许数据，未应用表单提示展示已应用内容。
- [ ] `/preview` 不自行加载快照；单独导入公开站 CSS，首页入口导入管理端 CSS。预览创建独立播放器，接收有效新快照和销毁时 close，展示子树重新挂载，禁用 autoplay；阻止外链导航。
- [ ] 添加桌面／手机、语言、关闭和重试图标控件及可访问名称；确保容器不会撑大页面。
- [ ] 管理端测试与类型检查通过，检查暂存区提交 `feat(首页预览): 接入隔离的实时视觉预览`。

## 任务 3：浏览器回归、审查与说明

文件：`apps/admin/test/e2e/homepage-preview.spec.ts`、根和管理端 README、本计划与路线图。

- [ ] 先新增 E2E 覆盖 iframe 实际渲染／消息、应用前后快照区别、非法 JSON、恢复、语言和尺寸、关闭重开、自动播放和样式隔离；运行确认预期行为，再补足真实缺陷。
- [ ] 执行 `pnpm verify`、`pnpm test:coverage`、`pnpm test:automation`、管理端与公开站 `test:e2e`。顺序执行会更新同一 Nuxt 目录的命令，避免构建竞态。
- [ ] 使用本地浏览器连接实际操作预览，检查控制台及手机布局。
- [ ] 独立规格审查通过后再执行代码质量审查；修复确认问题并重跑相关验证。
- [ ] 同步预览使用说明、限制和路线图状态；记录真实验证结果。
- [ ] 分别提交浏览器回归与文档，完成 git diff/status 检查，保留功能分支，不自动推送或合并。
