# 管理端素材工作台设计规格

## 1. 背景

「遇健我」的 Go 服务已经提供素材签名上传、完成确认和删除接口，也能在本地开发模式通过 `/local-upload/*` 接收文件。管理端目前只能维护完整 JSON 快照，无法上传文件、查看服务端素材记录或把完成的素材安全写入快照。

这导致内容维护链路在素材环节中断：编辑人员需要自行计算 SHA-256、构造素材记录并手工填写稳定地址。下一阶段应先补齐素材工作台，再继续建设结构化内容表单。

## 2. 目标

编辑人员应能在管理端完成以下闭环：

1. 选择允许的图片、动图、音频或视频文件。
2. 填写来源、署名、许可和双语替代文本。
3. 在浏览器中计算 SHA-256，并在创建上传前完成 MIME、扩展名和大小校验。
4. 使用 Go 服务返回的签名 URL 直接上传文件，再调用完成确认接口。
5. 查看最近创建的素材，并按状态、类型或文件名筛选当前结果。
6. 把已完成素材转换为 canonical 快照素材记录，插入当前合法草稿并继续编辑。
7. 在本地开发环境使用现有上传适配器完成同一套流程，不依赖真实云账号。

## 3. 方案选择

### 3.1 采用方案：服务端分页素材列表与管理端直传工作台

Go 服务新增只读素材列表接口。接口返回服务端已经持久化的稳定素材记录，并使用不透明游标分页。管理端负责文件预检、摘要计算、签名 URL 直传、完成确认和快照记录转换。

该方案复用现有对象存储抽象，不把腾讯云 COS SDK 类型传入领域层或管理端。素材上传完成后，快照只保存服务端返回的稳定 `src`，不保存上传 URL、临时请求头或对象存储内部地址。

### 3.2 未采用方案

- **只增加上传按钮：** 实现较快，但页面刷新后无法找回已上传素材，也无法复用历史素材。
- **先建设完整结构化内容编辑器：** 最终体验更好，但素材仍需手工准备，无法形成真实内容维护闭环。
- **由 Go 服务代理上传文件：** 可以规避浏览器跨域配置，但会让大文件经过应用服务，占用服务器带宽和内存，也偏离现有签名直传架构。

## 4. 范围

### 4.1 包含

- `GET /api/v1/assets` 素材列表接口。
- 按 `status` 筛选，默认返回未删除素材；支持 `pending`、`ready`、`deleted`。
- 使用 `limit` 和不透明 `cursor` 的稳定分页，按 `createdAt DESC, id DESC` 排序。
- 内存仓储与 PostgreSQL 仓储使用一致的筛选、排序和分页语义。
- 管理端素材 API 类型、创建上传、对象存储 `PUT`、完成确认和列表读取。
- 浏览器端 SHA-256、文件约束校验和稳定错误代码。
- 素材上传表单、进度状态、最近素材列表和客户端文件名／类型筛选。
- 把 `ready` 素材插入当前快照的 `assets` 数组。
- 中文和英文界面、键盘操作、触控目标和 `aria-live` 状态。
- Go、TypeScript、Vue 和 OpenAPI 契约测试。

### 4.2 不包含

- 真实 OIDC 登录页面或 Token 刷新。
- 腾讯云 COS、EdgeOne 控制台配置和真实云冒烟。
- 分片上传、断点续传、并发多文件上传或后台上传队列。
- 图片裁剪、转码、压缩、音视频波形或缩略图生成。
- 自动把素材绑定到音乐、视频、活动等业务记录。
- 修改 canonical 内容 Schema。
- 在管理端永久保存文件、Token、签名 URL 或上传请求头。

## 5. 服务端契约

### 5.1 素材列表接口

新增：

```http
GET /api/v1/assets?status=ready&limit=50&cursor=<opaque>
Authorization: Bearer <session-token>
```

规则如下：

- 使用现有 `create_asset` 权限，允许 `editor` 和 `admin` 读取素材库。
- `status` 可省略；省略时返回 `pending` 和 `ready`，不返回 `deleted`。
- `limit` 默认 50，最小 1，最大 100。
- `cursor` 是服务端生成的 Base64URL 字符串，编码上一页末项的 `createdAt` 和 `id`。
- 游标无效返回 `400 invalid_request`，不得回退到第一页。
- 响应包含 `items` 和可选的 `nextCursor`。
- 列表项继续复用现有 `Asset` 表示，不返回 `blob_key`、创建者身份或对象存储内部信息。

响应示例：

```json
{
  "items": [
    {
      "id": "asset_example",
      "src": "https://media.yujian.me/assets/asset_example/source.webp",
      "status": "ready",
      "metadata": {
        "fileName": "cover.webp",
        "contentType": "image/webp",
        "declaredSize": 120000,
        "checksum": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        "width": 1200,
        "height": 1200
      },
      "rights": {
        "source": { "zh-CN": "官方授权素材" }
      },
      "createdAt": "2026-10-09T00:00:00Z"
    }
  ],
  "nextCursor": "eyJjcmVhdGVkQXQiOi4uLn0"
}
```

### 5.2 仓储查询

素材服务新增查询值对象：

```go
type ListQuery struct {
    Status          []domain.AssetStatus
    BeforeCreatedAt *time.Time
    BeforeID        string
    Limit           int
}

type ListPage struct {
    Items      []domain.AssetRecord
    NextCursor string
}
```

仓储读取 `limit + 1` 条记录判断是否存在下一页。相同时间戳使用 `id` 作为次级排序键，避免分页重复或遗漏。列表是只读操作，不写审计日志。

