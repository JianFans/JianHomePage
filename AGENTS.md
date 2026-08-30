# 仓库代理工作指南

本文件适用于整个仓库。子目录存在更具体的 `AGENTS.md` 时，以距离目标文件最近的规则为准。

## 指令优先级

1. 执行环境中的系统级和安全指令。
2. 用户在当前任务中的明确要求。
3. 距离目标文件最近的 `AGENTS.md`。
4. 用户已批准的规格、实现计划和仓库文档。
5. 现有代码模式与通用工程惯例。

发现冲突时先遵循高优先级要求。只有缺少会实质改变结果的信息时才停止提问；其余情况应读取代码、做保守假设并继续完成任务。

## 开始工作

执行任何修改前：

1. 阅读当前目录及父目录中适用的 `AGENTS.md`。
2. 检查仓库根目录是否存在 `.codegraph/`。
3. 运行 `git status --short --branch`，确认分支、暂存区和用户已有改动。
4. 阅读相关模块、测试、专项 README 和最近提交，不根据文件名猜测行为。
5. 对多步骤任务创建计划；每次只保留一个进行中的步骤。
6. 在编辑前向用户简要说明准备修改什么以及为什么。

工作区可能不是干净的。所有未知改动都视为用户内容：不要回退、覆盖、暂存或提交与当前任务无关的文件。

## CodeGraph

仓库根目录存在 `.codegraph/` 时，在使用 `rg`、`find` 或逐文件阅读定位代码前，优先运行：

```bash
codegraph explore "<符号、文件或问题>"
```

CodeGraph 用于获取符号源码和调用路径。若 `.codegraph/` 不存在，不要自行创建索引，直接使用 `rg` 和 `rg --files`。

## Skills 工作流

执行环境提供 Skills 时，先完整读取适用的 `SKILL.md`，再进行任务操作。用户点名的 Skill 必须使用；未点名但明显适用的 Skill 也必须使用。

| 场景 | 必需流程或 Skill |
| --- | --- |
| 开始任务 | `using-superpowers` |
| 新功能、组件、行为或创造性修改 | `brainstorming`，获得设计确认后再实现 |
| 多步骤实现 | `writing-plans`，随后使用允许的计划执行流程 |
| 功能或缺陷修复 | `test-driven-development`，先写失败行为测试 |
| Bug、失败测试或异常行为 | `systematic-debugging`，先定位根因 |
| 接收审查意见 | `receiving-code-review`，先验证意见再修改 |
| 请求合并前审查 | `requesting-code-review` 与 `chinese-code-review` |
| 中文 README、运维或技术文档 | `chinese-documentation` |
| 中文提交信息 | `chinese-commit-conventions` |
| 宣称完成、修复、测试通过或准备提交 | `verification-before-completion` |
| 开发工作收尾 | `finishing-a-development-branch` |

Skills 与用户指令冲突时，以用户明确要求为准。不要因为流程复杂而跳过适用 Skill，也不要引入任务范围之外的 Skill 产物。

除非用户或适用流程明确要求，不要创建子代理、额外任务、工作树、分支、PR 或远端操作。

## 仓库布局

| 路径 | 所有权与职责 |
| --- | --- |
| `apps/web` | Nuxt 公开静态站、i18n、SEO、试听与 E2E |
| `apps/admin` | Nuxt/Vue 内容管理端 |
| `apps/server` | Go 内容、素材、鉴权、审计和发布服务 |
| `packages/schema` | canonical 内容 Schema、生成类型和语义校验 |
| `content/fixtures` | 仅供开发和测试的内容快照 |
| `scripts` | fixture、EdgeOne 和静态产物工具 |
| `docs/superpowers/specs` | 已批准的设计规格 |
| `docs/superpowers/plans` | 可执行实现计划和运维计划 |

模块专属细节优先阅读：

- `apps/web/EDGEONE.md`
- `apps/admin/README.md`
- `apps/server/README.md`

## 架构与契约

以下约束不可在普通功能修改中破坏：

- `packages/schema/schema/content-snapshot.schema.json` 是内容快照的唯一规范。
- 不手工修改 `packages/schema/src/generated.ts`、`apps/server/internal/snapshot/generated.go` 或其他生成文件。
- Schema 变更后运行 `pnpm schema:generate` 和 `pnpm verify:go`，保持 TypeScript、Go 和 OpenAPI 副本一致。
- 公开站只从构建时快照渲染，不添加浏览器端运行时内容 API。
- `content/fixtures` 只用于开发和测试，EdgeOne 正式构建必须拒绝 fixture 路径。
- 第三方音乐和社交站点是外链目标，不是运行时数据源。
- 内部跳转必须指向实际生成的稳定锚点，并遵守板块启用、编排、限制和时间窗口。
- 素材类型、MIME、大小、SHA-256、权利信息和稳定公开地址必须相互一致。
- PostgreSQL、OIDC、S3/COS、EdgeOne 等基础设施通过显式接口接入，不把服务商 SDK 类型泄漏到领域模型。
- 生产模式不得静默降级为内存仓储、开发身份或本地对象存储。

