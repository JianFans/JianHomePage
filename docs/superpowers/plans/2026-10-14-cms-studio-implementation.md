# CMS 版本工作区实现计划

> **面向 AI 代理的工作者：** 使用 `subagent-driven-development` 执行独立任务，实施后分别检查规格与代码质量。认证、内容版本、客户端和公开站按职责验证；不发布到远端或真实云环境。

**目标：** 将已认可的 CMS 设计迁入 Nuxt，提供真实登录会话、独立模块、版本浏览与复制、统一保存、Toast 与克制动效，并完成公开页操作区优化。

**架构：** Go 提供标准 OIDC 授权码与 PKCE 登录、HttpOnly 不透明会话、CSRF 保护和 PostgreSQL 会话仓储。版本分页与来源是服务端元数据，复制保存源版本快照而非客户端猜测的内容。Vue 复用已有 canonical 校验和表单事务，由统一保存入口应用当前表单并执行乐观更新；公开站继续只渲染构建时快照。

**技术栈：** Nuxt、Vue、Go、PostgreSQL、OIDC、Vitest、Playwright、Lucide。

## 执行状态

- [x] 用户认可布局、提示、强调色与在开发阶段完善动效。
- [x] 建立隔离工作区及 `feat/cms-studio` 分支。
- [x] 基线依赖与测试验证。
- [x] 任务 1：认证与版本服务契约（规格与质量审查通过）。
- [x] 任务 2：管理端会话、API 与版本状态（单元、类型检查与浏览器验证通过）。
- [x] 任务 3：布局、模块与统一保存（桌面与手机浏览器验证通过）。
- [x] 任务 4：Toast 与动效（独立计时、队列、暂停、键盘与减少动态效果验证通过）。
- [x] 任务 5：公开站交互优化（单元、静态构建与浏览器验证通过）。
- [x] 任务 6：文档、浏览器验证与独立审查。

## 文件职责

| 文件或目录 | 职责 |
| --- | --- |
| `apps/server/internal/auth/session.go` 与测试 | 授权状态、PKCE、nonce、会话与 CSRF；服务商通过接口替换 |
| `apps/server/internal/store/postgres`、`memory` | 会话持久化与稳定分页；迁移保持事务与校验和 |
| `apps/server/internal/content`、`domain` | 版本摘要、名称、来源关系与原子复制 |
| `apps/server/internal/httpapi`、`config`、`cmd/api` | HTTP 契约、启动校验、依赖装配与 CORS |
| `apps/admin/utils/admin-api.ts` | 会话凭据、CSRF、版本分页与复制客户端 |
| `apps/admin/composables/useAdminSession.ts` | 内存中的身份、CSRF、恢复与退出状态 |
| `apps/admin/composables/useAdminWorkspace.ts` | 版本切换、保存与并发响应保护 |
| `apps/admin/pages/index.vue`、`components/Studio*.vue` | 布局、版本操作、模块导航与高级快照工具 |
| `apps/admin/components/ToastViewport.vue`、`composables/useToasts.ts` | 独立消息生命周期与动效 |
| `apps/admin/assets/css/studio.css` | 设计色、尺寸、组件层次与减少动态效果 |
| `apps/web/components/home/HeroShowcase.vue` | 跳转按钮与固定翻页区域 |
| `apps/web/components/home/HomepageView.vue`、`site` | 平台去重与按滚动显示回顶 |

## 任务 1：认证与版本服务契约

先在 Go 编写失败测试，覆盖匿名、真实签名、回调状态失配、nonce、PKCE、过期、退出和 CSRF 拒绝。复用现有 OIDC 的签名、issuer、audience 与角色验证。账号密码由配置的身份提供器登录页承接，不把密码提交给 Bearer 验证器，不添加固定账号、模拟登录或生产开发身份回退。

```text
GET  /api/v1/auth/session   -> { authenticated, loginAvailable, principal?, csrfToken?, expiresAt? }
GET  /api/v1/auth/login     -> 配置的 OIDC 授权入口
GET  /api/v1/auth/callback  -> 校验 state、nonce、授权码与 PKCE，创建会话后跳回固定管理端 URL
POST /api/v1/auth/logout    -> 验证 Origin 与 X-CSRF-Token，撤销会话并清理 Cookie
GET  /api/v1/versions?limit=20&cursor=... -> { items: 摘要[], nextCursor? }
POST /api/v1/versions/{versionId}/clone  -> { name }，返回完整独立草稿
```

