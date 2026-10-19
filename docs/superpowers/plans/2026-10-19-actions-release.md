# Actions 编译与程序发布计划

> 执行方式：使用 writing-plans 和 executing-plans，在当前任务内逐项实现。用户已指定参考 NaiList 的工作流组织，并要求编译在 Actions 完成；不运行本地项目编译，不触发真实发布。

**目标：** 通过 GitHub Actions 获取包含管理端、API、页面生成和 Pages 上传功能的 Go 单程序，无需在开发电脑或服务器编译。

**设计：** 保留 `code-check.yml` 的 push、PR、手动入口和 `workflow_call`。`release.yml` 仅手动触发，选择 patch、minor、major，从实际文件版本与稳定标签的较高值计算下一版本。准备阶段同步根目录、公开站、管理端、Schema 包的 `package.json` 和 Go 应用版本文件，创建版本提交和标签，通过一次原子推送更新默认分支与标签。所有门禁、内嵌资源、Linux 双架构单程序和 GHCR 镜像固定使用该版本提交。Release 先上传至草稿，文件齐全后再公开；重跑核对已有文件，只补传草稿中的缺失文件，拒绝覆盖。Go 工具链版本与内容 Schema 契约版本不随应用发版修改。

**边界：** 编译不接触生产密钥、不连接腾讯云、不自动替换服务器程序。准备阶段要求启动提交仍是最新默认分支；分支发生并发更新或标签冲突时原子推送失败，不留下单边远端修改。标签不得覆盖；二进制包附校验和，所有下载只读取本次运行的产物。普通 CI 仅只读，版本提交、标签、镜像和 Release 的写权限只授予对应发布任务。文件初始版本为 `0.0.0`，第一次选择 minor 得到 `v0.1.0`。

**文件：** `.github/workflows/code-check.yml`、`.github/workflows/release.yml`、`scripts/release-version.mjs`、`scripts/release-version.test.mjs`、`scripts/release-workflow.test.mjs`、`scripts/automation-config.test.mjs`、`package.json` 测试入口、`README.md`、`docs/DEPLOYMENT.md`、`apps/server/README.md`。

## 任务 1：发布版本与工作流契约

- [x] 为稳定版本排序、三种递增、非法选项和预发布标签编写 `node:test` 用例，运行并确认缺失实现时失败。
- [x] 编写最小版本计算脚本，使用 BigInt 避免数字版本排序和精度问题；加入仓库测试入口。
- [x] 扩展 YAML 结构测试，约束复用入口、固定检出、门禁依赖、产物传递、发布权限与手动触发，先运行确认失败。

## 任务 2：Actions 构建与发布

- [x] 为 `code-check.yml` 增加 `workflow_call` 的 `ref`、`version`；各任务使用 `inputs.ref || github.sha` 检出。
- [x] Go 门禁后将内嵌资源打成 tar 并上传；Linux 双架构任务下载、解包并编译 `CGO_ENABLED=0` 单程序，amd64 执行健康检查及管理页面冒烟，然后上传 tar.gz。
- [x] 新建手动 `Release`：prepare 要求最新默认分支，计算版本、同步 Node 与 Go 文件并原子推送版本提交和标签；checks 调用完整门禁；image 发布 GHCR；publish 核实标签、合并二进制下载并生成 SHA256SUMS.txt 后创建 Release。
- [x] 执行 `pnpm test:automation`、版本脚本测试与 actionlint；不本地编译项目或镜像。
- [x] 执行真实工作流 Bash，通过本地 GitHub CLI 替身验证首次发布、草稿续传、公开响应丢失、文件不一致、已公开缺文件和标签漂移；不连接真实账号。

## 任务 3：文档与收尾

- [x] 部署指南第 2 节改为 Actions 点击路径、首次版本选择、架构选择、下载、校验、解压和安装。
- [x] 同步 README 与服务端文档；说明 Actions 编译不等于服务器部署，云密钥继续只放服务器。
- [x] 检查 diff 与文档链接，运行 lint 和脚本测试；独立审查并修复有效问题，按工作流和部署文档分别原子提交。

**验证说明：** 本地只检查脚本、YAML、shell 和文档；Linux 编译、Docker 构建与实际 Release 上传由推送后的 Actions 验证。不得把本地配置检查描述成远端构建成功。

## 任务 4：补齐实际版本同步

- [x] 先验证失败：版本文件一致性、合法性、幂等性、文件版本领先标签与工作流契约；随后补充真实临时 Git 仓库的版本提交及原子推送验证。
- [x] 增加四份 Node 清单与 Go 应用版本，提供 `--version`；更新 prepare 并移除后置 tag 任务，保持所有构建固定版本提交。
- [x] 在 Actions 校验文件与发布版本一致，验证二进制版本；同步部署说明。
- [x] 运行脚本、lint、格式及工作流检查，独立审查。不在本地编译或实际发布；按版本能力和发布接入分别原子提交。

本地验证：自动化测试 28 项通过，ESLint、actionlint、35 个 Bash 步骤语法、变更 Go 文件的 gofmt 和 `git diff --check` 通过；离线 `--frozen-lockfile --lockfile-only --ignore-scripts` 校验通过，锁文件无需变更。独立审查没有新增发现。Go 测试、二进制 `--version` 冒烟和实际发布留给 Actions；本地没有编译项目或镜像，也没有推送远端。