## 6. 管理端上传模型

新增独立的纯函数与组合式函数边界：

```ts
export interface AssetUploadDraft {
  file: File
  sourceZhCN: string
  sourceEn: string
  credit: string
  license: string
  altZhCN: string
  altEn: string
}

export type AssetUploadStage =
  | 'idle'
  | 'hashing'
  | 'creating'
  | 'uploading'
  | 'completing'
  | 'succeeded'
  | 'failed'
```

上传顺序固定为：

```text
选择文件
  -> 本地预检
  -> Web Crypto 计算 SHA-256
  -> POST /api/v1/assets/uploads
  -> PUT 签名上传地址并原样携带服务端请求头
  -> POST /api/v1/assets/{assetId}/complete
  -> 将完成素材加入当前素材页
  -> 用户确认后插入快照
```

上传 `PUT` 不携带管理 API 的 Bearer Token，也不自动附加管理 API 的 `Content-Type`。客户端只发送签名响应要求的请求头，并确保实际文件类型与声明一致。

## 7. 文件约束与快照转换

管理端前置规则与服务端保持一致：

| MIME | 扩展名 | 最大大小 | 快照 `kind` |
| --- | --- | ---: | --- |
| `image/webp` | `.webp` | 20 MiB | `image` |
| `image/gif` | `.gif` | 20 MiB | `gif` |
| `audio/mpeg` | `.mp3` | 100 MiB | `audio` |
| `audio/wav` | `.wav` | 100 MiB | `audio` |
| `video/mp4` | `.mp4` | 2 GiB | `video` |

插入快照时生成以下字段：

- `id`：服务端素材 ID。
- `kind`：由 MIME 映射。
- `src`：服务端持久化的稳定公开地址。
- `mimeType`、`byteSize`、`checksum`：来自已确认的服务端 metadata。
- `width`、`height`、`durationSeconds`：服务端完成确认后存在时写入。
- `alt`：来自上传表单，`zh-CN` 必填，`en` 可选。
- `rights`：与创建上传时提交的权利信息一致。

只有当前编辑器文本可以解析为对象、包含 `assets` 数组且不存在相同素材 ID 时才能插入。插入后重新运行现有 canonical Schema 与语义诊断，不自动保存草稿。

## 8. 页面体验

素材工作台作为现有内容编辑区下方的独立面板，不改动公开首页，也不引入新的全局导航层级。

面板分为两列：

1. **上传区：** 文件选择、来源、署名、许可、双语替代文本，以及一个主上传按钮。上传阶段使用单一进度状态展示，不弹出遮挡页面的模态框。
2. **素材区：** 显示最近素材的紧凑网格。图片和 GIF 使用稳定比例预览；音频和视频使用类型图标与文件信息，避免自动加载或自动播放大媒体。

每项素材提供“插入快照”按钮。已存在于当前快照的素材显示不可操作状态。筛选控件只作用于已加载页面，服务端状态筛选会重新请求第一页；“加载更多”追加下一页。

移动端改为单列。操作按钮触控高度不小于 44 px。上传状态使用 `aria-live="polite"`，错误文本使用 `role="alert"`。在 `prefers-reduced-motion` 下不显示循环进度动画。

## 9. 错误处理与安全

- 文件预检失败时不计算摘要、不创建服务端素材记录。
- 摘要计算失败、签名过期、对象存储上传失败和完成确认失败使用不同稳定错误代码。
- 对象存储上传失败后保留当前文件和表单内容，允许重新创建新的签名上传；不复用可能已过期的 URL。
- 完成确认失败时保留素材 ID，并允许重试完成确认，不重复上传已经成功写入的对象。
- 页面卸载后不继续把异步结果写入已销毁的 Vue 状态。
- Token 只用于管理 API；不得发送到签名上传域名。
- 列表和快照预览不渲染任意 HTML、SVG、脚本或富文本。
- 删除素材继续只允许 `admin`，本轮不在管理端提供删除按钮，避免误删历史引用素材。

## 10. 测试策略

- 素材服务测试覆盖默认状态、显式状态、稳定排序、下一页游标和无效游标。
- 内存与 PostgreSQL 仓储测试覆盖相同时间戳下的 ID 次序和分页边界。
- HTTP 测试覆盖鉴权、查询参数、响应结构和错误映射。
- OpenAPI 契约测试覆盖新路径、查询参数和分页响应。
- 管理端纯函数测试覆盖 MIME、扩展名、大小、SHA-256、metadata 转换和快照去重插入。
- API 客户端测试确认 Bearer Token 只发送给管理 API，签名上传只携带服务端指定请求头。
- 组合式函数测试覆盖完整阶段流转、上传失败、完成重试、分页追加和组件销毁。
- 页面测试覆盖双语文案、键盘入口、移动布局结构、状态播报和插入按钮门禁。
- 管理端 E2E 使用本地 Go 服务和 `/local-upload/*` 完成一个小型 WebP 上传、完成确认、插入快照和保存草稿闭环。

## 11. 验收标准

- 编辑人员无需命令行即可上传一个受支持文件并获得 `ready` 素材。
- 管理端不接触对象存储密钥，也不会把 Bearer Token 发送到签名上传地址。
- 刷新管理端后可以通过素材列表重新找到已创建素材。
- 素材分页在相同创建时间下不重复、不遗漏。
- 已完成素材可以插入当前快照，插入后的快照继续通过 canonical 校验。
- 本地开发上传闭环可以自动测试，不要求真实 COS 或 EdgeOne。
- 所有受影响的前端、Go、OpenAPI、覆盖率和 E2E 门禁通过。
