# Go 内容与发布服务实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 构建可供「遇健我」管理端使用的 Go 内容与发布服务，完成 OIDC/RBAC、草稿与审核、乐观锁、素材适配、不可变快照、发布幂等、历史回滚和审计日志。

**架构：** 服务以完整 `YujianContentSnapshot` 作为版本单元，PostgreSQL 保存版本、素材记录、发布任务、当前发布指针和审计日志。业务层只依赖 `Repository`、`BlobStore`、`BuildTrigger`、`Notifier` 和 `IdentityProvider` 接口；公开站继续只读取构建时快照，不依赖本服务在线。

**技术栈：** Go 1.25、标准库 `net/http`、pgx v5、PostgreSQL、go-oidc v3、jsonschema v6、go-jsonschema、oapi-codegen v2、OpenAPI 3.1。

---

## 范围边界

- 本计划实现服务商无关的发布编排和可测试 fake，不接入腾讯云专有 SDK。
- EdgeOne Pages 的具体触发、查询、回调和回源配置由 `2026-08-29-edgeone-publishing.md` 实现。
- 管理端页面由 `2026-08-29-admin-console.md` 实现。
- PostgreSQL 集成测试必须使用显式 `TEST_DATABASE_URL`；没有数据库时单元测试仍可运行，但不能把集成验收标记完成。

## 文件结构

```text
apps/server/go.mod                                  Go module 与固定依赖
apps/server/go.sum                                  依赖校验值
apps/server/generate.go                             Schema 与 OpenAPI 生成入口
apps/server/cmd/api/main.go                         进程启动、依赖装配与优雅退出
apps/server/cmd/schema-sync/main.go                 同步规范 JSON Schema 到可嵌入目录
apps/server/internal/config/config.go               环境变量配置与生产安全门禁
apps/server/internal/config/config_test.go          配置正反例
apps/server/internal/snapshot/generated.go          由 JSON Schema 生成，禁止手改
apps/server/internal/contract/schema.json           由规范 Schema 同步，禁止手改
apps/server/internal/contract/validator.go           嵌入并校验内容快照
apps/server/internal/contract/validator_test.go      fixture、错误路径与生成类型测试
apps/server/internal/domain/domain.go               状态、角色、版本、素材、发布任务
apps/server/internal/ports/ports.go                 可迁移服务商接口
apps/server/internal/store/postgres/migrations/*.sql 数据库迁移
apps/server/internal/store/postgres/migrate.go      嵌入式迁移执行器
apps/server/internal/store/postgres/repository.go   pgx 仓储与事务
apps/server/internal/store/postgres/repository_integration_test.go
apps/server/internal/auth/principal.go              Principal 与上下文
apps/server/internal/auth/rbac.go                   角色权限判断
apps/server/internal/auth/oidc.go                   OIDC Bearer Token 校验
apps/server/internal/auth/middleware.go             鉴权中间件与开发身份门禁
apps/server/internal/auth/auth_test.go               OIDC fake、RBAC 与生产禁用测试
apps/server/internal/content/service.go             草稿、审核和状态流转
apps/server/internal/content/service_test.go        乐观锁、角色与流转测试
apps/server/internal/assets/service.go              签名上传、完成校验与删除
apps/server/internal/assets/service_test.go         文件约束与 BlobStore fake 测试
apps/server/internal/publish/service.go             快照、幂等发布、状态与回滚
apps/server/internal/publish/service_test.go        重复请求、失败保留和回滚测试
apps/server/internal/httpapi/openapi.yaml            OpenAPI 3.1 规范副本
apps/server/internal/httpapi/oapi-codegen.yaml       生成配置
apps/server/internal/httpapi/generated.go            OpenAPI 生成类型与 std-http 接口
apps/server/internal/httpapi/handler.go              HTTP 适配、错误映射与请求 ID
apps/server/internal/httpapi/handler_test.go          认证、ETag、错误码和路由测试
apps/server/internal/testkit/fakes.go                单元测试复用 fake
apps/server/README.md                                本地配置、迁移、测试和运行说明
packages/schema/openapi/admin.yaml                   管理 API 的规范源文件
```

