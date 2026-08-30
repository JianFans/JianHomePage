# EdgeOne Pages 部署基线

公开首页使用 Nuxt 静态生成，不依赖运行时内容 API。EdgeOne Pages 只负责构建和分发版本化产物；后续 Go 服务通过独立源站接入 EdgeOne，不与首页静态托管耦合。

## 控制台配置

| 配置项 | 值 |
| --- | --- |
| Root directory | `/` |
| Install command | `pnpm install --frozen-lockfile` |
| Build command | `pnpm verify:edgeone` |
| Output directory | `apps/web/.output/public` |
| Node.js | `22` |

关闭 SPA fallback。首页、`robots.txt` 和 `sitemap.xml` 都是构建时生成的真实文件，不应回退到同一个 HTML。

## 内容快照

本地执行 `pnpm verify` 时默认读取 `content/fixtures/homepage.json`，便于开发和测试。EdgeOne 执行 `pnpm verify:edgeone` 时必须通过 `CONTENT_SNAPSHOT_PATH` 注入经过审核的不可变快照；变量值支持仓库根目录下的相对路径或构建环境中的绝对路径。变量缺失、文件不存在或路径指向 `content/fixtures` 时，构建直接失败。

正式发布链路需满足以下约束：

- 快照包含版本号、发布批次和资源清单。
- 构建开始后不再修改同一快照。
- 回滚时重新构建或恢复指定快照，不读取数据库当前状态。
- 首页浏览器端不得根据 `CONTENT_SNAPSHOT_PATH` 或服务商 API 动态取数。
- 素材 `src` 使用内容服务返回的稳定 HTTPS 地址，建议统一位于 EdgeOne 加速的 `https://media.yujian.me`；不得把 COS 内部端点或短期签名 URL 写入快照。

构建启动时会读取并解析选定快照；文件不存在、JSON 无效、不符合 canonical JSON Schema、包含非法 URL 或存在悬空内容引用时直接失败，避免发布错误快照。

## 缓存建议

| 路径 | 建议 Cache-Control |
| --- | --- |
| `/_nuxt/*` | `public, max-age=31536000, immutable` |
| `/media/*` | `public, max-age=3600, stale-while-revalidate=86400` |
| `/`, `/index.html`, `/_payload.json` | `public, max-age=0, must-revalidate` |
| `/robots.txt`, `/sitemap.xml` | `public, max-age=300` |

`/_nuxt/*` 文件名包含内容哈希，可以长期缓存。仓库内 `/media/*` fixture 文件名稳定，不能使用 `immutable`。内容服务上传的对象键包含素材 ID，替换素材时应创建新素材而不是覆盖旧对象；EdgeOne 媒体域名可据此设置较长缓存，并保留主动刷新能力。

## 发布前检查

在仓库根目录运行：

```bash
pnpm verify
```

EdgeOne 正式项目使用 `pnpm verify:edgeone`，并由发布工作流在构建前提供 `CONTENT_SNAPSHOT_PATH`。

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

后续 Go 服务建议使用独立源站域名，并在 EdgeOne 中配置 `/api/*` 或专用子域名的回源规则。`MEDIA_PUBLIC_BASE_URL` 配置为 EdgeOne 媒体域名，源站可使用腾讯云 COS；域名、对象存储和构建触发器都通过适配器配置，不把腾讯云 SDK 类型写入内容领域模型，以便迁移服务商。

发布服务会在后台自动对账 EdgeOne 构建任务。即使管理端关闭，`pending` 任务仍会按原幂等键恢复，`building` 任务也会继续查询并完成生产指针切换；管理端刷新操作只用于缩短状态展示延迟。
