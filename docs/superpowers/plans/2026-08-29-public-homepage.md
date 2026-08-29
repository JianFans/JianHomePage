# 「遇健我」公开首页实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 构建可从版本化内容快照生成、可独立部署到 EdgeOne Pages 的「遇健我」响应式双语首页，并完成试听、媒体降级、可访问性和静态产物验证。

**架构：** `packages/schema` 提供 JSON Schema、生成类型和双语 fixture；`apps/web` 使用 Nuxt 静态生成模式读取构建时快照。首页组件只消费规范化内容，不调用运行时内容 API；国际化、播放器和媒体降级通过小型 composable 隔离并单独测试。

**技术栈：** pnpm workspace、Nuxt、Vue 3、TypeScript、Vue I18n、`@lucide/vue`、AJV、Vitest、Vue Test Utils、Playwright、axe-core。

---

## 文件结构

```text
AGENTS.md                                      仓库导航、常用命令和生成文件约束
package.json                                   根脚本与工具版本
pnpm-workspace.yaml                            workspace 成员
tsconfig.base.json                             TypeScript 严格模式基线
eslint.config.mjs                              根静态检查配置
content/fixtures/homepage.json                 双语首页测试内容
packages/schema/package.json                   内容契约包
packages/schema/schema/content-snapshot.schema.json
packages/schema/scripts/generate.mjs           生成 TypeScript 类型
packages/schema/src/generated.ts               自动生成类型
packages/schema/src/index.ts                   契约包公开入口
packages/schema/test/snapshot.test.ts           fixture 正反例测试
apps/web/package.json                          Nuxt 应用依赖与命令
apps/web/nuxt.config.ts                        静态生成、SEO、模块与 CSP 配置
apps/web/app.vue                               应用壳与播放器挂载点
apps/web/pages/index.vue                       首页板块编排
apps/web/assets/css/main.css                   设计 Token、全局排版与响应式基础
apps/web/plugins/i18n.client.ts                浏览器语言检测与偏好持久化
apps/web/plugins/i18n.server.ts                默认中文 SSR/SSG 实例
apps/web/composables/useLocale.ts              语言切换和提示状态
apps/web/composables/useContentSnapshot.ts     构建时快照读取
apps/web/composables/useAudioPlayer.ts          单实例播放器状态
apps/web/utils/localized.ts                    多语言字段回退
apps/web/utils/sections.ts                     板块显隐和引用解析
apps/web/components/ui/IconButton.vue           图标按钮、Tooltip 和焦点状态
apps/web/components/ui/PlatformLinks.vue        已知平台注册表与窄屏菜单
apps/web/components/site/SiteHeader.vue         品牌与图标导航
apps/web/components/home/HeroShowcase.vue       图片/视频大封面
apps/web/components/home/MusicSection.vue       音乐封面与试听入口
apps/web/components/home/VideoSection.vue       不对称影像布局
apps/web/components/home/EventSection.vue       现场列表与空板块隐藏
apps/web/components/home/MomentSection.vue      图片片段布局
apps/web/components/home/ArtistSection.vue      精简简介与官方平台
apps/web/components/player/AudioDock.vue        底部迷你播放器
apps/web/public/media/*                         非人物占位视觉与试听样本
apps/web/test/setup.ts                          Vitest DOM 环境
apps/web/test/unit/*.test.ts                    工具、composable 与组件测试
apps/web/test/e2e/home.spec.ts                  桌面与移动端主流程
apps/web/test/e2e/accessibility.spec.ts         axe 和键盘验证
apps/web/playwright.config.ts                   浏览器、端口与截图配置
apps/web/EDGEONE.md                             Pages 构建参数与发布说明
```

## 任务 1：建立 workspace 与测试入口

**文件：**

- 创建：`package.json`
- 创建：`pnpm-workspace.yaml`
- 创建：`tsconfig.base.json`
- 创建：`eslint.config.mjs`
- 创建：`AGENTS.md`
- 创建：`apps/web/package.json`
- 创建：`apps/web/nuxt.config.ts`
- 创建：`apps/web/tsconfig.json`
- 创建：`apps/web/vitest.config.ts`
- 创建：`apps/web/test/setup.ts`
- 测试：`apps/web/test/unit/app-shell.test.ts`

