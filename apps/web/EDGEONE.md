# EdgeOne 域名、缓存和网页检查

首次部署按 [部署操作指南](../../docs/DEPLOYMENT.md) 操作。公开首页由 Go 程序内置的 Vue SSR 提前生成，再由同一程序上传 EdgeOne Pages；访客打开首页时不用请求 Go 后端获取内容。本文用于查 Pages 设置、缓存和上线后的检查方法。

## Git 构建基线与 Direct Upload

本项目的后台自动发布方案使用 Direct Upload（直接上传）项目：Go worker 生成 `index.html`、`robots.txt`、`sitemap.xml`，组合内嵌的 `/site/assets/*` 浏览器文件，再上传 Pages。按这套方案部署，不需要在 Pages 填安装命令或构建命令，也不要同时启用 Git 自动部署。配置为 `PAGES_API_TOKEN`、`PAGES_PROJECT_ID`、`PAGES_REGION` 和 `PAGES_PUBLIC_URL`，详见 [服务端参数](../server/README.md#环境变量)。

下面仅供单独研究 Git 集成构建时参考，与根 `edgeone.json` 对应，不是首次部署的额外步骤：

| 配置项 | 值 |
| --- | --- |
| Root directory | `/` |
| Install command | `pnpm install --frozen-lockfile` |
| Build command | `pnpm verify:edgeone` |
| Output directory | `apps/web/.output/public` |
| Node.js | 满足根 `package.json` 的 `engines.node` 要求 |

关闭 SPA fallback。首页、`robots.txt` 和 `sitemap.xml` 都是构建时生成的真实文件，不应回退到同一个 HTML。

## 内容快照

正式后台发布读取 COS 中已经审核的不可变快照，校验原始 checksum、release ID、canonical 契约、冻结目标和稳定 HTTPS 素材，再使用内嵌渲染器生成页面。生产 worker 拒绝相对素材地址，不使用开发 fixture，也无需 `CONTENT_SNAPSHOT_PATH`。

本地执行 `pnpm verify` 时默认读取 `content/fixtures/homepage.json`。独立 Nuxt／可选 Git 构建执行 `pnpm verify:edgeone` 时，仍必须通过 `CONTENT_SNAPSHOT_PATH` 注入经过审核的非 fixture 快照；变量值支持仓库根目录下的相对路径或构建环境中的绝对路径。变量缺失、文件不存在或指向 `content/fixtures` 时，该门禁直接失败。

本地预览可在 `pnpm build:server` 后运行 `dist/yujian-server --export-site snapshot.json --output empty-directory`，Windows 使用 `.exe`。输出目录必须为空；该模式允许 fixture 和开发相对素材，不连接数据库或 Pages，不表示已完成正式发布。

正式发布链路需满足以下约束：

- 快照包含版本号、发布批次和资源清单。
- 构建开始后不再修改同一快照。
- 回滚时重新构建或恢复指定快照，不读取数据库当前状态。
- 首页浏览器端不得根据 `CONTENT_SNAPSHOT_PATH` 或服务商 API 动态取数。
- 素材 `src` 使用内容服务返回并持久化的稳定 HTTPS 地址，建议统一位于 EdgeOne 加速的 `https://media.yujian.me`；不得把 COS 内部端点或短期签名 URL 写入快照，发布时也不得按当前源站配置重算历史地址。

构建启动时会读取并解析选定快照；文件不存在、JSON 无效、不符合 canonical JSON Schema、包含非法 URL 或存在悬空内容引用时直接失败，避免发布错误快照。

## 界面动效

`assets/css/motion-tokens.css` 是前后台共用的速度与缓动参数；`assets/css/motion.css` 提供首屏、轮播、卡片和控件反馈。滚动入场通过 CSS view timeline 渐进增强，不依赖 JavaScript 隐藏或显示内容；不支持时直接展示原布局，访客的减少动态效果偏好会实时生效。轮播直接替换媒体，不保留离场视频；试听继续使用单实例播放器。发布新动画需要重新生成并上传静态站，更新管理程序不会改变已经部署的首页资源。

## 缓存建议

| 路径 | 建议 Cache-Control |
| --- | --- |
| `/site/assets/*` | `public, max-age=31536000, immutable` |
| `/`, `/index.html` | `public, max-age=0, must-revalidate` |
| `/robots.txt`, `/sitemap.xml` | `public, max-age=300` |

`/site/assets/*` 文件名包含内容哈希，可以长期缓存。独立 Nuxt 构建的 `/_nuxt/*` 可采用同样策略，`/_payload.json` 应重新验证；Go 输出不依赖 Nuxt payload。管理端 HTML 由 Go 返回 `no-store`，管理端带哈希的 Nuxt 资源长期缓存。

生产媒体从稳定 HTTPS 媒体域名读取。内容服务上传的对象键包含素材 ID，替换素材时应创建新素材而不是覆盖旧对象；EdgeOne 媒体域名可据此设置较长缓存，并保留主动刷新能力。开发 `/media/*` fixture 文件名稳定，不能使用 `immutable`。

## 发布前检查

在仓库根目录运行：

```bash
pnpm verify
pnpm build:server:assets
pnpm verify:go
pnpm test:embedded
```

`pnpm test:embedded` 检查 Go 导出产物的浏览器渲染与交互。独立 Nuxt／Git 构建另使用 `pnpm verify:edgeone`，并在构建前提供非 fixture 的 `CONTENT_SNAPSHOT_PATH`。以上本地门禁不能替代真实云环境验证。

完整门禁会执行 ESLint、TypeScript、单元测试、静态生成与产物校验，并检查：

- `index.html`、`robots.txt` 和 `sitemap.xml` 是否存在。
- 首页引用的本地资源是否完整。
- 产物是否包含运行时内容 API 标记。
- 首屏 JavaScript 未压缩体积是否超过 320 KiB。

## 部署后冒烟检查

```text
GET https://yujian.me/             -> 200, text/html
GET https://yujian.me/robots.txt   -> 200, text/plain
GET https://yujian.me/sitemap.xml  -> 200, application/xml
```

同时检查：首页 canonical 为 `https://yujian.me`，静态资源返回 200，页面没有横向滚动，试听资源可访问，SPA fallback 保持关闭。

## 动态服务接入

公开域名由 Pages 分发，管理域名使用独立 HTTPS 源站或反向代理，将 `/`、`/preview`、`/_nuxt/*` 与 `/api/*` 一并转发至 Go 程序，保持管理端与 API 同源。`MEDIA_PUBLIC_BASE_URL` 配置为长期稳定的 EdgeOne 媒体域名，源站可使用腾讯云 COS；域名、对象存储和 Pages 通过适配器配置，不把服务商 SDK 类型写入内容领域模型。迁移服务商时切换回源并保持公开媒体域名不变，历史快照无需改写。COS 直传 CORS、TLS 和 OIDC 回调仍需在真实账号中配置验证。

Go worker 会在后台自动对账发布任务。即使管理端关闭，任务也会继续处理并完成生产指针切换；管理端刷新操作只用于缩短状态展示延迟。任务冻结完整 Go 二进制 SHA-256 和 Pages 目标，升级前先结束旧活动任务。

云端创建结果不明时，任务保持 `submitting`，不会自动重试创建。管理员核实云端后，使用受认证和 RBAC 保护的 API 绑定部署 ID，或确认未创建部署并结束任务；详见 [内置 Pages 发布与恢复](../server/README.md#内置-pages-发布与恢复)。历史任务须用匹配冻结身份的旧程序恢复，不直接修改数据库或重复上传。