```ts
interface VersionSummary {
  id: string
  name: string
  sourceVersionId?: string
  status: 'draft' | 'in_review' | 'publishing' | 'published' | 'archived'
  revision: number
  checksum: string
  createdAt: string
  updatedAt: string
  reviewApproved?: boolean
}
```

列表不发送快照；读取完整版本继续使用既有 GET 接口。新增可选名称与来源字段保持老版本可读，老版本名称回退 ID。分页按创建时间及 ID 倒序，游标严格校验，权限检查覆盖直接领域调用。复制在事务中读取源、复制 canonical 快照、创建新 ID、修订 1 与来源，记录审计；不改变线上指针，不复制审核批准状态。源不存在或无权限时不创建目标。

会话 Cookie 使用 HttpOnly、生产 Secure、SameSite=Lax，随机不透明值只在服务端以摘要索引。会话在数据库存储身份和有效期；退出使旧 Cookie 失效。跨 Origin 管理端使用明确允许列表与带凭据 CORS，不使用通配符。写请求验证 CSRF；已有非浏览器 Bearer 请求保持支持。会话、授权码、Token、密码和签名 URL 不进入日志。

新增 `ADMIN_OIDC_CLIENT_ID`、`ADMIN_OIDC_CLIENT_SECRET`（可选）、`ADMIN_OIDC_REDIRECT_URL`、`ADMIN_RETURN_URL`、`ADMIN_SESSION_KEY`，任何部分配置必须成组完整且生产只接受 HTTPS。没有配置时会话查询明确返回登录不可用，不能悄悄授予身份。状态 Cookie 使用短有效期与认证加密。

验证：先跑目标包失败用例，再实现并运行目标包、`pnpm verify:go`、`pnpm test:coverage:go`。同步 HTTP OpenAPI、迁移和服务端 README。

## 任务 2：会话、API 与版本状态

新增客户端失败测试，确认管理请求带 `credentials: 'include'`，写操作携带内存 CSRF，签名素材 PUT 不带 Cookie、Authorization 或 CSRF。会话恢复不从 localStorage 或 URL 读取凭据；只保存语言偏好。

```ts
interface AdminSession {
  authenticated: boolean
  loginAvailable: boolean
  principal?: { subject: string; name?: string; email?: string; roles: string[] }
  csrfToken?: string
  expiresAt?: string
}
```

会话恢复成功后读取版本摘要并选择最新草稿，若没有则选择最近版本。首次安装没有版本时允许从高级工具导入审核前快照，保存为第一份草稿；不在生产中自动载入 fixture。所有未授权响应撤下管理界面但保留当前会话的未保存编辑状态，重新登录前明确提示保存恢复边界。

版本列表与读取使用请求序号或 AbortController 保护迟到响应；重复载入不能覆盖更新目标。切换版本有继续编辑、放弃并切换、保存并切换三个选择。保存失败不能切换。复制仅调用服务端 clone，不信任客户端传入快照。历史及审核、发布版本在 UI 与服务端均只读；操作能力来自身份角色与状态。

验证：`pnpm --filter @yujian/admin test -- test/unit/admin-api.test.ts test/unit/admin-workspace.test.ts test/unit/admin-session.test.ts`。

## 任务 3：正式 CMS 布局与统一保存

先写页面测试：匿名只显示登录入口；恢复会话后导航可操作；同一时刻只有选中模块内容可见；选版本不发布；历史不能修改；失败保存保留输入。替换旧页面连接表单，使用独立侧栏、顶部版本栏、主内容区、手机抽屉和紧凑版本选择。

模块为概览、首页编排、音乐、影像、现场、片段、素材库和版本历史。版本切换不依赖手输 ID。编辑列表进入记录内容，音乐按基本信息、曲目与平台拆分；英文默认折叠，可访问名称完整。高级 JSON 编辑、导入导出、诊断与拒绝原因通过明确入口提供。

复用已有 `useMusicWorkspace`、`useHomepageWorkspace` 等事务。当前脏表单切换模块时先提供保存或放弃，避免多个互相过期的表单堆积。顶部保存先同步应用当前有效表单，再提交 Go；任何阶段失败保留表单。成功后重建干净表单基线。快照来源、完整校验与乐观 revision 不变。预览继续复用既有安全 iframe 协议，不含令牌。

