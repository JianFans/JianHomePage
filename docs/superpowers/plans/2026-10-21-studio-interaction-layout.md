# 管理端交互布局实现计划

> **面向 AI 代理的工作者：** 使用 executing-plans 逐任务实现。用户已授权独立完成，不等待交互式确认。

**目标：** 分离版本上下文与模块标题，让审核发布在各模块可达，并优化导航和长表单操作。

**架构：** 复用现有 StudioWorkspace，将历史页工作流按钮提取为版本上下文组件。仅调整 Vue 结构、双语文案与 CSS，不修改 API。

**技术栈：** Nuxt、Vue、TypeScript、Vitest、Playwright。

**规格：** [管理端交互布局优化](../specs/2026-10-21-studio-interaction-layout-design.md)。

## 全局约束

- 从本地 master 的 be34941 新建 feat/studio-interaction-layout，保留已有未推送提交。
- 不推送或部署；按职责中文签名提交。
- 保持未保存保护、RBAC、发布确认、只读浏览、素材表单保活和实时减少动态效果。

## 审查重点（Review Focus）

- 不同角色与版本状态组合不应出现越权可用动作。
- 未保存模块表单不能绕过审核发布禁用条件。
- 长名称、320 px 窄屏和短视口不能隐藏操作或横向溢出。
- 吸顶后键盘定位的输入必须在可视范围内。
- 模块切换和版本切换后操作目标必须与显示版本一致。

## 文件职责

- StudioVersionWorkflow.vue：基于已有 workspace 能力呈现下一步动作。
- pages/index.vue：版本上下文、模块标题与全局动作布局。
- StudioContent.vue：去除重复版本 banner 与历史页操作。
- StudioNavigation.vue、studio-copy.ts：导航分组和双语名称。
- studio.css：响应式布局、吸顶顶栏与滚动定位间距。
- studio-shell.test.ts、studio-layout.spec.ts：状态／角色、工作流入口和真实浏览器布局回归。

### 任务 1：集中版本上下文与工作流

- [ ] 在 studio-shell.test.ts 添加各模块可达下一步、未保存禁用、角色／状态过滤与模块标题测试；更新已通过审核用例，断言不再显示审核动作。
- [ ] 运行 `pnpm --filter @yujian/admin test -- test/unit/studio-shell.test.ts`，确认因布局／入口缺失失败。
- [ ] 提取 StudioVersionWorkflow（studio、locale props，workflow 事件），复用 canSubmitReview／canApprove／canPublish／canRollback；放入 index 的版本上下文；模块 h1 独立；精简概览和历史。
- [ ] 重跑该测试并运行管理端 typecheck，通过后提交实现和测试。

### 任务 2：导航分组与长表单可达性

- [ ] 添加分组语义测试及 studio-layout.spec.ts 浏览器测试：各模块标题、工作流目标、未保存保护、320 px 长名称、短视口、滚动后保存与键盘控件可见。
- [ ] 运行新增测试，确认分组／吸顶尚未满足。
- [ ] 侧栏按用途分组；顶栏单行吸顶，配置 scroll-padding 避免聚焦遮挡；自然换行版本上下文，不固定整套版本栏。
- [ ] 运行管理端单元测试、typecheck、lint、完整 E2E；检查真实截图和动效／减少动态效果，更新 README 后原子提交。

### 任务 3：完整审查与收尾

- [ ] 依据规格独立审查 be34941..HEAD，验证发现，只修有效缺陷。
- [ ] 运行 `pnpm verify`，确认生成与静态产物无回归；UI 交互以管理端完整 E2E 为门禁。
- [ ] 确认 diff、签名、分支与工作区，记录证据；不推送、不部署。
