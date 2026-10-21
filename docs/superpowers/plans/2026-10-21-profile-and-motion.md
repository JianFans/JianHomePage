# 用户资料与全站动效实现计划

> **面向 AI 代理的工作者：** 使用 executing-plans 逐任务实现，持续执行，不在任务之间请求确认。

**目标：** 补齐 OIDC 用户资料获取与前后台动效。

**架构：** 资料只在 Go 登录回调中从可信 UserInfo 补齐，权限继续来自已验证 ID token。前端使用现有 CSS 与 Vue Transition，滚动动效渐进增强。

**技术栈：** Go net/http、Vue、CSS、Vitest、Playwright。

**规格：** [设计说明](../specs/2026-10-21-profile-and-motion-design.md)。

## 全局约束

- 不新增运行时依赖、内容 API 或数据库迁移。
- 凭据不得进入日志或浏览器；UserInfo 不授予权限。
- 无动画、无 JavaScript 和减少动态效果时内容仍可用。
- 每个职责验证后单独提交，不推送、不部署。

## 审查重点

1. UserInfo 的身份不匹配或重定向不得泄露 token 或改变角色。
2. 资料请求失败不能使原本有效的登录失败或清空签名资料。
3. 轮播离场节点不能延续旧视频播放或重复媒体请求。
4. 动画不得藏住静态页面、遮挡焦点或制造横向溢出。
5. 减少动态效果覆盖新动画，并能在页面打开后切换。

## 任务 1：用户资料

- [x] 在 `apps/server/internal/auth/session_profile_test.go` 添加签名登录回归：UserInfo 返回同 sub 的显示名称和头像；不匹配、失败、重定向保留签名资料，角色不变。
- [x] 运行 `go test ./internal/auth -run UserInfo -count=1`，先确认资料断言失败。
- [x] 在 `session.go` discovery 中返回 UserInfo 地址，在 `session_profile.go` 完成受限查询与资料合并；复用 `oidc.go` 的字段解析和 URL 安全规则。
- [x] 调整 `studio.css` 收起侧栏展示头像，补充 `studio-profile.spec.ts` 中间屏幕和手动收起覆盖。
- [x] 跑 auth 包、管理端单元测试、类型检查、profile 浏览器测试；同步服务端 README 中资料来源与重新登录说明。
- [x] 提交 `fix(身份): 补齐登录用户显示资料`（`455ecaf`）。

## 任务 2：前后台动效

- [x] 在公开站与管理端 E2E 添加正常与减少动态效果的行为测试；轮播切换产生动画且仍可立即操作，减少动态效果时不产生新动画。
- [x] 运行对应 E2E，确认缺失动效导致断言失败。
- [x] 新建两端 `motion.css`，按规格统一节奏、scroll timeline、控件反馈；将 Hero 媒体和标题按幻灯片设置 key，直接替换媒体，避免保留离场视频。
- [x] 将管理端已有 Transition、Toast 和弹窗迁入统一时长和缓动；保留可访问性与状态逻辑。
- [x] 跑 `pnpm test`、`pnpm lint`、`pnpm typecheck`、两端 E2E、`pnpm build:server:assets`、`pnpm verify:go`、`pnpm test:embedded`。
- [x] 同步前端 README／动效维护说明，提交 `feat(界面): 完善前后台动效与交互反馈`。

## 最终检查

- [x] 独立整分支审查并修复有效发现。
- [x] `git diff --check`、签名、工作区状态和职责边界检查。
- [x] 交付说明包含测试结果和真实 OIDC／云环境未验证项。

## 验证记录

- 资料和动效测试均先复现失败，再验证修复。
- 滚动入场初版降低文字透明度，使屏幕外内容未满足对比度要求；改为仅位移，保留完整文字对比度，四组中英文／桌面手机 axe 扫描通过。
- `pnpm test`：Schema 42、公开站 113、管理端 436、脚本 52 项通过。
- `pnpm lint`、`pnpm typecheck` 通过。
- `go test ./internal/auth -cover -count=1` 通过，语句覆盖率 87.4%。独立审查另行复跑 auth 包、HeroShowcase 与 HomepageView 共 32 项单元测试，未发现需修复的新增问题。
- 公开站 E2E：17 项通过；Go 导出页面专用的 1 项在开发站运行中按配置跳过，另由 `pnpm test:embedded` 验证。
- 管理端全量 E2E：桌面和手机共 158 项通过，包括资料显示、动效、媒体释放、键盘焦点、Toast 计时与窄屏滚动。
- 公开站静态生成与 `pnpm verify:static` 通过，首屏 JavaScript 292 KiB。
- `pnpm build:server:assets` 通过；`pnpm test:embedded` 使用实际 Go 程序导出页面，18 项浏览器测试全部通过。
- `pnpm verify:go` 通过：151 个 Go 文件格式检查、代码生成、全包测试与 vet。
- 真实 Casdoor、生产数据库及云端发布未执行；资料需升级后退出重登确认，公开站新动效需重新发布静态页面后生效。