更新原页面 E2E，使其通过会话与版本 API 拦截进入真实布局；保留表单单元用例，不为旧连接 UI 留兼容的隐藏页面。浏览器用例覆盖保存失败、未保存保护、历史复制、只读、语言和响应式。

验证：管理端单元测试、typecheck、E2E 与覆盖率。

## 任务 4：Toast 与动效

先写假计时测试：多条独立消息、最多 3 条可见、队列补位、手动关闭、悬停与聚焦暂停、页面隐藏暂停、卸载清理与文本安全。`useToasts` 只管理消息状态；Vue `TransitionGroup` 处理出入及补位，关闭时保留退出动画。

消息在右下角，普通 5 秒、错误 7 秒，关闭按钮至少 44 px。对话框提示保持可操作，重要错误提供必要字段上下文，持久状态不是 Toast。用鼠尾草绿、雾蓝、淡紫、琥珀和珊瑚红建立语义，正文与控件满足对比度。

切页与列表切换、抽屉、弹层、工具按钮、标签页和封面采用简短颜色、透明度或小位移动效。搜索输入不反复触发整页动画。所有动画遵守减少动态效果，避免布局跳动和不必要的 JS 动画依赖。

验证：Toast 单元、管理端 E2E 中真实 reducedMotion 模拟、手机可见区域与键盘关闭。

## 任务 5：公开站操作区优化

先添加失败测试：轮播跳转在翻页上方且等宽；无目标项翻页位置稳定；艺术家板块展示时页脚不重复平台；艺术家停用时页脚仍提供一次导航；回顶初始隐藏、480 px 后出现、返回后隐藏。

修改 HeroShowcase 控件分组，媒体按钮独立；回顶迁入固定页头末端并预留宽度，监听使用 passive 并卸载清理。跳转保持稳定锚点与安全外链，减少动态效果时即时滚动。使用现有六类板块过滤判断艺术家是否实际渲染，不删除 Schema 字段。

验证：公开站单元、typecheck、generate、E2E、静态产物门禁。

## 任务 6：验证、文档与收尾

- 运行 `pnpm lint`、`pnpm typecheck`、`pnpm test`、`pnpm test:coverage`、`pnpm verify:go`、`pnpm test:coverage:go`、`pnpm generate` 与 `pnpm verify:static`。
- 运行两个应用的 Playwright 测试；新增 Chromium 缓存缺失时使用已安装 Chrome 或项目明确安装命令，不修改锁文件版本。
- 对 Go 认证、版本元数据、保存保护分别进行规格审查，再进行质量审查，修复有效发现并重跑受影响验证。
- 在本地启动正式应用，以浏览器检查桌面与手机、Toast、导航、预览和公开站；真实身份提供器与云基础设施不可访问时明确记录验证边界。
- 同步根 README、管理端与服务端 README，提供准确的认证配置、迁移和开发方式，不写临时机器状态。
- 按认证、内容版本、管理端、公开站和文档职责原子提交；不推送、合并或发布。

## 最终验证记录

- ESLint、根类型检查、根测试、静态生成与静态产物检查通过。
- 覆盖率按模块串行验证：Schema 34 项、管理端 334 项、公开站 109 项通过；语句覆盖率分别为 90.52%、88.45% 与 89.50%。并行运行曾出现 Nuxt 初始化超时，串行复验通过，未调整超时或放宽断言。
- Go 格式、生成、全包测试与 vet 通过；全包语句覆盖率为 83.32%。
- 使用本机 Chrome 完成管理端 74 项和公开站 11 项 Playwright 测试；检查桌面、手机与 Toast 截图。CI 默认浏览器保持 Playwright Chromium。
- 独立审查后修复在途保存锁、历史只读记录切换、双语字段诊断与聚焦、跨分页草稿选择、弹窗焦点及会话过期保护；最终复审未发现新的有效缺陷。
- 管理端浏览器保存与会话测试使用拦截 API 验证契约，不替代真实身份提供器或数据库联调。本地缺少 PostgreSQL 测试连接且 Docker 不可用，真实数据库集成测试未执行；真实 OIDC、对象存储及 EdgeOne 集成尚未验证。