## 设计与实现

### 设计

- 创造性或行为修改先探索现状、明确成功标准并获得设计确认。
- 方案优先复用现有框架、组件、领域接口和测试工具。
- 只添加能降低真实复杂度、减少有意义重复或符合现有模式的抽象。
- 不进行与当前任务无关的重构、依赖升级、格式化或元数据改动。

### TDD

行为变更遵循以下顺序：

1. 添加能复现需求或缺陷的失败测试。
2. 运行最窄测试，确认失败原因符合预期。
3. 编写最少实现使测试通过。
4. 重新运行窄测试。
5. 运行受影响模块测试。
6. 根据风险扩大到完整门禁。

文档、纯格式和生成产物同步可以不添加行为测试，但仍要验证事实、命令、链接和 diff。

### 调试

- 先读取完整错误输出并复现问题，不根据单个堆栈片段猜修复。
- 区分代码缺陷、测试缺陷、环境权限、网络和服务商故障。
- 修复根因，不通过放宽断言、吞掉错误或关闭安全检查让测试变绿。
- 同一阻塞连续出现时记录已验证事实，再决定是否需要用户输入。

## 前端规则

- 延续现有低饱和深色视觉语言，不添加深浅色切换。
- 首页是实际展示体验，不添加营销式落地页、说明卡片或大段功能说明。
- 保持图标优先和文字克制；工具按钮优先使用 `@lucide/vue` 图标并提供可访问名称。
- 保持响应式布局、稳定尺寸、键盘操作、触控目标和 `prefers-reduced-motion` 支持。
- 不嵌套装饰性卡片，不使用会遮挡内容的浮层，不让动态内容导致布局跳动。
- 首页语言不写入 URL。优先使用 `localStorage`，其次使用浏览器标准语言 API，最后回退默认语言并显示非侵入式提示。
- 所有外链使用安全的 `target` / `rel` 组合；平台按钮不能替代无障碍文本。
- 音乐试听必须保持单实例播放器，异步媒体事件不得覆盖当前曲目状态。
- SEO、canonical、Open Graph、JSON-LD、robots 和 sitemap 必须由内容快照驱动。

## 服务端规则

- 保持领域服务依赖小而明确的接口，适配器负责服务商协议和 URL 细节。
- 所有生产配置在启动阶段严格校验；秘密信息只从环境变量或密钥服务读取，不写入日志、测试 fixture 或提交。
- OIDC 只信任配置的 issuer、audience 和已验证签名；管理写操作继续执行 RBAC。
- 内容版本使用 canonical JSON 与匹配 checksum；数据库 JSONB 读回后也必须验证。
- 素材上传必须验证允许的 MIME、扩展名、大小、SHA-256 和权利结构。
- 发布、回滚和状态刷新必须保持幂等、单活动任务和乐观并发约束。
- 发布目标在构建期间保持冻结；任务失败或目标漂移时按领域规则释放状态。
- 数据库迁移必须可重复检查、在事务中执行，并为生产升级提供明确前置条件。

## 编辑规则

- 查找文件和文本优先使用 `rg --files` 与 `rg`。
- 手工修改文件使用 `apply_patch`；不要使用 shell 重定向、`cat` 或 Python 脚本进行简单读写。
- 默认使用 ASCII；文件已有中文且中文能提高可读性时可以使用 UTF-8。
- 注释只解释不明显的原因、约束或复杂算法，不复述代码。
- 使用结构化解析器处理 JSON、YAML、OpenAPI 和 Schema，不进行脆弱的字符串拼接。
- 不使用 `git reset --hard`、`git checkout --` 或其他破坏性命令，除非用户明确要求并确认目标。
- 不回退用户已有改动。任务文件同时包含用户修改时，先理解并在其基础上工作。
- 不读取、输出或提交秘密信息；命令输出中出现凭据时立即停止传播。

## 命令与验证

### 根目录

