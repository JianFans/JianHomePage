# 片段全量目录实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 executing-plans 逐任务实现此计划。步骤使用复选框语法跟踪进度。

**目标：** 添加 `/moments/` 静态目录，展示所有发布快照片段，并接入首页入口、双语导航、SEO 与 Go 发布产物。

**架构：** 复用 Vue/Nuxt 页面、共享图片与站点导航组件。Nuxt 和现有 Goja SSR 渲染器都输出相同的目录 HTML，内容只来自不可变快照。

**技术栈：** Vue、Nuxt、TypeScript、Vitest、Playwright、Go、Goja。

**规格：** [片段全量目录设计](../specs/2026-11-20-moments-catalog-design.md)

## 全局约束

- `/moments/` 展示完整 `snapshot.moments`，不受首页 `enabled`、`itemIds` 或 `limit` 影响。
- 目录按 `snapshot.moments` 顺序展示。
- 外部链接使用安全的 `target` / `rel` 属性。
- 页面不请求运行时内容 API，不添加 Node.js 生产运行时。
- 内部导航按目标内容 ID 的类型指向对应锚点：宣传位和艺人资料为 `/#<id>`，发行和曲目为 `/music/#<id>`，影像为 `/video/#<id>`，现场为 `/events/#<id>`，片段为 `/moments/#<id>`。
- Nuxt 静态输出和 Go 导出均必须包含 `moments/index.html`。
- 公开站保持低饱和深色、响应式布局、图标优先和 `prefers-reduced-motion`。

## 审查重点（Review Focus）

- 首页精选数量少于全量片段时，目录仍须展示快照中的每条记录。
- 首页片段板块被停用或删除编排时，完整目录不能随之消失。
- 内容没有说明或跳转目标时，页面仍应完整显示图片且不产生错误链接。
- 内部锚点应按目标类型指向同源页面中真实存在的 ID，外部链接应隔离新标签页 opener。
- 缺少或加载失败的素材仍由现有 `FallbackImage` 提供可访问替代呈现。

---

### 任务 1：片段目录页面与入口

**文件：**
- 创建：`apps/web/utils/moments-catalog.ts`
- 创建：`apps/web/components/moment/MomentCatalog.vue`
- 创建：`apps/web/pages/moments/index.vue`
- 创建：`apps/web/test/unit/moments-catalog.test.ts`
- 创建：`apps/web/test/unit/moment-catalog-component.test.ts`
- 修改：`apps/web/components/home/MomentSection.vue`
- 修改：`apps/web/components/home/HomepageView.vue`
- 修改：`apps/web/components/site/SiteHeader.vue`
- 修改：`apps/web/pages/index.vue`
- 修改：`apps/web/standalone/App.vue`
- 修改：`apps/web/test/unit/home-page.test.ts`
- 修改：`apps/web/test/e2e/home.spec.ts`

- [x] **步骤 1：编写失败的测试**

在数据测试中断言 `/moments/` URL、双语页面元数据、按目标内容类型解析的同源页面锚点和快照顺序。组件测试构造超过首页片段上限的快照，断言所有记录均呈现，并覆盖本地化说明、无说明、外部安全链接、内部链接、无目标图像和空状态。

- [x] **步骤 2：运行测试验证失败**

运行：`pnpm --filter @yujian/web exec vitest run test/unit/moments-catalog.test.ts test/unit/moment-catalog-component.test.ts`

预期：测试因目录工具和组件不存在而失败。

- [x] **步骤 3：实现页面与入口**

建立 `/moments/` 目录组件和元数据工具。按快照数组顺序渲染所有有效 Moment 与其资产，使用 `FallbackImage` 展示媒体，并为每张卡片预留稳定宽高比；可选的素材宽高只作为尺寸提示。内部目标根据目标 ID 匹配的内容类型，链接到首页、`/music/`、`/video/`、`/events/` 或 `/moments/` 中对应的稳定锚点。外部目标使用安全新标签属性，无目标内容保持非链接状态。首页片段板块使用现有图标入口，目录导航标记当前页面。Nuxt 与 standalone 入口均提供组件所需的 `moments` 页面类型。

- [x] **步骤 4：运行单测与相关页面测试**

运行：`pnpm --filter @yujian/web exec vitest run test/unit/moments-catalog.test.ts test/unit/moment-catalog-component.test.ts test/unit/home-page.test.ts`

预期：所有目录数据、组件和首页入口测试通过。

---

### 任务 2：静态生成、Go 导出与文档

**文件：**
- 修改：`apps/web/utils/crawler-files.ts`
- 修改：`apps/web/nuxt.config.ts`
- 修改：`apps/web/standalone/client.ts`
- 修改：`apps/web/standalone/renderer.ts`
- 修改：`apps/server/internal/sitegen/generator.go`
- 修改：`apps/server/internal/sitegen/generator_test.go`
- 修改：`apps/web/test/unit/crawler-files.test.ts`
- 修改：`apps/web/test/unit/nuxt-config.test.ts`
- 修改：`apps/web/test/e2e/embedded.spec.ts`
- 修改：`scripts/verify-static-output.mjs`
- 修改：`scripts/verify-static-output.test.mjs`
- 修改：`README.md`
- 修改：`apps/web/EDGEONE.md`
- 修改：`docs/superpowers/specs/2026-11-20-moments-catalog-design.md`

- [x] **步骤 1：扩展生成路径测试**

断言 sitemap 包含 canonical `/moments/`，Nuxt 预渲染该路径，静态校验要求 `moments/index.html` 与有效本地资源；Go 测试断言完整片段目录、canonical、空状态、快照内容转义以及主页目录入口。

- [x] **步骤 2：运行最窄测试验证失败**

运行：`pnpm --filter @yujian/web exec vitest run test/unit/crawler-files.test.ts test/unit/nuxt-config.test.ts`

运行：`node --test scripts/verify-static-output.test.mjs`

预期：新增 moments 路径断言因静态生成配置缺失而失败。

- [x] **步骤 3：连接 Nuxt 与 Go 静态输出**

为 Nuxt sitemap 和 prerender 路由增加 `/moments/`。Standalone renderer 生成页面 HTML 和 `data-page="moments"` hydration 数据。Go `Generator.Generate` 校验 moments HTML 非空并写入 `moments/index.html`。静态产物校验器将其列入必需页面。同步 README 与 EdgeOne 页面缓存说明。

- [x] **步骤 4：运行 Web 单测、静态生成、Go 门禁与产物校验**

运行：`pnpm --filter @yujian/web test`

运行：`pnpm build:server:assets`

运行：`$env:GOCACHE = Join-Path $env:TEMP 'yujian-go-build-cache'; pnpm verify:go`

运行：`pnpm verify:static`

预期：Web 单测、Go 格式/生成/测试/vet 和静态产物校验全部通过，静态输出含 `moments/index.html`。

- [x] **步骤 5：审查并原子提交**

检查 `git diff --check` 与完整差异。仅暂存片段目录实现、对应测试和同步文档，然后提交：

```text
feat(公开站): 添加全部片段目录
```
