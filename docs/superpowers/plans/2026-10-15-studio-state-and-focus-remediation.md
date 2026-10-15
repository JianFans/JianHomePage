# 管理端发布状态与焦点修复计划

> **执行方式：** 使用 executing-plans 在当前分支逐项执行。每项遵循 TDD，验证后原子提交，最后独立审查。

**目标：** 修复完整审查复现的发布状态不同步、预览键盘越界和跨记录校验定位问题。

**架构：** 发布操作保留已接受任务，再读取服务端版本状态；同步必须遵守会话 epoch 并保留未保存文本。弹窗仅监听受控同源 iframe 的键盘边界，不改变预览安全消息协议。字段诊断继续复用 canonical JSON Pointer 与现有焦点定位工具。

**技术栈：** Vue、Nuxt、Vitest、Playwright。

## 任务 1：发布状态与版本同步

涉及 `apps/admin/composables/useAdminWorkspace.ts`、`apps/admin/test/unit/admin-workspace.test.ts` 和 `apps/admin/test/e2e/studio-publish.spec.ts`。

- [x] 添加发布创建后 publishing、完成后 published/新修订、回滚和失败状态同步的测试；覆盖版本读取失败仍保留任务、旧任务不替换当前版本、未保存文本和过期会话结果保护。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/admin-workspace.test.ts --maxWorkers=4`，确认状态断言失败。
- [x] 在统一发布操作中记录任务并同步服务端版本，保持忙碌锁直到同步完成；成功时重新读取已加载或当前选中的旧线上版本，反映归档状态和新修订。不同版本仅更新摘要，不重置分页或选中上下文。
- [x] 重跑单测及浏览器状态回归，按 `fix(管理端): 同步发布任务关联的版本状态` 提交。

## 任务 2：预览键盘边界

涉及 `apps/admin/components/StudioDialog.vue`、`apps/admin/test/e2e/homepage-preview.spec.ts`。

- [x] 增加 iframe 末项 Tab 回到弹窗、iframe 内 Escape 关闭、重复打开及忙碌时不关闭的回归。
- [x] 运行 `pnpm --filter @yujian/admin test:e2e homepage-preview.spec.ts --grep '键盘' --workers=2`，确认末项焦点失败。
- [x] 为同源 iframe 文档注册并清理监听；Tab 边界移交到弹窗候选，Escape 复用忙碌守卫；跨源文档不读取。打开期间观察 iframe 插入和移除，关闭或卸载时断开观察器。
- [x] 重跑单测及桌面、手机浏览器回归，按 `fix(预览): 完善嵌入页面的键盘边界` 提交。

## 任务 3：跨记录诊断定位

涉及 `apps/admin/components/VideoWorkbench.vue`、`apps/admin/components/LocalizedTextFields.vue`、`apps/admin/components/MomentWorkbench.vue`、`apps/admin/components/MomentFields.vue` 和对应字段单测、影像浏览器测试。

- [x] 添加非当前影像标题、海报、时长及片段素材、说明、外链的错误定位回归，验证选择、字段路径、无障碍说明与修正后保存。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/workbench-field-issues.test.ts --maxWorkers=4`，确认定位断言失败。
- [x] 补齐记录索引及字段路径，将现有诊断绑定到出错字段；复用共享工具，不改变校验规则。
- [x] 重跑窄测试和浏览器回归，按 `fix(工作台): 定位非当前记录的校验错误` 提交。

## 收尾

- [x] `pnpm lint`、管理端 typecheck、完整单测与覆盖率通过。
- [x] 完整管理端 Chrome E2E 通过；独立审查没有待修复问题。
- [x] 更新管理端 README 和本计划的实际验证记录，检查 diff 与暂存区，将文档作为独立提交。

不执行推送、合并、部署或真实云写操作。

## 验证记录

原始回归分别复现发布状态断言失败、预览末项 Tab 未返回弹窗、非当前记录字段定位失败。独立审查补出的旧线上版本归档同步，以及 iframe 单独移除时释放监听，也先观察失败测试再实现修复。

| 检查 | 结果 |
| --- | --- |
| `pnpm lint` | 通过；发布浏览器测试调整后另跑对应文件 ESLint |
| `pnpm --filter @yujian/admin typecheck` | 通过 |
| `pnpm --filter @yujian/admin test:coverage --maxWorkers=4` | 38 个文件、357 项测试通过；语句 88.69%、分支 80.81%、函数 86.32%、行 91.77%，达到仓库门禁 |
| `PLAYWRIGHT_CHANNEL=chrome pnpm --filter @yujian/admin test:e2e --workers=2` | Nuxt 构建成功，桌面／手机共 106 项通过；环境变量写法适用于 POSIX shell，PowerShell 使用 `$env:PLAYWRIGHT_CHANNEL='chrome'` |
| 独立复审 | 3 个窄单测文件、36 项通过；没有剩余待修复发现 |
| `git diff --check`、分项暂存区检查 | 通过 |

浏览器测试使用真实组件、焦点和 iframe 文档，仅拦截 HTTP 边界；发布测试同时模拟目标发布和旧线上版本归档。它们验证管理端契约与交互，不替代真实 PostgreSQL、OIDC、COS 或 EdgeOne 联调。本次未修改服务端、发布进程或公开站实现，未重复这些模块的完整门禁，也未执行真实云验证。