## 任务 1：初始化 Go module、配置与进程壳

**文件：**

- 创建：`apps/server/go.mod`
- 创建：`apps/server/internal/config/config.go`
- 创建：`apps/server/internal/config/config_test.go`
- 创建：`apps/server/cmd/api/main.go`

- [ ] **步骤 1：初始化 module 并固定依赖**

运行：

```bash
cd apps/server
go mod init yujian.me/server
go get github.com/jackc/pgx/v5@v5.8.0
go get github.com/coreos/go-oidc/v3@v3.20.0
go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.2
go get -tool github.com/atombender/go-jsonschema@v0.23.1
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
```

预期：`go.mod` 使用 `go 1.25`，所有版本固定且 `go mod tidy` 退出码为 `0`。

- [x] **步骤 2：编写失败的配置测试**

```go
func TestLoadRejectsDevelopmentIdentityInProduction(t *testing.T) {
    env := map[string]string{
        "APP_ENV": "production",
        "DATABASE_URL": "postgres://example",
        "OIDC_ISSUER": "https://id.example.com",
        "OIDC_AUDIENCE": "yujian-admin",
        "ALLOW_DEV_IDENTITY": "true",
    }
    _, err := Load(func(key string) string { return env[key] })
    if !errors.Is(err, ErrUnsafeDevelopmentIdentity) {
        t.Fatalf("expected unsafe identity error, got %v", err)
    }
}
```

- [x] **步骤 3：运行测试确认配置加载器缺失**

运行：`go test ./internal/config -run TestLoadRejectsDevelopmentIdentityInProduction`

预期：FAIL，提示 `Load` 或 `ErrUnsafeDevelopmentIdentity` 未定义。

- [x] **步骤 4：实现配置和健康检查**

`Config` 必须包含 `Environment`、`Address`、`DatabaseURL`、`OIDCIssuer`、`OIDCAudience`、`AllowDevIdentity` 和 `ShutdownTimeout`。生产环境缺少数据库/OIDC 配置或启用开发身份时直接返回错误。`cmd/api/main.go` 使用 `signal.NotifyContext`、`http.Server` 和超时上下文优雅退出；`GET /healthz` 返回 `200 application/json` 与 `{"status":"ok"}`。

- [x] **步骤 5：验证配置与进程壳**

运行：

```bash
go test ./internal/config
go test ./cmd/api
go vet ./...
```

预期：全部通过，无 `go vet` 警告。

- [ ] **步骤 6：提交**

```bash
git add apps/server
git commit -m "chore(服务端): 初始化 Go 内容服务"
```

## 任务 2：生成 Go 快照类型并嵌入契约校验

**文件：**

- 创建：`apps/server/generate.go`
- 创建：`apps/server/cmd/schema-sync/main.go`
- 创建：`apps/server/internal/snapshot/generated.go`
- 创建：`apps/server/internal/contract/schema.json`
- 创建：`apps/server/internal/contract/validator.go`
- 创建：`apps/server/internal/contract/validator_test.go`

- [ ] **步骤 1：编写失败的契约测试**

测试读取 `../../../content/fixtures/homepage.json`，断言：

```go
func TestValidatorAcceptsFixtureAndGeneratedType(t *testing.T) {
    raw := readFixture(t)
    if err := NewValidator().Validate(raw); err != nil {
        t.Fatalf("fixture should be valid: %v", err)
    }
    var value snapshot.YujianContentSnapshot
    if err := json.Unmarshal(raw, &value); err != nil {
        t.Fatalf("generated type should decode fixture: %v", err)
    }
    if value.SchemaVersion != "1.0.0" {
        t.Fatalf("unexpected schema version %q", value.SchemaVersion)
    }
}

func TestValidatorReportsJSONPointer(t *testing.T) {
    raw := bytes.Replace(readFixture(t), []byte(`"schemaVersion": "1.0.0"`), []byte(`"schemaVersion": "2.0.0"`), 1)
    err := NewValidator().Validate(raw)
    if err == nil || !strings.Contains(err.Error(), "/schemaVersion") {
        t.Fatalf("expected schema path, got %v", err)
    }
}
```

