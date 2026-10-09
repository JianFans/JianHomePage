# 遇健我管理端

这是「遇健我」的 Vue 管理端，用于维护版本化内容快照、提交审核、查看 EdgeOne 构建状态和发起回滚。管理端默认以 SPA 方式构建，不参与公开首页的静态首屏。

## 快照维护

内容工作台直接复用 `@yujian/schema` 的 canonical Schema 和语义校验规则，在编辑阶段提供以下能力：

- 即时显示 JSON 语法、必填字段、字段类型、跨记录引用和素材类型问题。
- 每条诊断包含 JSON Pointer 路径，并区分 Schema 问题与语义问题。
- 合法快照显示板块、作品、曲目、影像、现场、片段、素材和试听数量。
- 从本地导入不超过 2 MiB 的 `.json` 文件。
- 把当前合法快照导出为以 `releaseId` 命名的格式化 JSON 文件。

本地导入只读取文件内容，不上传原文件。导出只包含已经通过校验的内容快照，不包含 API 地址、会话 Token、版本状态或发布任务。客户端校验用于提前反馈，不能替代 Go 服务端的最终契约、权限和乐观锁校验。

## 素材工作台

素材工作台把文件预检、SHA-256、签名直传、完成确认和快照插入串成一个可恢复流程。上传表单要求填写中文来源和中文替代文本；英文来源、英文替代文本、署名和许可信息可选。素材完成确认并进入 `ready` 状态后，才能插入当前 JSON 快照。

支持的文件约束如下：

| 类型 | MIME | 扩展名 | 大小上限 |
| --- | --- | --- | --- |
| 图片 | `image/webp` | `.webp` | 20 MiB |
| 动图 | `image/gif` | `.gif` | 20 MiB |
| MP3 音频 | `audio/mpeg` | `.mp3` | 100 MiB |
| WAV 音频 | `audio/wav` | `.wav` | 100 MiB |
| MP4 视频 | `video/mp4` | `.mp4` | 2 GiB |

浏览器以 4 MiB 分块计算 SHA-256，不会把整个大文件一次性读入内存。文件仍需完整经过浏览器和对象存储网络链路；编辑人员应保持页面打开，直到界面显示上传完成。

素材上传流程如下：

1. 客户端校验 MIME、扩展名、大小和必填权利字段。
2. 客户端分块计算 SHA-256，并携带 Bearer Token 请求 `POST /api/v1/assets/uploads`。
3. 客户端向服务端返回的签名 URL 执行 `PUT`，只发送响应中声明的上传请求头。
4. 签名上传成功后，客户端携带 Bearer Token 请求 `POST /api/v1/assets/{assetId}/complete`。
5. 服务端复核 MIME、实际大小和 SHA-256，素材进入 `ready` 状态。
6. 编辑人员补全卡片中的替代文本并插入快照，再单独保存草稿。

签名 URL 属于对象存储临时能力。管理 API 的 Bearer Token 不会发送到该 URL；客户端也不会把签名 URL 写入快照。快照只保存服务端返回的稳定 `src`。若完成确认暂时失败，界面只重试确认步骤，不会重复上传已经写入的对象。

上传成功不等于内容已经发布。素材插入只更新当前编辑器文本，后续仍需保存草稿、提交审核、通过审核并完成发布流程。

## 本地运行

```bash
pnpm --filter @yujian/admin dev
```

默认 API 地址为 `http://127.0.0.1:8080`，也可以通过 `ADMIN_API_BASE_URL` 覆盖：

```bash
ADMIN_API_BASE_URL=https://api.yujian.me pnpm --filter @yujian/admin dev
```

Go 开发服务的签名 URL 固定指向 `http://127.0.0.1:8080/local-upload/*`，因此本地素材闭环应让服务监听默认端口。开发身份使用 `X-Dev-Subject` 和 `X-Dev-Roles`，管理端不会在浏览器中伪造这两个请求头；浏览器联调应使用真实 OIDC Token，或由受控的本地反向代理注入开发身份。自动化测试则分别覆盖 Go HTTP 上传闭环和管理端 API、快照插入逻辑。

## 使用流程

1. 填写 Go 服务地址和当前会话的 OIDC Bearer Token。
2. 输入版本 ID 并载入现有版本，或导入、编辑本地 JSON 创建草稿。
3. 根据右侧摘要和诊断修正内容；只有完整有效的快照才能保存。
4. 保存时由服务端返回新的 `ETag` 和 revision；客户端不会覆盖本地未确认的版本。
5. 提交审核、通过或退回，所有状态转换由 Go 服务的 RBAC 再次校验。
6. 发布和回滚自动生成 `Idempotency-Key`，可重复点击而不会创建多个逻辑任务。
7. 发布任务进入构建阶段后，使用「刷新状态」查询 EdgeOne 适配器返回的构建状态。

## 安全边界

- Token 只保存在当前页面的内存状态，不写入 `localStorage`、URL 或静态产物。
- Bearer Token 只发送给配置的管理 API；对象存储签名 `PUT` 不携带 `Authorization`。
- 生产 COS 必须允许管理端 Origin 的 `PUT`、`HEAD`，并放行签名响应要求的 `Content-Type`、`X-Amz-Checksum-Sha256` 等请求头。
- 管理端不解析或渲染任意 HTML、CSS 和脚本；快照以 JSON 编辑和预览为主。
- 生产 API 应使用 HTTPS、短时 OIDC Token、CSP 和独立管理域名。
- 公开首页不依赖管理端；管理端不可用不会影响已发布静态站。

## 验证命令

```bash
pnpm --filter @yujian/admin test
pnpm --filter @yujian/admin typecheck
pnpm --filter @yujian/admin build
```