- [x] **步骤 1：创建 workspace 配置和依赖清单**

根 `package.json` 固定 `pnpm@11.23.0`，提供 `dev`、`test`、`lint`、`typecheck`、`generate` 和 `verify` 脚本。`apps/web/package.json` 声明 Nuxt、Vue、Vue I18n、Lucide、Vitest、Vue Test Utils、Playwright 和 axe-core。

- [x] **步骤 2：安装依赖**

运行：

```bash
pnpm install
```

预期：生成 `pnpm-lock.yaml`，命令退出码为 `0`。

- [x] **步骤 3：编写失败的应用壳测试**

```ts
// apps/web/test/unit/app-shell.test.ts
import { mountSuspended } from '@nuxt/test-utils/runtime'
import App from '../../app.vue'

describe('应用壳', () => {
  it('提供主内容区域和全局播放器挂载点', async () => {
    const wrapper = await mountSuspended(App)
    expect(wrapper.find('main').exists()).toBe(true)
    expect(wrapper.find('[data-testid="audio-dock-host"]').exists()).toBe(true)
  })
})
```

- [x] **步骤 4：运行测试并确认因应用壳缺失而失败**

运行：

```bash
pnpm --filter @yujian/web test -- app-shell.test.ts
```

预期：FAIL，提示无法解析 `apps/web/app.vue`。

- [x] **步骤 5：实现最小应用壳**

```vue
<!-- apps/web/app.vue -->
<template>
  <main>
    <NuxtPage />
  </main>
  <div data-testid="audio-dock-host" />
</template>
```

- [x] **步骤 6：验证测试、类型和静态检查**

运行：

```bash
pnpm --filter @yujian/web test -- app-shell.test.ts
pnpm --filter @yujian/web typecheck
pnpm lint
```

预期：全部通过且没有警告。

- [x] **步骤 7：提交**

```bash
git add AGENTS.md package.json pnpm-workspace.yaml pnpm-lock.yaml tsconfig.base.json eslint.config.mjs apps/web
git commit -m "chore(工程): 初始化 Nuxt 工作区"
```

## 任务 2：建立版本化内容契约

**文件：**

- 创建：`packages/schema/package.json`
- 创建：`packages/schema/schema/content-snapshot.schema.json`
- 创建：`packages/schema/scripts/generate.mjs`
- 创建：`packages/schema/src/generated.ts`
- 创建：`packages/schema/src/index.ts`
- 创建：`packages/schema/test/snapshot.test.ts`
- 创建：`content/fixtures/homepage.json`

- [ ] **步骤 1：编写失败的 fixture 契约测试**

```ts
// packages/schema/test/snapshot.test.ts
import { readFileSync } from 'node:fs'
import Ajv from 'ajv'
import schema from '../schema/content-snapshot.schema.json'

const fixture = JSON.parse(
  readFileSync(new URL('../../../content/fixtures/homepage.json', import.meta.url), 'utf8'),
)

describe('首页快照契约', () => {
  const validate = new Ajv({ allErrors: true, formats: false }).compile(schema)

  it('接受完整双语 fixture', () => {
    expect(validate(fixture), JSON.stringify(validate.errors)).toBe(true)
  })

  it('拒绝没有版本号的快照', () => {
    const { schemaVersion: _, ...invalid } = fixture
    expect(validate(invalid)).toBe(false)
  })

  it('拒绝非 HTTPS 的外部平台链接', () => {
    const invalid = structuredClone(fixture)
    invalid.site.socialLinks[0].url = 'http://example.com'
    expect(validate(invalid)).toBe(false)
  })
})
```

- [ ] **步骤 2：运行测试并确认因 Schema 与 fixture 缺失而失败**

运行：

```bash
pnpm --filter @yujian/schema test
```