- [ ] **步骤 2：运行测试确认生成类型和校验器缺失**

运行：`go test ./internal/contract`

预期：FAIL，提示 `snapshot` 包或 `NewValidator` 不存在。

- [ ] **步骤 3：实现跨平台 Schema 同步和代码生成**

`cmd/schema-sync` 从仓库根 `packages/schema/schema/content-snapshot.schema.json` 复制字节到 `apps/server/internal/contract/schema.json`。`generate.go` 使用以下指令：

```go
//go:generate go run ./cmd/schema-sync
//go:generate go tool go-jsonschema --schema-package=https://yujian.me/schemas/content-snapshot/1.0.0=yujian.me/server/internal/snapshot --schema-output=https://yujian.me/schemas/content-snapshot/1.0.0=internal/snapshot/generated.go ../../packages/schema/schema/content-snapshot.schema.json
package server
```

运行：`go generate ./...`。

- [ ] **步骤 4：实现嵌入式验证器**

`validator.go` 使用 `//go:embed schema.json`，在构造时编译一次 Draft 2020-12 Schema；`Validate([]byte)` 先使用 `json.Unmarshal` 拒绝语法错误，再返回包含 JSON Pointer 的验证错误。禁止在请求期间读取仓库文件。

- [ ] **步骤 5：验证生成文件无漂移**

运行：

```bash
go generate ./...
go test ./internal/contract
git diff --exit-code apps/server/internal/contract/schema.json apps/server/internal/snapshot/generated.go
```

预期：测试通过，第二次生成没有差异。

- [ ] **步骤 6：提交**

```bash
git add apps/server packages/schema/schema/content-snapshot.schema.json
git commit -m "feat(内容契约): 生成 Go 快照类型与校验器"
```

## 任务 3：定义领域模型、权限和服务商端口

**文件：**

- 创建：`apps/server/internal/domain/domain.go`
- 创建：`apps/server/internal/ports/ports.go`
- 创建：`apps/server/internal/auth/rbac.go`
- 创建：`apps/server/internal/auth/auth_test.go`

- [x] **步骤 1：编写失败的 RBAC 与状态测试**

```go
func TestCanEnforcesRoleBoundaries(t *testing.T) {
    cases := []struct {
        role auth.Role
        permission auth.Permission
        allowed bool
    }{
        {auth.RoleEditor, auth.PermissionEditDraft, true},
        {auth.RoleEditor, auth.PermissionPublish, false},
        {auth.RoleReviewer, auth.PermissionReview, true},
        {auth.RolePublisher, auth.PermissionRollback, true},
        {auth.RoleAdmin, auth.PermissionManageUsers, true},
    }
    for _, test := range cases {
        if got := auth.Can(test.role, test.permission); got != test.allowed {
            t.Fatalf("role %s permission %s: got %v", test.role, test.permission, got)
        }
    }
}
```

- [x] **步骤 2：运行测试确认领域常量缺失**

运行：`go test ./internal/auth -run TestCanEnforcesRoleBoundaries`

预期：FAIL，提示角色或权限未定义。

- [x] **步骤 3：实现最小领域模型**

领域类型必须包含：

```go
type ContentStatus string
const (
    StatusDraft ContentStatus = "draft"
    StatusInReview ContentStatus = "in_review"
    StatusPublished ContentStatus = "published"
    StatusArchived ContentStatus = "archived"
)

type ContentVersion struct {
    ID string
    Status ContentStatus
    Revision int64
    Snapshot json.RawMessage
    Checksum string
    ReviewApproved bool
    CreatedBy string
    UpdatedBy string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

同时定义 `AssetRecord`、`PublishJob`、`PublishPointer`、`AuditEntry`、`Principal` 和明确的 sentinel errors：`ErrNotFound`、`ErrConflict`、`ErrForbidden`、`ErrInvalidTransition`、`ErrDuplicateRequest`。

- [x] **步骤 4：定义端口接口**

`ports.go` 定义：

```go
type BlobStore interface {
    CreateUpload(context.Context, UploadRequest) (SignedUpload, error)
    Stat(context.Context, string) (BlobMetadata, error)
    Put(context.Context, string, io.Reader, BlobMetadata) error
    Delete(context.Context, string) error
    SignedReadURL(context.Context, string, time.Duration) (string, error)
}

