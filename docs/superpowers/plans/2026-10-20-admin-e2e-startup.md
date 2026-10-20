# 管理端 E2E 启动超时修复

**已验证现象：** Release v0.0.8 的 `Run admin E2E` 在 `config.webServer` 等待 120 秒后失败，测试尚未开始。此前同代码的普通 CI 成功；本地原配置 130 项测试通过，因此未稳定复现远端底层阻塞。原配置将 Nuxt 构建与 preview 启动放在同一条受启动超时约束的命令中，并使用 `localhost`。

**修复范围：** 将构建移至 `test:e2e` 脚本，在 Playwright 启动前完成；构建失败直接终止并保留完整输出。Playwright 直接启动 `.output/server/index.mjs`，固定 Nitro 监听地址及探测地址为 `127.0.0.1:49318`，输出启动日志，保留原有 120 秒启动上限。Actions 管理端测试步骤另设 15 分钟总上限，包含构建及全部测试，防止构建永久挂起。不跳过测试、不放宽断言、不降级依赖。

- [x] 添加自动化契约测试，确认旧的组合命令及脚本不满足新契约。
- [x] 调整 `apps/admin/package.json` 和 `apps/admin/playwright.config.ts`，同步模块 README。
- [x] 运行修改后的全部桌面／手机浏览器测试及 Linux／Node 26 干净构建启动检查。
- [x] 验证自动化测试、ESLint、actionlint 和 diff。

**验证结果：** 修改后的桌面／手机浏览器测试 130 项通过（6.4 分钟），自动化测试 34 项通过；ESLint、actionlint 和 `git diff --check` 通过。使用 Node 26 Alpine 容器完成干净依赖安装、管理端构建及首页、`/preview` 启动检查；该容器与 GitHub Ubuntu runner 不完全相同。

**远端验证：** 代码推送后仍需实际 CI 确认。新配置会区分构建失败、服务提前退出和监听探测超时，不能仅凭旧日志断言具体是 DNS 或构建卡顿。