预期：FAIL，提示找不到 Schema 或 fixture。

- [ ] **步骤 3：实现 JSON Schema 和双语 fixture**

Schema 必须完整定义 `SiteConfig`、`HomepageSection`、`HeroSlide`、`Release`、`Track`、`Video`、`Event`、`Moment`、`ArtistProfile`、`Asset`、`LocalizedText` 和 `PlatformLink`。所有外部 URL 使用 `^https://` 约束；首页板块使用判别联合 `type`；未知属性默认拒绝。

fixture 使用稳定 ID，包含 3 个大封面、5 个音乐作品、3 个影像、2 个现场、3 个片段和音乐人信息。示例内容明确放在 `content/fixtures`，不声称是真实履历或发行信息。

- [ ] **步骤 4：生成 TypeScript 类型**

```js
// packages/schema/scripts/generate.mjs
import { compileFromFile } from 'json-schema-to-typescript'
import { mkdir, writeFile } from 'node:fs/promises'

const output = await compileFromFile(
  new URL('../schema/content-snapshot.schema.json', import.meta.url),
  { bannerComment: '/* 此文件由 pnpm schema:generate 生成，请勿手工修改。 */' },
)
await mkdir(new URL('../src', import.meta.url), { recursive: true })
await writeFile(new URL('../src/generated.ts', import.meta.url), output)
```

- [ ] **步骤 5：验证契约、生成文件和类型检查**

运行：

```bash
pnpm schema:generate
pnpm --filter @yujian/schema test
pnpm --filter @yujian/schema typecheck
git diff --exit-code packages/schema/src/generated.ts
```

预期：全部通过，生成文件无漂移。

- [ ] **步骤 6：提交**

```bash
git add packages/schema content/fixtures package.json pnpm-lock.yaml
git commit -m "feat(内容契约): 添加首页静态快照模型"
```

## 任务 3：实现内容加载与多语言回退

**文件：**

- 创建：`apps/web/utils/localized.ts`
- 创建：`apps/web/composables/useContentSnapshot.ts`
- 创建：`apps/web/composables/useLocale.ts`
- 创建：`apps/web/plugins/i18n.server.ts`
- 创建：`apps/web/plugins/i18n.client.ts`
- 创建：`apps/web/locales/zh-CN.ts`
- 创建：`apps/web/locales/en.ts`
- 测试：`apps/web/test/unit/localized.test.ts`
- 测试：`apps/web/test/unit/locale.test.ts`

- [ ] **步骤 1：编写多语言回退失败测试**

```ts
// apps/web/test/unit/localized.test.ts
import { resolveLocalized } from '../../utils/localized'

describe('resolveLocalized', () => {
  it('返回请求语言', () => {
    expect(resolveLocalized({ 'zh-CN': '作品', en: 'Release' }, 'en')).toBe('Release')
  })

  it('英文缺失时回退简体中文', () => {
    expect(resolveLocalized({ 'zh-CN': '作品' }, 'en')).toBe('作品')
  })

  it('所有文本为空时返回空字符串', () => {
    expect(resolveLocalized({}, 'en')).toBe('')
  })
})
```

- [ ] **步骤 2：运行测试并确认 `resolveLocalized` 缺失**

运行：

```bash
pnpm --filter @yujian/web test -- localized.test.ts
```

预期：FAIL，提示无法解析导出。

- [ ] **步骤 3：实现最小回退函数**

```ts
// apps/web/utils/localized.ts
import type { LocalizedText } from '@yujian/schema'

export type SupportedLocale = 'zh-CN' | 'en'

export function resolveLocalized(value: LocalizedText, locale: SupportedLocale): string {
  return value[locale]?.trim() || value['zh-CN']?.trim() || ''
}
```

- [ ] **步骤 4：编写语言选择失败测试**

`locale.test.ts` 覆盖以下独立行为：

```ts
expect(detectLocale({ stored: 'en', browser: ['zh-CN'] })).toEqual({ locale: 'en', notify: false })
expect(detectLocale({ stored: null, browser: ['en-US'] })).toEqual({ locale: 'en', notify: true })
expect(detectLocale({ stored: null, browser: [] })).toEqual({ locale: 'zh-CN', notify: false })
```

