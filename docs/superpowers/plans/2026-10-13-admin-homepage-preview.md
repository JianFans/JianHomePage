# 管理端首页视觉预览实现计划

> 面向 AI 代理：使用 subagent-driven-development 与 TDD 实现，分阶段执行规格和质量审查。用户已授权独立完成并原子提交；使用当前功能分支，不创建额外工作树或远端发布。

**目标：** 在管理端安全预览已应用快照的真实首页，复用公开站展示逻辑。

**架构：** 共享 HomepageView，管理端 iframe 专用页面独立加载公开样式，通过严格的同源窗口消息传递有效快照和语言。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright、canonical Schema。

## 任务 1：共享渲染

文件：`apps/web/components/home/HomepageView.vue`、`apps/web/pages/index.vue`、`apps/web/components/home/HeroShowcase.vue`、`apps/web/test/unit/homepage-view.test.ts`。如控制器需要脱离 Nuxt，提取到 `apps/web/utils/audio-player.ts`，原 composable 保留公开导出与插件取用兼容。

- [x] 添加失败测试：显式传入快照与语言渲染六类板块；使用 generatedAt；发出语言切换及音乐试听意图；预览禁用视频自动播放，公开站默认行为不变。
- [x] 运行 `pnpm --filter @yujian/web test test/unit/homepage-view.test.ts`，确认功能缺失失败。
- [x] 抽取现有首页模板与纯计算，接口如下，不复制展示组件或契约。

```ts
defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
  player: AudioPlayerController
  autoplayEnabled?: boolean
}>()
defineEmits<{ toggleLocale: [] }>()
```

- [x] 页面保留 useHead、SEO、构建快照、语言偏好和 LocaleNotice；播放器 Dock 继续由公开站 app.vue 承担。
- [x] 窄测试、公开站单元测试与类型检查通过，检查 diff 后提交 `refactor(首页渲染): 提取显式快照展示边界`。

## 任务 2：预览消息与管理端交互

文件：`apps/admin/utils/homepage-preview.ts`、`apps/admin/components/HomepagePreview.vue`、`apps/admin/pages/preview.vue`、`apps/admin/pages/index.vue`、`apps/admin/nuxt.config.ts`、`apps/admin/composables/useAdminWorkspace.ts`；对应管理端 unit 测试。

- [x] 先写消息与生命周期失败测试，覆盖精确 origin/source、协议与语言拒绝、canonical 拒绝、只传快照和语言、过期画面撤下、关闭清理、未应用提示、加载失败可重试。
- [x] `useAdminWorkspace` 暴露只读的分析原文，供预览比较 `editorText === analyzedText`，复用既有 250 ms 分析。
- [x] 实现最小固定协议；接收方只能从预期窗口接收消息，构造时列举允许字段。

```ts
type PreviewLocale = 'zh-CN' | 'en'
type PreviewMessage =
  | { channel: 'yujian-homepage-preview-v1'; type: 'ready' }
  | { channel: 'yujian-homepage-preview-v1'; type: 'snapshot'; snapshot: YujianContentSnapshot; locale: PreviewLocale }
  | { channel: 'yujian-homepage-preview-v1'; type: 'locale'; locale: PreviewLocale }
```

- [x] 预览初始关闭；源变化立即撤下旧 iframe。有效且分析追上后挂载，握手成功才传快照。语言变化只传明确允许数据，未应用表单提示展示已应用内容。
- [x] `/preview` 不自行加载快照；单独导入公开站 CSS，首页入口导入管理端 CSS。预览创建独立播放器，接收有效新快照和销毁时 close，展示子树重新挂载，禁用 autoplay；阻止外链导航。
- [x] 添加桌面／手机、语言、关闭和重试图标控件及可访问名称；确保容器不会撑大页面。
- [x] 管理端测试与类型检查通过，检查暂存区提交 `feat(首页预览): 接入隔离的实时视觉预览`。

## 任务 3：浏览器回归、审查与说明

文件：`apps/admin/test/e2e/homepage-preview.spec.ts`、根和管理端 README、本计划与路线图。

- [x] 先新增 E2E 覆盖 iframe 实际渲染／消息、应用前后快照区别、非法 JSON、恢复、语言和尺寸、关闭重开、自动播放和样式隔离；运行确认预期行为，再补足真实缺陷。
- [x] 执行 `pnpm verify`、`pnpm test:coverage`、`pnpm test:automation`、管理端与公开站 `test:e2e`。顺序执行会更新同一 Nuxt 目录的命令，避免构建竞态。
- [x] 使用本地浏览器连接实际操作预览，检查控制台及手机布局。
- [x] 独立规格审查通过后再执行代码质量审查；修复确认问题并重跑相关验证。
- [x] 同步预览使用说明、限制和路线图状态；记录真实验证结果。
- [x] 分别提交浏览器回归与文档，完成 git diff/status 检查，保留功能分支，不自动推送或合并。