type BuildTrigger interface {
    Trigger(context.Context, BuildRequest) (BuildRun, error)
    Status(context.Context, string) (BuildRun, error)
}

type Notifier interface {
    PublishCompleted(context.Context, PublishResult) error
}

type IdentityProvider interface {
    Authenticate(context.Context, string) (domain.Principal, error)
}
```

仓储接口按业务用例命名，不暴露 pgx 类型。

- [x] **步骤 5：运行领域和权限测试**

运行：`go test ./internal/auth ./internal/domain ./internal/ports`

预期：全部通过。

- [ ] **步骤 6：提交**

```bash
git add apps/server/internal/domain apps/server/internal/ports apps/server/internal/auth
git commit -m "feat(领域模型): 定义内容状态权限与服务商端口"
```

## 任务 4：建立 PostgreSQL 迁移与事务仓储

**文件：**

- 创建：`apps/server/internal/store/postgres/migrations/0001_initial.sql`
- 创建：`apps/server/internal/store/postgres/migrate.go`
- 创建：`apps/server/internal/store/postgres/repository.go`
- 创建：`apps/server/internal/store/postgres/repository_integration_test.go`

- [ ] **步骤 1：编写需要真实 PostgreSQL 的失败测试**

集成测试读取 `TEST_DATABASE_URL`，为空时调用 `t.Skip` 并明确说明未执行集成验收；有值时创建独立 schema，执行迁移并验证：

1. 创建草稿后 revision 为 `1`。
2. 使用错误 revision 更新返回 `domain.ErrConflict`。
3. 相同 `idempotency_key` 只能创建 1 个发布任务。
4. 发布指针与审计日志在同一事务中提交。

- [ ] **步骤 2：运行测试确认迁移器与仓储缺失**

运行：`go test ./internal/store/postgres -run TestRepository -count=1`

预期：FAIL，提示 `Open`、`Migrate` 或仓储方法未定义；若没有 `TEST_DATABASE_URL`，输出明确的 SKIP。

- [ ] **步骤 3：创建初始迁移**

`0001_initial.sql` 创建以下表和约束：

```sql
CREATE TABLE content_versions (
  id text PRIMARY KEY,
  status text NOT NULL CHECK (status IN ('draft','in_review','published','archived')),
  revision bigint NOT NULL CHECK (revision > 0),
  snapshot jsonb NOT NULL,
  checksum text NOT NULL,
  review_approved boolean NOT NULL DEFAULT false,
  created_by text NOT NULL,
  updated_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE assets (
  id text PRIMARY KEY,
  blob_key text NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('pending','ready','deleted')),
  metadata jsonb NOT NULL,
  rights jsonb NOT NULL,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  deleted_at timestamptz
);