- [ ] **步骤 5：实现 `detectLocale`、服务端默认实例和客户端持久化**

`detectLocale` 只接受 `zh-CN` 与 `en`，优先 `localStorage`，再匹配 `navigator.languages`，最后回退 `zh-CN`。客户端插件只在挂载后切换语言，并通过 `useState('locale-notice')` 暴露一次性非阻断提示。

- [ ] **步骤 6：验证测试和类型**

运行：

```bash
pnpm --filter @yujian/web test -- localized.test.ts locale.test.ts
pnpm --filter @yujian/web typecheck
```

预期：全部通过。

- [ ] **步骤 7：提交**

```bash
git add apps/web packages/schema
git commit -m "feat(国际化): 添加双语检测与内容回退"
```

## 任务 4：生成非人物视觉资源与试听样本

**文件：**

- 创建：`apps/web/public/media/hero-studio.webp`
- 创建：`apps/web/public/media/hero-stage.webp`
- 创建：`apps/web/public/media/cover-01.webp` 至 `cover-05.webp`
- 创建：`apps/web/public/media/video-01.webp` 至 `video-03.webp`
- 创建：`apps/web/public/media/moment-01.webp` 至 `moment-03.webp`
- 创建：`apps/web/public/media/preview-sample.wav`
- 修改：`content/fixtures/homepage.json`

- [ ] **步骤 1：使用 imagegen 技能生成低饱和占位视觉**

要求：不生成王子健或任何可识别人物；图像为原创的抽象录音棚、舞台光影、纸张与唱片纹理；使用近黑、雾灰、灰蓝和少量暖铜；不包含文字、Logo、渐变光球或高饱和紫色。

- [ ] **步骤 2：为首页用途裁切和压缩资源**

英雄图保持至少 `1600 × 1000`，封面为正方形，影像海报为 `16:9`，片段使用 `4:5`、`1:1` 和 `3:4`。所有资源转换为 WebP，单张文件大小和像素尺寸写入资源清单。

- [ ] **步骤 3：生成 3 秒无版权提示音样本**

使用本地脚本生成简单的低音量正弦波 WAV，只用于验证播放器，文件名和资源元数据明确标记为 fixture。

- [ ] **步骤 4：运行契约测试验证资源引用**

运行：

```bash
pnpm --filter @yujian/schema test
```

预期：fixture 中的所有本地资源路径均存在，尺寸和媒体类型匹配。

- [ ] **步骤 5：提交**

```bash
git add apps/web/public/media content/fixtures packages/schema/test
git commit -m "feat(视觉资源): 添加首页占位媒体"
```

## 任务 5：实现图标导航与大封面

**文件：**

- 创建：`apps/web/assets/css/main.css`
- 创建：`apps/web/components/ui/IconButton.vue`
- 创建：`apps/web/components/site/SiteHeader.vue`
- 创建：`apps/web/components/home/HeroShowcase.vue`
- 创建：`apps/web/components/site/LocaleNotice.vue`
- 创建：`apps/web/pages/index.vue`
- 测试：`apps/web/test/unit/hero-showcase.test.ts`
- 测试：`apps/web/test/unit/icon-button.test.ts`

- [ ] **步骤 1：编写图标按钮失败测试**

```ts
it('提供可访问名称和悬浮说明', () => {
  const wrapper = mount(IconButton, { props: { label: '播放' }, slots: { default: '▶' } })
  expect(wrapper.get('button').attributes('aria-label')).toBe('播放')
  expect(wrapper.get('[role="tooltip"]').text()).toBe('播放')
})
```

- [ ] **步骤 2：实现最小 `IconButton` 并验证测试通过**

组件使用原生 `button`，稳定尺寸为 `44 × 44 CSS px`，Tooltip 通过 `aria-describedby` 关联；禁用状态不能触发 `click`。

