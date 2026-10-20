# 按标签关联 Actions 发布运行

**目标：** 将版本准备与构建发布拆开，使发布运行关联实际版本提交。

**方案：** 按用户最新要求，`release-prepare.yml` 使用仓库 Secret `RELEASE_TOKEN` 手动同步版本并原子推送 commit/tag。`release.yml` 仅接受 tag push，不提供 dispatch 入口，不由其他工作流调用；校验稳定版本标签、提交身份及 Node／Go 文件版本后复用完整 CI、发布镜像和裸二进制。移除原来的 dispatch job。默认 `GITHUB_TOKEN` 不能触发后续 push 工作流，因此不允许作为准备阶段推送凭据的后备。保留现有草稿续传与拒绝覆盖行为，不触发真实发布。

## 执行步骤

- [x] 修改 `scripts/release-workflow.test.mjs` 与 `scripts/automation-config.test.mjs`，先验证仅 tag push 触发、独立推送凭据和缺少凭据提前失败；运行目标测试确认旧实现失败。
- [x] 准备工作流移除 dispatch job，发布工作流移除手动入口；保留 tag 及版本校验。测试实际 Bash 的版本准备、合法及非法发布引用、版本不符和标签漂移。
- [x] 同步根 README、服务端 README 和部署说明，明确第一次手动发版、原 tag 补跑及触发权限。
- [x] 运行 `pnpm test:automation`、`pnpm lint`、actionlint 与 Bash 语法检查，检查 diff。不创建远端发布。

**成功标准：** 发布工作流事件为 tag push，所有检查与构建使用该运行的 SHA；准备工作流只推送，不调度其他工作流。缺少独立凭据时不修改版本，且原子推送、版本一致性和不可覆盖发布测试仍通过。

**验证结果：** 34 项自动化测试、ESLint、actionlint 及 `git diff --check` 通过；发布测试执行实际 Bash，覆盖版本准备、原子推送、标签校验及草稿续传。公开 GitHub API 实测可通过完整标签引用解析 commit，兼容附注标签且避免同名分支歧义。未触发真实 Actions；运行关联及仓库权限仍需在代码推送后实际发版确认。
