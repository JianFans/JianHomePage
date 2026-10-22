# 交互动效与异步状态实现计划

> **面向 AI 代理的工作者：** 使用 executing-plans，按复选框追踪。用户授权独立完成设计、实现与审查，不等待确认。

**目标：** 为前后台异步操作、媒体和组件加载建立真实、稳定、可访问的动效反馈。

**架构：** 共用 ActivityIndicator 和动效参数，管理端 AsyncButton／LoadingSkeleton 复用已有状态；视图只呈现请求状态，不改变后端协议。

**技术栈：** Vue、Nuxt、CSS、Vitest、Playwright、Go 内嵌 Vue SSR。

**规格：** [交互动效与异步状态设计](../specs/2026-10-22-interaction-motion-states-design.md)。

## 全局约束

- 分支 feat/interaction-motion-states 基于 a22593e；不推送、不部署，原子化签名提交。
- 140／220／420 ms 既有时长；加载旋转 900 ms，骨架呼吸 1600 ms。
- 不等待动画才发送请求、不虚构百分比、不保留离场媒体，不增加依赖。
- 实时减少动态效果、键盘可达、稳定尺寸、禁用 JS 可读。

## 审查重点（Review Focus）

- 快速完成、错误与重试不得留下转圈状态或重复请求。
- 非当前操作按钮不得错误显示 loading；角色、disabled 与表单提交行为不得改变。
- 版本与预览迟到响应不得使旧内容重新可交互。
- 刷新列表期间不销毁已有输入或显示错误的空结果。
- 缓存图片、picture 响应式源、失败与换源时不产生隐藏图片或过期事件污染。

## 文件职责

- apps/web/components/ui/ActivityIndicator.vue 与 motion-tokens.css：共享加载图形与循环参数。
- apps/admin/components/AsyncButton.vue、LoadingSkeleton.vue、StudioLoading.vue：按钮及等待占位。
- 管理端 index、StudioVersionWorkflow、StudioContent、StudioNavigation、AssetWorkbench、HomepagePreview、PublishStatusPanel：对应真实异步状态。
- apps/web/components/ui/FallbackImage.vue、home/MusicSection.vue、player/AudioDock.vue：媒体加载反馈。
- 两端 motion.css：记录／细节交互反馈与减少动态效果。
- 对应 unit 和 e2e：语义、状态、事件竞争和真实浏览器动画验证。

### 任务 1：共享反馈组件与异步按钮

- [x] 写 AsyncButton 单元测试：名字不变、loading 禁用且点击不派发、disabled 合并、type／attrs 透传；运行确认失败。
- [x] 实现 ActivityIndicator、AsyncButton、LoadingSkeleton 与循环参数；以 CSS 静态回退支持减少动态效果。
- [x] 将保存／复制／审核／发布确认／导航退出／列表刷新分页改用 AsyncButton，局部操作标识区分按钮；添加壳层慢请求断言。
- [x] 运行管理端单元测试、typecheck、lint，通过后签名提交。

### 任务 2：工作区、素材、预览与发布组件加载

- [x] 添加初始加载、保留旧列表、预览等待和 read-only pending 的失败测试。
- [x] 实现 StudioLoading、版本读取指示、素材与历史骨架，接入素材按钮；预览等待移入视口并在 ready 前 inert；发布状态图标和状态淡入。
- [x] 保留错误、重试、滚动和输入，补充 editor details／record 的渐进动效。
- [x] 运行管理端单元和定向 E2E，确认慢保存不抖动、按钮不重复提交、减少动态效果停止循环；原子提交。

### 任务 3：公开站媒体反馈

- [x] 为 FallbackImage 写加载、缓存、失败、换源旧事件测试并确认失败；为音乐与播放器加载 aria-busy 添加测试。
- [x] 实现图片状态背景／淡入、共用音频加载图形；保留原生 img／picture 和 SSR 可见性。
- [x] 运行 web 单元、typecheck 和 E2E，含延迟图片／音频、实时 reduce 和无 JS；原子提交。

### 任务 4：审查、门禁与文档

- [x] 同步管理端 README、公开站动效说明与规格事实。
- [x] 完整两端 E2E、pnpm verify、pnpm test:embedded（先构建内嵌资源），禁止并发构建覆盖同一 .output。
- [x] 以 requesting-code-review 进行一次独立整分支审查，验证有效问题后 TDD 修复。
- [x] 检查工作区、diff、签名及计划完成记录；保留本地分支，不推送或部署。

## 验证与审查结果

- `pnpm verify` 通过：lint、类型检查、615 项模块测试、脚本测试、静态生成和产物检查。
- 覆盖率门禁通过：Schema、管理端、公开站语句覆盖率分别为 90.62%、89.89%、90.69%。并发运行时公开站有一次 Nuxt 初始化钩子超时，单独以 `--maxWorkers=2` 运行完整公开站覆盖率套件后 117 项全部通过，未放宽超时或断言。
- 公开站完整 E2E：20 项通过、1 项跳过。管理端完整 E2E 初次 173 项通过，3 项失败；修复错误定位与短动画观察时机后，桌面／手机受影响套件 34 项全部通过。
- `pnpm build:server:assets`、发布／Pages／sitegen／adminweb／webassets／cmd/api 六个 Go 包测试通过；`pnpm test:embedded` 的 21 项真实浏览器检查全部通过，涵盖 Go 导出、语言、无运行时内容 API、媒体等待、动效、减少动态效果和无障碍。
- 独立审查发现两项有效问题：确认重试按钮卸载且错误点亮上传按钮，以及初始化前失败图片保留等待态。均先复现再修复；完整测试另发现动画取消的 Promise 拒绝和折叠区错误字段聚焦问题，已补充验证。
- 未扩展本轮范围的既有边界：`picture` 候选源失败回退、studio 素材首次读取失败后的空列表文案。两者均存在于基线，现有候选源与通知行为保持不变。
- 本轮未修改后端协议或云配置，不推送、不部署；真实云集成不属于本地验证结论。
