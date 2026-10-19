# Actions 编译与程序发布计划

> 执行方式：使用 writing-plans 和 executing-plans，在当前任务内逐项实现。用户已指定参考 NaiList 的工作流组织，并要求编译在 Actions 完成；不运行本地项目编译，不触发真实发布。

**目标：** 通过 GitHub Actions 获取包含管理端、API、页面生成和 Pages 上传功能的 Go 单程序，无需在开发电脑或服务器编译。

**设计：** 保留 `code-check.yml` 的 push、PR、手动入口，增加 `workflow_call`，所有任务检出同一提交。Go 检查完成后保存内嵌资源，二进制任务复用这些资源，交叉编译 Linux amd64、arm64 并打包。`release.yml` 仅手动触发，选择 patch、minor、major，从稳定版本标签计算下一版本；复用完整检查，成功后为已验证提交打标签，再发布 GHCR amd64 镜像和 GitHub Release。Release 先上传至草稿，文件齐全后再公开；重跑核对已有文件，只补传草稿中的缺失文件，拒绝覆盖。项目尚无应用版本字段，因此不新增仅为发布而维护的版本提交。

**边界：** 编译不接触生产密钥、不连接腾讯云、不自动替换服务器程序。发布流程冻结默认分支的启动提交；标签不得覆盖；二进制包附校验和，所有下载只读取本次运行的产物。普通 CI 仅只读，写标签、镜像和 Release 的权限只授予对应发布任务。版本从 `v0.0.0` 起计算，第一次选择 minor 得到 `v0.1.0`。

**文件：** `.github/workflows/code-check.yml`、`.github/workflows/release.yml`、`scripts/release-version.mjs`、`scripts/release-version.test.mjs`、`scripts/release-workflow.test.mjs`、`scripts/automation-config.test.mjs`、`package.json` 测试入口、`README.md`、`docs/DEPLOYMENT.md`、`apps/server/README.md`。

## 任务 1：发布版本与工作流契约

- [x] 为稳定版本排序、三种递增、非法选项和预发布标签编写 `node:test` 用例，运行并确认缺失实现时失败。
- [x] 编写最小版本计算脚本，使用 BigInt 避免数字版本排序和精度问题；加入仓库测试入口。
- [x] 扩展 YAML 结构测试，约束复用入口、固定检出、门禁依赖、产物传递、发布权限与手动触发，先运行确认失败。

## 任务 2：Actions 构建与发布

- [x] 为 `code-check.yml` 增加 `workflow_call` 的 `ref`、`version`；各任务使用 `inputs.ref || github.sha` 检出。
- [x] Go 门禁后将内嵌资源打成 tar 并上传；Linux 双架构任务下载、解包并编译 `CGO_ENABLED=0` 单程序，amd64 执行健康检查及管理页面冒烟，然后上传 tar.gz。
- [x] 新建手动 `Release`：prepare 冻结默认分支提交并计算版本；checks 调用完整门禁；tag 只创建指向固定提交的新标签；image 发布 GHCR；publish 核实标签、合并二进制下载并生成 SHA256SUMS.txt 后创建 Release。
- [x] 执行 `pnpm test:automation`、版本脚本测试与 actionlint；不本地编译项目或镜像。
- [x] 执行真实工作流 Bash，通过本地 GitHub CLI 替身验证首次发布、草稿续传、公开响应丢失、文件不一致、已公开缺文件和标签漂移；不连接真实账号。

## 任务 3：文档与收尾

- [x] 部署指南第 2 节改为 Actions 点击路径、首次版本选择、架构选择、下载、校验、解压和安装。
- [x] 同步 README 与服务端文档；说明 Actions 编译不等于服务器部署，云密钥继续只放服务器。
- [x] 检查 diff 与文档链接，运行 lint 和脚本测试；独立审查并修复有效问题，按工作流和部署文档分别原子提交。

**验证说明：** 本地只检查脚本、YAML、shell 和文档；Linux 编译、Docker 构建与实际 Release 上传由推送后的 Actions 验证。不得把本地配置检查描述成远端构建成功。

本地验证结果：自动化测试 19 项通过，ESLint、actionlint、35 个 Bash 步骤语法和 23 个文档本地链接检查通过。独立审查发现的 Release 中断恢复问题已修复并通过复查；没有运行项目编译、镜像构建或远端发布。