| 命令 | 验证范围 |
| --- | --- |
| `pnpm lint` | ESLint |
| `pnpm typecheck` | Schema、管理端和公开站类型检查 |
| `pnpm test` | JavaScript、Vue 和脚本测试 |
| `pnpm generate` | Schema 类型与 Nuxt 静态生成 |
| `pnpm verify` | 前端完整门禁和静态产物检查 |
| `pnpm verify:go` | Go generate、全包测试和 Go vet |
| `pnpm verify:edgeone` | 正式快照约束下的 EdgeOne 完整门禁 |

### 按改动范围

- Schema：`pnpm --filter @yujian/schema test`、`pnpm schema:generate`、`pnpm verify:go`。
- 公开站：对应 Vitest 文件、`pnpm --filter @yujian/web typecheck`、`pnpm --filter @yujian/web generate`。
- 首页交互、响应式或无障碍：`pnpm --filter @yujian/web test:e2e`。
- 管理端：`pnpm --filter @yujian/admin test`、`pnpm --filter @yujian/admin typecheck`。
- Go 服务：在 `apps/server` 运行目标包测试，再运行 `pnpm verify:go`。
- EdgeOne 配置或构建快照：使用非 fixture 的 `CONTENT_SNAPSHOT_PATH` 运行 `pnpm verify:edgeone`。

完成声明和提交前必须运行能证明结论的最新命令，并阅读退出码与失败数。不得用「之前通过」「理论上可行」或局部测试替代当前证据。

## 文档规则

- 中文说明使用自然短句、全角标点，并在中文与英文、数字之间留空格。
- 专有名词保留标准写法，例如 Nuxt、Vue、PostgreSQL、OIDC、EdgeOne。
- 标题层级连续，代码块标注语言，命令可以直接运行。
- 根 README 提供稳定入口；完整变量、API 和缓存规则放在模块专项文档中。
- 不留下占位符、未确认承诺、临时机器状态或无法验证的上线结论。

## Git 与提交

- 只有用户明确要求时才创建提交、推送、PR、标签或发布。
- 在 `master` 直接提交必须有用户明确授权；否则使用适当分支或 worktree。
- 提交前运行 `git diff --cached --name-status` 和 `git diff --cached --check`，确认没有遗漏或混入无关文件。
- 使用 Conventional Commits：type 使用英文，scope 和 subject 使用中文。
- subject 使用动宾短语，不超过 50 个字符，不以句号结尾。
- 每个提交只表达一个可独立理解和回滚的职责；测试与实现放在同一提交。
- 数据库、公共 API 或配置不兼容变更必须在 footer 写明 `BREAKING CHANGE` 和迁移方式。
- 不修改、折叠、重写或强制推送既有提交，除非用户明确要求。

示例：

```text
fix(发布链路): 冻结构建期间的目标版本
docs(仓库): 完善项目与代理工作指南
chore(依赖): 替换存在安全公告的版本
```

## 代码审查

用户要求 review 时采用 findings-first：

1. 按严重程度列出真实缺陷、行为回归、安全风险和缺失测试。
2. 每项提供文件和尽可能精确的行号，并说明触发条件与影响。
3. 区分已验证问题、合理风险和需要外部环境确认的事项。
4. Findings 之后再写开放问题、假设和简短变更摘要。
5. 没有发现问题时明确说明，同时列出剩余测试缺口和真实云环境风险。

收到审查意见后先在当前代码库验证技术正确性。不要因为意见来自审查者就盲目实施，也不要用礼貌性附和替代分析。

## 上线完成标准

代码测试通过不等于已经上线。宣称可生产发布前还要确认：

- EdgeOne 构建使用非 fixture 的审核快照。
- PostgreSQL、OIDC、S3/COS、稳定媒体域名和 EdgeOne 参数完整。
- 首次执行 `0003_publish_target_freeze.sql` 前没有活动发布任务。
- COS 直传 CORS、EdgeOne `/api/*` 回源、TLS 和域名配置已验证。
- 真实环境通过健康检查、首页、素材、发布、回滚、robots 和 sitemap 冒烟。
- 回滚方案、不可变快照和历史发布指针可用。

无法访问真实云账号时，应明确报告「代码与本地门禁通过，但真实云集成尚未验证」，不能推断上线成功。

## 完成检查清单

- 需求与最新用户消息一致。
- 适用 Skills 已完整读取并遵循。
- 变更范围聚焦，没有覆盖用户已有内容。
- 失败测试先出现，修复后窄测试与受影响套件通过。
- 生成文件由生成命令更新且副本一致。
- `git diff --check` 通过。
- 需要提交时，暂存区只包含当前职责并使用中文原子提交。
- 最终回复说明改动、验证证据、未执行项目和真实剩余风险。
