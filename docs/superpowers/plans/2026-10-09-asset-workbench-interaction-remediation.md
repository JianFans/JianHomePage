# 素材工作台交互与关闭边界修复计划

> 使用 executing-plans 在当前已授权分支内逐项实现；已有审查和用户修复授权，不创建新分支、不部署。

**目标：** 修复上传编辑丢失、跨 API 重试、超时测试误报、关闭超时和非法编码查询五项问题。

**架构：** 上传时锁定表单。API 地址变化使素材工作区旧操作失效，并清除连接所属列表、游标、重试和替代文本缓存；同一 API 的 Token 更新保留确认重试。HTTP 关闭超时后主动断开连接，再等待素材文件清理。查询解析错误直接返回 400。

**技术栈：** Vue、Vitest、Go net/http。

## 任务 1：上传表单冻结

文件：`apps/admin/components/AssetWorkbench.vue`、`apps/admin/test/unit/asset-workbench-component.test.ts`。

- [x] 以延迟创建上传响应挂起真实组件，断言文件和六个文本输入均 disabled；完成后断言卡片保留原 alt 且输入重新启用。先运行失败测试。
- [x] 对输入绑定 `:disabled="activeUpload"`，保持失败后的编辑和确认重试能力。
- [x] 运行 `pnpm --filter @yujian/admin exec vitest run test/unit/asset-workbench-component.test.ts` 与 typecheck，原子提交。

## 任务 2：连接归属与在途结果

文件：`apps/admin/composables/useAssetWorkspace.ts`、`apps/admin/components/AssetWorkbench.vue` 及两个对应测试文件。

- [x] 先测试 A 的确认失败后切换 B：旧 retry 不发送请求，旧列表、游标和完成记录清空。延迟 A 的列表与摘要结果，切换 B 并加载后确认 A 结果不能写回或继续创建上传；同一 A 更新 Token 后确认重试使用新 Token。
- [x] 对规范化 API 地址建立同步 watch，递增上传和列表序号，清除连接所属状态，保留文件和表单草稿。组件清除旧连接替代文本与待恢复文本，防止同 ID 素材继承旧值。
- [x] 运行 `pnpm --filter @yujian/admin test` 与 typecheck，原子提交。

## 任务 3：期限测试稳定性

文件：`apps/server/cmd/api/main_test.go`。

- [x] 使用已有失败证据：原测试重复 20 次出现 3 次 control 误报。
- [x] control handler 记录 `io.Copy` 的实际错误，通过 `net.Error.Timeout()` 断言读期限生效；等待客户端延迟送出请求体后才允许写响应，保证默认读写期限均已过期。签名 PUT 保持延迟请求体和 204 断言，不放宽生产期限或删除 control。
- [x] 在 `apps/server` 执行 `go test ./cmd/api -run TestLocalUploadOutlivesAPIDeadlines -count=50`，再运行目标包测试，原子提交。

## 任务 4：有界关闭

文件：`apps/server/cmd/api/main.go`、`apps/server/cmd/api/main_test.go`。

- [x] 用真实 TCP PUT、只发送部分请求体，并以请求体读取信号确认上传已开始；取消服务上下文，断言服务及本地存储清理在有界时间内返回。原代码因清理等待未读完的请求体而失败；测试清理主动断开客户端防止挂住。
- [x] `Shutdown` 失败时执行 `server.Close()`，保留原关闭错误并合并 Close 错误，再清理依赖。正常优雅关闭不受影响。
- [x] 关闭回归测试连续运行 20 次通过，再执行 `go test ./cmd/api ./internal/providers/local -count=1`，原子提交。

## 任务 5：严格解析查询

文件：`apps/server/internal/httpapi/handler.go`、`apps/server/internal/httpapi/handler_routes_test.go`。

- [x] 增加 `cursor=%ZZ`、`limit=%ZZ` 和合法参数混入非法编码的用例，断言 400 且服务未调用。四个新增用例修复前均返回 200，修复后通过。
- [x] 使用 `url.ParseQuery(request.URL.RawQuery)`，遇解析错误立即拒绝，保持现有重复参数、范围和游标校验。
- [x] 在 `apps/server` 执行 `go test ./internal/httpapi -count=1`，原子提交。

## 最终门禁

- [x] 独立只读复审 `197e882..f0888a3`，未发现需要修复的缺陷；复审另行运行管理端两份测试及后端目标回归测试。
- [x] 运行 `pnpm verify`、`pnpm verify:go`、`pnpm test:coverage`、`pnpm test:coverage:go`。
- [x] 检查生成副作用与 `git diff --check`，记录验证事实并单独提交本计划。真实云集成仍不在本次修复范围。

## 验证记录

| 命令或检查 | 结果 |
| --- | --- |
| `pnpm verify` | lint、typecheck、前端测试、脚本测试、静态生成及产物校验通过；Schema 19 项、管理端 79 项、公开站 82 项、脚本 21 项测试通过 |
| `pnpm verify:go` | gofmt 检查、Go generate、全包测试和 go vet 通过 |
| `pnpm test:coverage` | 覆盖率门禁通过；语句覆盖率：Schema 90.47%、管理端 89.52%、公开站 87.43% |
| `pnpm test:coverage:go` | 全包语句覆盖率 82.37%，门槛 80.00% |
| 上传期限回归测试 `-count=50` | 连续 50 次通过 |
| 活动上传关闭回归测试 `-count=20` | 连续 20 次通过 |
| 独立复审 | 无需修复项；两份管理端测试 22 项通过，后端目标回归测试重复 3 次通过 |
| `git diff --check` | 通过；Nuxt 测试工具对 `.nuxtrc` 的版本改写已恢复，未纳入提交 |

## 验证边界

本轮未修改数据库或基础设施适配器，不重跑 PostgreSQL 实库测试，也不进行推送和部署。真实 COS、OIDC、EdgeOne 和云端关闭行为尚未验证，不能据此宣称已经上线。

本轮未执行 Go race 检测。API 切换回归测试显式覆盖列表和摘要阶段的迟到结果；签名 PUT、完成确认及错误返回仍复用相同的序号保护，未逐阶段增加挂起用例。
