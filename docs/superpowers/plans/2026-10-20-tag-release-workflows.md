# 按标签关联 Actions 发布运行

**目标：** 将版本准备与构建发布拆开，使发布运行关联实际版本提交。

**方案：** `release-prepare.yml` 手动同步版本并原子推送 commit/tag，独立 dispatch job 使用 `GITHUB_TOKEN` 的 `actions: write` 权限，在该 tag 上触发 `release.yml`。后者同时接受 tag push 和手动触发，仅允许稳定版本标签，核对提交身份及 Node／Go 文件版本后复用完整 CI、发布镜像和裸二进制。保留现有草稿续传与拒绝覆盖行为。不需要新增 PAT，不触发真实发布。

## 执行步骤

- [x] 修改 `scripts/release-workflow.test.mjs` 与 `scripts/automation-config.test.mjs`，先验证工作流职责、tag dispatch、权限隔离及发布身份；运行 `pnpm test:automation` 确认旧实现失败。
- [x] 拆分两个 YAML；准备推送与 dispatch 使用独立 job；发布解析 tag 并检查版本。测试实际 Bash 的版本准备、合法及非法发布引用、版本不符和标签漂移。
- [x] 同步根 README、服务端 README 和部署说明，明确第一次手动发版、原 tag 补跑及触发权限。
- [x] 运行 `pnpm test:automation`、`pnpm lint`、actionlint 与 Bash 语法检查，检查 diff。不在本地编译项目，不创建远端发布。

**成功标准：** 发布工作流由 tag ref 触发，所有检查与构建使用该运行的 SHA；准备工作流不再构建。dispatch 失败可独立补跑，且原子推送、版本一致性和不可覆盖发布测试仍通过。

**验证结果：** 33 项自动化测试、ESLint、actionlint、37 个 Shell 步骤的 Bash 语法及 `git diff --check` 通过。公开 GitHub API 实测可通过完整标签引用解析 commit，兼容附注标签且避免同名分支歧义。未触发真实 Actions；运行关联及仓库权限仍需在代码推送后实际发版确认。
