# 管理端退出与版本选择修复计划

> **执行方式：** 使用 executing-plans 在当前分支逐项执行；每项先观察回归测试失败，再修复、验证并独立提交。

**目标：** 修复已确认的退出输入丢失和手机版本选择错位，保持现有未保存保护与只读浏览。

**架构：** 将会话忙碌状态纳入内容工作区现有 `busy` 契约，复用表单、导航和操作守卫。原生版本选择先恢复已加载版本，再向父组件请求切换；只有加载成功才改变确认值。

**技术栈：** Vue、Nuxt、Vitest、Playwright。

## 任务 1：退出期间冻结工作区

涉及文件：`apps/admin/composables/useAdminWorkspace.ts`、`apps/admin/composables/useStudioWorkspace.ts`、`apps/admin/pages/index.vue`、`apps/admin/components/StudioContent.vue`、`apps/admin/README.md`、`apps/admin/test/unit/studio-shell.test.ts`、`apps/admin/test/e2e/studio-session.spec.ts`。

- [x] 在 HTTP 边界延迟注销响应，断言内容字段、保存、版本选择和导航锁定；素材上传不能新启动；注销失败恢复原表单与操作，成功清理基线。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/studio-shell.test.ts --maxWorkers=4`，确认新增测试因未锁定而失败。
- [x] 将会话状态传入工作区，并让现有操作守卫共用它：

```ts
// useAdminWorkspace 的 session 参数增加可选只读 Ref。
busy?: Readonly<Ref<boolean>>
// busy 计算包含会话操作。
const busy = computed(() => session.busy?.value === true || importing.value || ['loading', 'saving', 'reviewing', 'publishing'].includes(workflow.value.status))
// useStudioWorkspace 调用时传入。
busy: toRef(session, 'busy')
```

- [x] 会话忙碌时将管理 shell 设为 inert，素材上传权限同时要求 `!session.busy`，手机导航传入工作区 `busy`。
- [x] 重跑单元测试与延迟注销浏览器测试；同步 README 的退出锁定与失败恢复说明。
- [x] 完成管理端门禁后，以 `fix(管理端): 锁定退出期间的编辑与操作` 提交实现、测试及对应说明。

## 任务 2：版本选择只显示已确认上下文

涉及文件：`apps/admin/components/StudioVersionBar.vue`、`apps/admin/test/unit/studio-version-bar.test.ts`、`apps/admin/test/e2e/studio-version.spec.ts`。

- [x] 单元测试断言 `change` 发出目标 ID 后，原生选择器仍显示当前版本；父组件更新版本后才显示新版本。
- [x] 浏览器测试在脏首页选择历史版本、取消并再次用键盘选同一目标；确认再次显示未保存对话框。失败加载后也应显示当前版本并允许重试。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/studio-version-bar.test.ts --maxWorkers=4`，确认恢复选择值的断言失败。
- [x] 增加带说明的事件处理器，并替换模板内联 `change`：

```ts
function selectVersion(event: Event) {
  const select = event.target as HTMLSelectElement
  const requested = select.value
  select.value = props.version?.id || ''
  emit('select', requested)
}
```

- [x] 重跑窄测试，执行 `pnpm lint`、`pnpm --filter @yujian/admin typecheck`、管理端全量单元测试与 `pnpm --filter @yujian/admin test:e2e --workers=2`。
- [x] 检查最终 diff 与暂存区，以 `fix(管理端): 恢复取消切换后的版本选择` 独立提交。

## 验证边界

浏览器测试只拦截 HTTP 边界，保持真实组件、原生选择器与权限状态。此次不修改服务端或云发布逻辑，不调用真实云 API。

原生选择器的方向键会立即触发 `change` 并把焦点移入确认弹窗。重选用例只按方向键，随后操作弹窗按钮；不再额外按 Enter，以免误触新弹窗的关闭按钮。

## 验证记录

- 退出回归测试先出现 2 个预期失败，修复后通过；版本选择回归测试先出现 1 个预期失败，修复后通过。
- 管理端全量单元测试 343/343 通过。
- lint 与管理端类型检查通过。
- Chrome 桌面和手机视口完整浏览器测试 86/86 通过，包含 10 个新增用例。
- 独立代码审查未发现需要继续修复的确认问题。
