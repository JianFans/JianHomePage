# 管理表单诊断补齐计划

> **面向 AI 代理的工作者：** 使用 `executing-plans` 在当前工作树内执行；用户已授权修复审查发现并原子化提交。

**目标：** 保存失败时定位实际错误输入，展开全部所属折叠区域，并提供当前语言的字段描述。

**架构：** 复用 `issueFieldAttributes` 和 `LocalizedTextFields`，由各工作台传递 canonical JSON Pointer 与完整诊断。曲目路径使用应用事务实际生成的数组顺序，不使用显示顺序。保持统一 Toast 和完整快照校验。

**技术栈：** Vue、TypeScript、Vitest、Playwright。

## 任务 1：补齐字段诊断

文件范围：`apps/admin/components/PlatformLinkFields.vue`、`MusicWorkbench.vue`、`MusicTrackFields.vue`、`VideoWorkbench.vue`、`EventWorkbench.vue`、`HeroSlideFields.vue`、`HomepageSectionFields.vue`、`HomepageWorkbench.vue`，以及 `apps/admin/utils/issue-accessibility.ts`、`music-workbench.ts`。

- [x] 在 `apps/admin/test/unit/workbench-field-issues.test.ts` 添加真实工作台回归。覆盖 HTTP 平台链接、空作品日期、曲目署名、宣传位外链／时间／焦点、板块更多链接／上限，以及非当前现场记录。
- [x] 运行 `pnpm --filter @yujian/admin test test/unit/workbench-field-issues.test.ts --maxWorkers=4`，确认失败来自未绑定路径或未展开折叠区域。
- [x] 透传链接与双语字段的 `path`、`issues`，为日期和数值输入绑定诊断及描述。抽取曲目合并顺序供应用与字段路径共用。
- [x] `focusSnapshotIssue` 展开错误字段的全部祖先 `details`，覆盖链接自定义英文标签的嵌套结构。
- [x] 重跑上述窄测试，要求错误输入获得焦点、`aria-invalid` 为 `true`、描述含准确路径和当前语言消息；修正后可应用且清除错误属性。

测试核心断言：

```ts
expect(form.apply()).toBe(false)
expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
expect(document.activeElement).toBe(field.element)
expect(field.attributes('aria-invalid')).toBe('true')
expect(description(field.element)).toContain(form.issues[0]!.path)
```

## 任务 2：浏览器验证与提交

- [x] 扩充 `apps/admin/test/e2e/studio-field-issues.spec.ts`，覆盖音乐／影像 HTTP 平台链接和清空作品日期；折叠区域关闭时保存也应展开并定位。检查修正后只提交有效快照。
- [x] 依次执行管理端覆盖率、类型检查、完整 E2E；另运行根目录 `pnpm lint` 和 `git diff --check`。避免同一 Nuxt 模块的测试、类型检查和构建并发。
- [x] 同步 `apps/admin/README.md` 的字段错误说明，审查最终 diff，清理本轮测试生成的无关元数据。
- [x] 检查暂存区后提交 `fix(管理表单): 补齐字段诊断与折叠区域定位`，不推送或发布。

## 验证记录

- 新增回归首先复现字段未绑定路径、嵌套区域未全部展开的问题；修复后两份窄测试共 57 项通过。
- 管理端单元测试 385 项通过，语句覆盖率 89.18%、分支覆盖率 81.03%、函数覆盖率 87.18%、行覆盖率 92.10%，满足现有门禁。
- 管理端类型检查、根目录 lint 和 diff 空白检查通过；桌面与移动端完整 E2E 共 122 项通过。
- 独立代码审查未发现新的有效问题。浏览器保存场景使用拦截 API 验证契约，未执行真实云环境发布验证。