- [ ] **步骤 3：编写大封面失败测试**

测试必须覆盖：图片轮播项、视频海报、静音/暂停按钮、当前页进度、`prefers-reduced-motion` 下不加载自动视频、移动端焦点位置 CSS 变量。

- [ ] **步骤 4：实现 `SiteHeader` 和 `HeroShowcase`**

首屏只显示品牌、音乐人名、必要的作品名和图标操作。媒体使用 `<picture>` 或 `<video poster>`；视频 `muted`、`playsinline`，不自动请求音频。轮播切换不修改容器高度。

- [ ] **步骤 5：实现低饱和全局设计 Token**

```css
:root {
  color-scheme: dark;
  --color-bg: #0d0f11;
  --color-surface: #15191b;
  --color-text: #e7e9e5;
  --color-muted: #8c9596;
  --color-accent: #9aadaf;
  --color-warm: #a48a79;
  --color-border: #303638;
  --radius-tool: 4px;
  --content-max: 72.5rem;
}
```

- [ ] **步骤 6：验证组件、类型和页面静态渲染**

运行：

```bash
pnpm --filter @yujian/web test -- icon-button.test.ts hero-showcase.test.ts
pnpm --filter @yujian/web typecheck
pnpm --filter @yujian/web generate
```

预期：测试通过，`.output/public/index.html` 包含「遇健我」和「王子健」。

- [ ] **步骤 7：提交**

```bash
git add apps/web
git commit -m "feat(首页): 实现品牌导航与媒体首屏"
```

## 任务 6：实现音乐板块与单实例播放器

**文件：**

- 创建：`apps/web/composables/useAudioPlayer.ts`
- 创建：`apps/web/components/ui/PlatformLinks.vue`
- 创建：`apps/web/components/home/MusicSection.vue`
- 创建：`apps/web/components/player/AudioDock.vue`
- 修改：`apps/web/app.vue`
- 测试：`apps/web/test/unit/audio-player.test.ts`
- 测试：`apps/web/test/unit/music-section.test.ts`

- [ ] **步骤 1：编写播放器状态失败测试**

```ts
it('同一时间只保留一个当前曲目', () => {
  const player = createAudioPlayerState()
  player.play(trackA)
  player.play(trackB)
  expect(player.current.value?.id).toBe(trackB.id)
  expect(player.status.value).toBe('playing')
})

it('失败后保留平台链接并清除播放状态', () => {
  const player = createAudioPlayerState()
  player.play(trackA)
  player.fail('media-error')
  expect(player.status.value).toBe('error')
  expect(player.current.value?.platformLinks).toEqual(trackA.platformLinks)
})
```

- [ ] **步骤 2：运行测试并确认状态工厂缺失**

运行：

```bash
pnpm --filter @yujian/web test -- audio-player.test.ts
```

预期：FAIL，提示 `createAudioPlayerState` 未定义。

- [ ] **步骤 3：实现最小播放器状态和 `AudioDock`**

状态包含 `idle`、`loading`、`playing`、`paused` 和 `error`。真实 `HTMLAudioElement` 只在首次播放后创建；切换曲目先暂停旧实例。Dock 仅在 `current` 存在时渲染，关闭后释放资源。

- [ ] **步骤 4：编写音乐板块失败测试**

覆盖：有试听时封面显示播放；无试听时没有播放按钮；平台按钮在右侧；窄屏只显示主平台和更多菜单；封面始终保留稳定正方形比例。

- [ ] **步骤 5：实现 `MusicSection` 和 `PlatformLinks`**

平台图标来自固定平台注册表，不接受任意 HTML。外部链接只允许 HTTPS，使用 `target="_blank" rel="noopener noreferrer"`。

- [ ] **步骤 6：验证测试和全部回归**

运行：

```bash
pnpm --filter @yujian/web test -- audio-player.test.ts music-section.test.ts
pnpm --filter @yujian/web test
pnpm --filter @yujian/web typecheck
```

预期：全部通过且输出无警告。

- [ ] **步骤 7：提交**