## 验收记录

本地验收完成于 2026-10-13，基线为主分支 `eff4de2`，工作保留在 `feat/admin-homepage-preview`。共享渲染、预览通信与交互、浏览器回归分别提交；没有修改 Go API、数据库、内容 Schema 或云部署配置。

### 验证结果

| 命令 | 结果 |
| --- | --- |
| `pnpm verify` | 退出码 0；lint、类型检查、Schema 19 项、管理端 208 项、公开站 105 项、脚本 21 项测试、静态生成与产物检查通过 |
| `pnpm test:coverage` | 退出码 0；三个工作区均通过现有覆盖率门禁，未降低阈值或排除新增生产代码 |
| `pnpm test:automation` | 退出码 0；3 项配置契约通过 |
| `pnpm --filter @yujian/admin test:e2e` | 退出码 0；生产构建下桌面与手机共 32 项通过，其中首页预览 16 项，包含未完成视频下载取消回归 |
| `pnpm --filter @yujian/web test:e2e` | 退出码 0；11 项通过，包含 axe、键盘、移动布局、单实例试听、语言与 SEO |

静态产物包含 18 个资源引用，首屏 JavaScript 为 287 KiB，未超过既有预算。覆盖率如下，顺序为语句、分支、函数、行：

| 工作区 | 语句 | 分支 | 函数 | 行 |
| --- | --- | --- | --- | --- |
| Schema | 90.47% | 76.92% | 95.91% | 94.51% |
| 管理端 | 88.82% | 82.44% | 86.04% | 91.43% |
| 公开站 | 89.35% | 79.09% | 91.75% | 89.42% |

### 回归修复与独立审查

- 失败单元测试复现迟到播放 Promise 在 A → B → A 轮播后覆盖当前状态；请求序号与元素身份共同过滤旧完成，当前播放拒绝仍可重试。
- 真实浏览器复现相邻宣传位复用视频素材时，界面显示暂停但原生视频仍在播放。切换前主动暂停旧媒体、重建视频并重新应用自动播放许可，旧元素事件也需核对身份；对应单元测试和真实浏览器回归均通过。
- 后续审查用本地慢流复现：只暂停并移除旧视频元素仍会继续下载。先补来源释放单元断言和慢流 E2E，两者均因旧来源或连接残留失败；切换前移除旧来源并调用 load() 后，窄测试通过。保留视频重建与旧事件、Promise 保护，独立复审未发现新增问题。
- 独立规格与代码质量审查检查了完整分支集成和最终 E2E，未发现仍需修复的阻塞问题。

### 实际使用与边界

通过本地浏览器连接实际输入有效快照，检查六类板块、1280／390 真实预览视口、缩放与样式隔离；操作语言切换、表单编辑和应用、无效 JSON 撤帧与恢复，以及关闭预览。表单应用前显示旧内容，应用后显示新标题；预览操作不改变 URL 或持久化语言偏好。

原生媒体 E2E 使用请求拦截提供本地 H.264 与 WAV，验证播放、同素材切换，以及更新和关闭后旧音视频暂停、来源属性清空、有限资源请求完成。另用本地 HTTP 慢流服务器检测未完成响应：切换后新连接存在、旧连接消失，关闭预览后当前连接也消失；该测试不保留旧 DOM 引用或模拟媒体方法。独立原生实验确认，iframe 移除后保留的媒体对象可能仍返回历史 `currentSrc`，因此不把该历史值误当成播放或网络泄漏。测试不证明真实云端取消、浏览器内部缓冲释放或其他浏览器行为；防抖时序和 `pageshow` 恢复由单元测试覆盖，尚未在真实浏览器中独立验证 BFCache 往返或 249／250 ms 精确边界。

实际浏览器中的公网测试素材存在连接或证书错误，降级展示正常；同源 sandbox 的浏览器警告与规格声明一致，仅提供文档和样式隔离。本轮没有验证真实 OIDC、Go API、COS 或 EdgeOne，也未运行正式快照的 `pnpm verify:edgeone`；未修改服务端，未重复 Go 与 Docker 门禁。代码与本地门禁通过，但真实云集成尚未验证。