CREATE TABLE publish_jobs (
  id text PRIMARY KEY,
  idempotency_key text NOT NULL UNIQUE,
  version_id text NOT NULL REFERENCES content_versions(id),
  snapshot_key text NOT NULL,
  snapshot_checksum text NOT NULL,
  build_id text,
  status text NOT NULL CHECK (status IN ('pending','building','succeeded','failed')),
  error_message text,
  requested_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE publish_pointer (
  slot text PRIMARY KEY,
  version_id text NOT NULL REFERENCES content_versions(id),
  snapshot_key text NOT NULL,
  snapshot_checksum text NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE TABLE audit_log (
  id bigserial PRIMARY KEY,
  actor_sub text NOT NULL,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  metadata jsonb NOT NULL,
  created_at timestamptz NOT NULL
);
```

- [ ] **步骤 4：实现嵌入迁移和 pgx 仓储**

迁移器用 `//go:embed migrations/*.sql`，在 `pg_advisory_xact_lock` 下按文件名顺序执行。仓储更新使用 `WHERE id=$1 AND revision=$2`，受影响行数为 `0` 时区分不存在与 revision 冲突。发布成功事务必须同时更新旧版本状态、当前版本状态、发布指针和审计日志。

- [ ] **步骤 5：执行 PostgreSQL 集成测试**

运行：

```bash
$env:TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/yujian_test?sslmode=disable"
go test ./internal/store/postgres -count=1
```

预期：全部通过且测试 schema 被清理。没有可用 PostgreSQL 时，此步骤保持未勾选。

- [ ] **步骤 6：提交**

```bash
git add apps/server/internal/store
git commit -m "feat(数据层): 添加 PostgreSQL 版本与发布仓储"
```

## 任务 5：实现 OIDC 鉴权、开发身份门禁与 RBAC 中间件

**文件：**

- 创建：`apps/server/internal/auth/principal.go`
- 创建：`apps/server/internal/auth/oidc.go`
- 创建：`apps/server/internal/auth/middleware.go`
- 修改：`apps/server/internal/auth/auth_test.go`

- [x] **步骤 1：编写失败的鉴权测试**

测试覆盖：Bearer Token 缺失返回 `401`；无权限角色返回 `403`；`editor` 可编辑但不可发布；`admin` 拥有全部权限；生产配置无法构造开发身份提供器；OIDC claims 中 `roles` 只接受已知角色。

- [x] **步骤 2：运行测试确认中间件缺失**

运行：`go test ./internal/auth`

预期：FAIL，提示 `Middleware`、`OIDCProvider` 或 Principal 上下文函数缺失。

- [ ] **步骤 3：实现 OIDC Provider 和中间件**

`OIDCProvider` 使用 issuer discovery 和 audience verifier。只读取 `sub`、`email`、`name`、`roles`；不信任客户端自报角色。中间件解析单个 `Authorization: Bearer <token>`，把 Principal 写入私有 context key，并用统一 JSON 错误体返回 `401/403`。

- [x] **步骤 4：实现显式开发身份**

只有 `APP_ENV != production && ALLOW_DEV_IDENTITY=true` 时，才允许读取 `X-Dev-Subject` 和 `X-Dev-Roles`。未启用时忽略这些 header；生产环境构造开发 Provider 直接返回 `config.ErrUnsafeDevelopmentIdentity`。

- [x] **步骤 5：验证安全边界**

运行：

```bash
go test ./internal/auth
go vet ./internal/auth/...
```

预期：全部通过。

- [ ] **步骤 6：提交**

```bash
git add apps/server/internal/auth apps/server/internal/config
git commit -m "feat(鉴权): 添加 OIDC 与 RBAC 中间件"
```

## 任务 6：实现草稿、乐观锁与审核流转

**文件：**

- 创建：`apps/server/internal/content/service.go`
- 创建：`apps/server/internal/content/service_test.go`
- 创建：`apps/server/internal/testkit/fakes.go`

- [x] **步骤 1：编写失败的内容服务测试**

使用内存 Repository fake 覆盖：

```go
func TestUpdateDraftUsesExpectedRevision(t *testing.T) {
    service := newContentService(t)
    draft := service.CreateDraft(ctx, editor, validFixture(t))
    _, err := service.UpdateDraft(ctx, editor, draft.ID, draft.Revision+1, validFixture(t))
    if !errors.Is(err, domain.ErrConflict) {
        t.Fatalf("expected conflict, got %v", err)
    }
}

func TestReviewRequiresReviewerAndValidTransition(t *testing.T) {
    service := newContentService(t)
    draft := service.CreateDraft(ctx, editor, validFixture(t))
    submitted := service.SubmitReview(ctx, editor, draft.ID, draft.Revision)
    if _, err := service.ApproveReview(ctx, editor, submitted.ID, submitted.Revision); !errors.Is(err, domain.ErrForbidden) {
        t.Fatalf("editor must not approve: %v", err)
    }
}
```

- [x] **步骤 2：运行测试确认服务缺失**

运行：`go test ./internal/content`

预期：FAIL，提示 `Service` 方法未定义。

- [x] **步骤 3：实现草稿与审核状态机**

规则固定为：

- `CreateDraft`：校验 Schema，状态 `draft`，revision `1`。
- `UpdateDraft`：仅 `draft`，需要 `editor`，校验 expected revision，成功后 revision `+1`。
- `SubmitReview`：`draft → in_review`，清除旧审核结论。
- `ApproveReview`：仅 `reviewer/admin`，状态保持 `in_review`，写入 `review_approved=true`。
- `RejectReview`：`in_review → draft`，revision `+1`，保留审计原因。
- 发布只接受 `in_review && review_approved=true`。

每次变更都在同一 Repository 事务中写审计记录。

- [x] **步骤 4：验证全部状态与错误场景**

运行：`go test ./internal/content -count=1`

预期：全部通过，并覆盖无权限、错误 revision、非法状态和无效快照。

- [ ] **步骤 5：提交**

```bash
git add apps/server/internal/content apps/server/internal/testkit
git commit -m "feat(内容管理): 添加草稿审核与乐观锁"
```

## 任务 7：实现素材签名上传与可迁移 BlobStore

**文件：**

- 创建：`apps/server/internal/assets/service.go`
- 创建：`apps/server/internal/assets/service_test.go`

- [x] **步骤 1：编写失败的素材测试**

测试覆盖：允许的图片、视频和音频 MIME；扩展名与 MIME 不匹配被拒绝；超过类型上限被拒绝；生成稳定逻辑 ID 和服务商无关 blob key；完成上传时校验实际大小、checksum 和媒体元数据；删除写审计且调用 BlobStore。

- [x] **步骤 2：运行测试确认素材服务缺失**

运行：`go test ./internal/assets`

预期：FAIL，提示 `CreateUpload` 或 `CompleteUpload` 未定义。

- [x] **步骤 3：实现素材约束和两阶段上传**

固定约束：图片 20 MiB、音频 100 MiB、视频 2 GiB；允许 `image/avif`、`image/webp`、`image/jpeg`、`audio/mpeg`、`audio/wav`、`video/mp4`、`video/webm`。`CreateUpload` 只返回 15 分钟签名 URL；`CompleteUpload` 调用 `BlobStore.Stat` 校验实际数据后才把记录从 `pending` 改为 `ready`。

业务表只保存 `blob_key`、checksum、尺寸、时长、rights 和 attribution，不保存腾讯云 bucket、region 或专有 URL。

- [x] **步骤 4：验证素材服务**

运行：`go test ./internal/assets -count=1`

预期：全部通过。

- [ ] **步骤 5：提交**

```bash
git add apps/server/internal/assets apps/server/internal/ports
git commit -m "feat(素材): 添加签名上传与资源适配层"
```

## 任务 8：实现不可变快照、发布幂等和回滚

**文件：**

- 创建：`apps/server/internal/publish/service.go`
- 创建：`apps/server/internal/publish/service_test.go`

- [x] **步骤 1：编写失败的发布编排测试**

测试必须覆盖：

1. 同一 `idempotency_key` 重复调用返回同一逻辑发布任务，不重复写 Blob 或触发构建。
2. 发布前再次执行 Schema、引用、HTTPS、素材 ready 状态和国际化回退校验。
3. 快照 key 包含 release ID 与 SHA-256，写入后不可覆盖。
4. BuildTrigger 失败时当前发布指针保持上一成功版本。
5. 成功后旧 published 版本归档，指针原子切换。
6. 回滚重新触发历史快照构建，不修改历史快照内容。

- [x] **步骤 2：运行测试确认发布服务缺失**

运行：`go test ./internal/publish`

预期：FAIL，提示 `Publish`、`RefreshStatus` 或 `Rollback` 未定义。

- [x] **步骤 3：实现确定性快照**

对规范化 JSON 使用稳定字段顺序编码，计算 SHA-256；key 格式为 `snapshots/<releaseId>/<checksum>.json`。`BlobStore.Put` 前先检查同 key 是否存在且 checksum 相同；不同内容禁止覆盖。

- [x] **步骤 4：实现发布状态机**

`Publish` 顺序：读取并锁定审核版本 → 校验 → 查询/创建幂等任务 → 写快照 → 调用 BuildTrigger → 保存 `building`。`RefreshStatus` 只在构建成功后事务切换发布指针；失败只更新任务错误。`Rollback` 创建新的发布任务，目标指向历史快照。

Notifier 失败只写告警，不回滚已成功发布；通知中不得包含 Token、签名 URL 或数据库连接信息。

- [x] **步骤 5：验证发布编排**

运行：`go test ./internal/publish -count=1`

预期：全部通过，fake 断言调用次数精确。

- [ ] **步骤 6：提交**

```bash
git add apps/server/internal/publish apps/server/internal/testkit
git commit -m "feat(发布): 添加幂等构建与历史回滚"
```

## 任务 9：定义 OpenAPI 并实现标准库 HTTP API

**文件：**

- 创建：`packages/schema/openapi/admin.yaml`
- 创建：`apps/server/internal/httpapi/openapi.yaml`
- 创建：`apps/server/internal/httpapi/oapi-codegen.yaml`
- 创建：`apps/server/internal/httpapi/generated.go`
- 创建：`apps/server/internal/httpapi/handler.go`
- 创建：`apps/server/internal/httpapi/handler_test.go`
- 修改：`apps/server/generate.go`

- [ ] **步骤 1：先编写失败的 HTTP 行为测试**

测试使用 `httptest` 覆盖：

- `GET /healthz` 无鉴权返回 `200`。
- `POST /api/v1/versions` editor 返回 `201`、`ETag: "1"`。
- `PUT /api/v1/versions/{id}` 缺少 `If-Match` 返回 `428`，revision 冲突返回 `409`。
- 审核、发布、回滚端点按角色返回 `403`。
- `Idempotency-Key` 缺失返回 `400`。
- 未知 JSON 字段和超大请求体返回 `400/413`。
- 所有错误体为 `{ "code": "...", "message": "...", "requestId": "..." }`。

- [ ] **步骤 2：运行测试确认路由缺失**

运行：`go test ./internal/httpapi`

预期：FAIL，提示 Router 或生成接口未定义。

- [x] **步骤 3：编写 OpenAPI 3.1 规范**

规范定义以下路径和 operation ID：

```text
GET    /healthz                         getHealth
POST   /api/v1/versions                 createVersion
GET    /api/v1/versions/{versionId}     getVersion
PUT    /api/v1/versions/{versionId}     updateVersion
POST   /api/v1/versions/{versionId}/review submitReview
POST   /api/v1/versions/{versionId}/approve approveReview
POST   /api/v1/versions/{versionId}/reject rejectReview
POST   /api/v1/assets/uploads           createAssetUpload
POST   /api/v1/assets/{assetId}/complete completeAssetUpload
DELETE /api/v1/assets/{assetId}         deleteAsset
POST   /api/v1/publishes                createPublish
GET    /api/v1/publishes/{publishId}    getPublish
POST   /api/v1/publishes/{publishId}/refresh refreshPublish
POST   /api/v1/rollbacks                createRollback
```

Bearer 安全方案应用于 `/api/v1/*`。版本更新使用 `If-Match`，发布和回滚使用必填 `Idempotency-Key`。

- [ ] **步骤 4：生成 Go API 类型**

`oapi-codegen.yaml` 生成 `models,std-http-server,spec`，package 为 `httpapi`。`generate.go` 先同步 `packages/schema/openapi/admin.yaml` 到 `internal/httpapi/openapi.yaml`，再运行：

```bash
go tool oapi-codegen --config internal/httpapi/oapi-codegen.yaml internal/httpapi/openapi.yaml
```

- [ ] **步骤 5：实现 Handler 与错误映射**

Handler 只负责请求解析、Principal、ETag/If-Match、调用应用服务和映射状态码。正文使用 `http.MaxBytesReader`；JSON decoder 启用 `DisallowUnknownFields`；外部错误信息不包含 SQL、Token、签名 URL 或内部堆栈。

- [ ] **步骤 6：验证规范、生成文件和 HTTP 测试**

运行：

```bash
go generate ./...
go test ./internal/httpapi -count=1
git diff --exit-code apps/server/internal/httpapi/generated.go apps/server/internal/httpapi/openapi.yaml
```

预期：全部通过，第二次生成无漂移。

- [ ] **步骤 7：提交**

```bash
git add packages/schema/openapi apps/server/internal/httpapi apps/server/generate.go
git commit -m "feat(管理接口): 添加 OpenAPI 与内容发布端点"
```

## 任务 10：装配服务、运行全量验证并记录运维基线

**文件：**

- 修改：`apps/server/cmd/api/main.go`
- 创建：`apps/server/README.md`
- 修改：`README.md`
- 修改：`package.json`

- [ ] **步骤 1：编写失败的应用装配测试**

创建 `cmd/api/main_test.go`，通过依赖注入的 fake Repository、IdentityProvider、BlobStore 和 BuildTrigger 启动 Handler，验证健康检查、创建草稿、提交审核和发起发布的最小闭环。

- [ ] **步骤 2：实现依赖装配与优雅关闭**

启动顺序固定为：加载配置 → 建立 pgx pool → 执行迁移 → 构造 validator/repository/providers/services → 构造 Router → 启动 HTTP server。关闭顺序为停止接收请求 → 等待 in-flight 请求 → 关闭 pgx pool。日志使用 `slog` JSON handler，并在每条请求日志包含 request ID、actor subject、route、status 和 duration。

- [ ] **步骤 3：补充根命令**

根 `package.json` 新增：

```json
{
  "scripts": {
    "test:go": "cd apps/server && go test ./...",
    "generate:go": "cd apps/server && go generate ./...",
    "verify:go": "cd apps/server && go generate ./... && go test ./... && go vet ./..."
  }
}
```

Windows 与 Linux 均必须通过；若 pnpm 在当前 shell 不支持 `cd &&`，改为 `pnpm --dir apps/server exec` 不适用于 Go，因此根验证文档同时保留直接进入目录的命令。

- [ ] **步骤 4：编写服务端 README**

记录全部环境变量、开发身份限制、数据库迁移、单元/集成测试命令、请求示例、发布状态机、快照 key 规则和故障恢复。不得写入真实密钥或服务商专有配置。

- [ ] **步骤 5：执行全量验证**

运行：

```bash
cd apps/server
go generate ./...
go test ./...
go vet ./...
go test -race ./...
```

若 `TEST_DATABASE_URL` 可用，再运行：

```bash
go test ./internal/store/postgres -count=1
```

返回仓库根后运行：

```bash
pnpm verify
git diff --check
git status --short
```

预期：Go 与公开站验证全部通过；生成文件无漂移；工作树只包含本计划变更。

- [ ] **步骤 6：提交**

```bash
git add apps/server packages/schema/openapi README.md package.json docs/superpowers/plans/2026-08-29-go-content-service.md
git commit -m "feat(服务端): 完成内容管理与发布编排"
```

## 完成标准

- OIDC、开发身份门禁和 `editor/reviewer/publisher/admin` 权限有自动化测试。
- 草稿、审核、乐观锁、素材校验、发布幂等、失败保留上一版本和回滚均有单元测试。
- PostgreSQL 事务与唯一约束通过真实数据库集成测试。
- JSON Schema 与 OpenAPI 生成文件可重复生成且无漂移。
- Go 服务不可用时，既有 Nuxt 静态产物仍能独立提供。
- EdgeOne、对象存储和通知提供商没有泄漏进业务模型。

## 当前执行状态（2026-08-30）

- 已完成并验证：领域模型、RBAC 与开发身份门禁、标准库 OIDC Provider、内容状态流转、素材约束、不可变快照、发布幂等、EdgeOne HTTP 触发适配器、HTTP 路由、内存开发仓储、嵌套 JSON Schema 与跨记录引用校验、PostgreSQL 迁移器和可替换 SQL 仓储接口。
- 已完成并验证：`apps/server/cmd/api` 可用内存依赖跑通「创建草稿 → 提交审核 → 通过 → 发起发布」闭环；`go test ./...` 与 `go vet ./...` 通过。
- 尚未完成：真实 PostgreSQL 连接池/驱动装配、OIDC 生产密钥与 issuer 配置、OpenAPI 代码生成、EdgeOne 账号侧真实构建回调和 PostgreSQL 集成测试。
- 阻塞原因：固定第三方依赖下载和 Docker/PostgreSQL 接入需要本机审批；在获得明确批准前不得绕过或重复尝试。