```bash
git add apps/web
git commit -m "feat(音乐): 添加作品入口与试听播放器"
```

## 任务 7：实现影像、现场、片段与音乐人板块

**文件：**

- 创建：`apps/web/utils/sections.ts`
- 创建：`apps/web/components/home/VideoSection.vue`
- 创建：`apps/web/components/home/EventSection.vue`
- 创建：`apps/web/components/home/MomentSection.vue`
- 创建：`apps/web/components/home/ArtistSection.vue`
- 修改：`apps/web/pages/index.vue`
- 测试：`apps/web/test/unit/sections.test.ts`

- [ ] **步骤 1：编写板块显隐失败测试**

```ts
it('隐藏未启用和没有可显示内容的板块', () => {
  const visible = resolveHomepageSections(snapshot, 'zh-CN')
  expect(visible.map(section => section.type)).toEqual([
    'hero', 'music', 'video', 'event', 'moment', 'artist',
  ])

  const withoutEvents = structuredClone(snapshot)
  withoutEvents.events = []
  expect(resolveHomepageSections(withoutEvents, 'zh-CN').some(s => s.type === 'event')).toBe(false)
})
```

- [ ] **步骤 2：运行测试并确认解析器缺失**

运行：

```bash
pnpm --filter @yujian/web test -- sections.test.ts
```

预期：FAIL，提示 `resolveHomepageSections` 未定义。

- [ ] **步骤 3：实现板块解析器**

解析器保持后台顺序，过滤 `enabled: false`，解析稳定内容 ID，并在引用缺失时跳过该条内容。`event` 只包含当前日期之后且状态为 `scheduled` 的活动；集合为空时移除板块。

- [ ] **步骤 4：实现媒体优先的 4 个板块**

`VideoSection` 使用 1 大 2 小不对称布局；`EventSection` 使用日期线；`MomentSection` 使用无文字图片拼贴；`ArtistSection` 只保留姓名、短简介和官方图标。所有媒体保持稳定宽高比和懒加载。

- [ ] **步骤 5：验证单元测试、静态生成和无运行时内容请求**

运行：

```bash
pnpm --filter @yujian/web test -- sections.test.ts
pnpm --filter @yujian/web generate
rg "api/|fetch\(" apps/web/.output/public/_nuxt
```

预期：测试与生成通过；搜索不出现运行时内容 API 调用。

- [ ] **步骤 6：提交**

```bash
git add apps/web
git commit -m "feat(首页): 补全影像现场与人物板块"
```

## 任务 8：完成 SEO、响应式和端到端验证

**文件：**

- 修改：`apps/web/nuxt.config.ts`
- 修改：`apps/web/assets/css/main.css`
- 创建：`apps/web/public/robots.txt`
- 创建：`apps/web/server/routes/sitemap.xml.ts`
- 创建：`apps/web/utils/structured-data.ts`
- 创建：`apps/web/playwright.config.ts`
- 创建：`apps/web/test/e2e/home.spec.ts`
- 创建：`apps/web/test/e2e/accessibility.spec.ts`

- [ ] **步骤 1：编写桌面与移动端失败测试**

`home.spec.ts` 使用 `1440 × 1000` 和 `390 × 844` 两个视口，验证：品牌和人物首屏可见、每个启用板块出现、语言不改 URL、偏好写入 `localStorage`、有试听曲目显示 Dock、无试听曲目只显示平台入口、移动端没有依赖悬浮的操作。

- [ ] **步骤 2：编写可访问性失败测试**

`accessibility.spec.ts` 使用 axe-core 扫描首页，键盘依次访问导航、播放、平台和语言按钮；断言视频存在暂停控制，所有图标按钮具有可访问名称。

- [ ] **步骤 3：运行端到端测试并记录失败**

运行：

```bash
pnpm --filter @yujian/web exec playwright install chromium
pnpm --filter @yujian/web test:e2e
```

预期：首次运行因响应式、结构化数据或可访问细节缺失而失败，失败信息与测试意图一致。

- [ ] **步骤 4：实现 SEO 和响应式细节**

