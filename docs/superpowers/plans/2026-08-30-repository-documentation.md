# 仓库说明文档实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 重写根目录 README 与代理指南，使项目事实、开发命令、上线门禁和代理工作流清晰且可执行。

**架构：** README 面向人工维护者，提供项目入口和上线清单；AGENTS 面向编码代理，提供仓库级强制规则。详细服务端与 EdgeOne 配置继续链接到各模块文档。

**技术栈：** Markdown、pnpm workspace、Nuxt、Vue、Go、PostgreSQL、OIDC、S3/COS、EdgeOne。

---

## 文件职责

- 修改：`README.md`，项目定位、架构、快速开始、命令、数据流和上线清单。
- 修改：`AGENTS.md`，代理启动流程、Skills、契约、TDD、验证、审查和提交规范。

### 任务 1：重写项目 README

- [ ] **步骤 1：核对项目事实**

运行：

```powershell
Get-Content package.json
Get-Content pnpm-workspace.yaml
Get-Content apps/server/go.mod
```

预期：确认 Node/pnpm/Go 版本、脚本名称和 workspace 布局。

- [ ] **步骤 2：改写 README**

使用以下固定章节：项目定位、当前范围、系统结构、关键约束、快速开始、命令矩阵、内容发布流程、生产上线、文档索引。

- [ ] **步骤 3：检查 README 中的命令和链接**

运行：

```powershell
rg -n "pnpm|apps/|packages/|docs/" README.md
```

预期：所有命令和相对路径都能在仓库中找到对应目标。

### 任务 2：重写代理指南

- [ ] **步骤 1：写入仓库启动流程**

规定先读取最近的 `AGENTS.md`、检查 `.codegraph/`、Git 状态和相关文件；保留用户已有改动。

- [ ] **步骤 2：写入开发与审查流程**

规定 Skills 路由、先设计后实现、TDD、系统化调试、生成文件约束、分层测试、原子提交和 findings-first 审查。

- [ ] **步骤 3：写入上线完成标准**

明确静态快照、生产变量、迁移、CORS、EdgeOne 回源和真实云环境冒烟要求。

### 任务 3：验证与提交

- [ ] **步骤 1：检查文档格式与占位符**

运行：

```powershell
rg -n "TODO|待定|稍后补充" README.md AGENTS.md
git diff --check
```

预期：搜索无结果，diff 检查退出码为 0。

- [ ] **步骤 2：运行完整门禁**

运行：

```powershell
pnpm verify
pnpm verify:go
pnpm --filter @yujian/web test:e2e
```

预期：所有命令退出码为 0。

- [ ] **步骤 3：创建原子提交**

```powershell
git add README.md AGENTS.md
git commit -m "docs(仓库): 完善项目与代理工作指南"
```