添加规范 URL、中文 Open Graph、`robots.txt`、站点地图和 `MusicGroup`、`MusicRecording`、`VideoObject`、`MusicEvent` JSON-LD。CSS 确保触控目标至少 `44 × 44 CSS px`，没有水平滚动，所有固定格式区域有 `aspect-ratio`，`prefers-reduced-motion` 禁用自动切换和非必要过渡。

- [ ] **步骤 5：执行截图和画布像素检查**

端到端测试保存桌面和移动端全页截图，并检查英雄媒体区域的像素方差不为零，证明视觉资源成功渲染。人工检查文字不重叠、下一板块在首屏底部可见、Dock 不遮挡页脚操作。

- [ ] **步骤 6：运行完整前端验证**

运行：

```bash
pnpm lint
pnpm typecheck
pnpm test
pnpm generate
pnpm --filter @yujian/web test:e2e
```

预期：全部通过且无控制台错误。

- [ ] **步骤 7：提交**

```bash
git add apps/web
git commit -m "test(首页): 完成响应式与可访问性验收"
```

## 任务 9：补充 EdgeOne Pages 构建说明和产物校验

**文件：**

- 创建：`apps/web/EDGEONE.md`
- 创建：`scripts/verify-static-output.mjs`
- 修改：`package.json`
- 修改：`README.md`
- 测试：`scripts/verify-static-output.test.mjs`

- [ ] **步骤 1：编写静态产物校验失败测试**

测试创建最小临时产物，断言校验器拒绝缺少 `index.html`、缺少静态资源、HTML 中存在运行时内容 API 标记和超过预算的初始脚本。

- [ ] **步骤 2：实现产物校验器**

校验器接收输出目录，检查 `index.html`、`robots.txt`、站点地图、资源引用、内容 API 标记和初始 JavaScript 预算。失败时输出具体文件和阈值。

- [ ] **步骤 3：记录 EdgeOne Pages 参数**

`EDGEONE.md` 固定以下控制台配置：

```text
Root directory: /
Install command: pnpm install --frozen-lockfile
Build command: pnpm --filter @yujian/web generate
Output directory: apps/web/.output/public
Node.js: 22
```

同时说明 `CONTENT_SNAPSHOT_PATH`、缓存头、SPA 回退必须关闭、部署后 `/`、`/robots.txt` 和 `/sitemap.xml` 的冒烟检查。

- [ ] **步骤 4：运行发布前验证**

运行：

```bash
pnpm generate
pnpm verify:static
```

预期：静态产物校验通过。

- [ ] **步骤 5：提交**

```bash
git add apps/web/EDGEONE.md scripts package.json README.md
git commit -m "docs(部署): 添加 EdgeOne Pages 构建基线"
```

## 任务 10：公开首页完成审计

**文件：**

- 修改：`docs/superpowers/plans/2026-08-29-public-homepage.md`

- [ ] **步骤 1：逐项核对设计规格第 4 至 7、12 至 15、17.2 和 18 节**

为每条要求记录对应代码、测试或构建产物。缺少直接证据的项目不得标记完成。

- [ ] **步骤 2：从干净依赖状态重新验证**

运行：

```bash
pnpm install --frozen-lockfile
pnpm verify
git status --short
```

预期：所有检查通过；工作树只包含计划复选框更新或保持干净。

- [ ] **步骤 3：提交计划状态**

```bash
git add docs/superpowers/plans/2026-08-29-public-homepage.md
git commit -m "docs(计划): 完成公开首页实现审计"
```

## 后续独立计划

公开首页完成后，按同一设计规格继续编写并执行以下计划：

1. `2026-08-29-go-content-service.md`：PostgreSQL、OIDC/RBAC、内容版本、资源适配和发布编排。
2. `2026-08-29-admin-console.md`：Vue 管理端、草稿、预览、审核、发布和回滚。
3. `2026-08-29-edgeone-publishing.md`：不可变快照、构建触发器、EdgeOne 状态回传与端到端发布测试。
