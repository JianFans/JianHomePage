# OIDC 角色映射修复计划

## 目标与边界

OIDC 令牌只需包含至少一个本站认可角色；忽略其他字符串角色，继续拒绝缺失、格式错误或完全不匹配的角色声明。内部 RBAC 和持久会话仍使用严格的标准角色校验。

通过 `OIDC_ROLE_EDITOR`、`OIDC_ROLE_REVIEWER`、`OIDC_ROLE_PUBLISHER`、`OIDC_ROLE_ADMIN` 分别配置外部角色名，默认沿用 `editor`、`reviewer`、`publisher`、`admin`。自定义值替换默认名，区分大小写；重复映射及含空白、控制字符的名称在启动时拒绝。浏览器 ID token 与 API Bearer token 共用映射。

## 执行顺序

1. [x] 补充签名 JWT、授权码回调与配置回归测试：覆盖混合角色、自定义名称、去重、无匹配、非法声明和非法映射。
2. [x] 实现配置默认值和校验，在两条认证路径接入映射；不改变内部角色与权限定义。
3. [x] 同步 `.env.example`、部署指南和服务 README；完成相关测试与独立审查后提交。不在本地构建发布二进制。

## 验证

- 测试先于实现：先运行相关测试，确认旧实现无法满足混合角色行为。
- `go test ./internal/auth ./internal/config ./cmd/api`，必要时先生成缺失的内嵌资源。
- `go test -race ./internal/auth ./internal/config`（工具链支持时）。
- `gofmt -l`、`git diff --check`；检查环境变量与两份文档一致。
- 真实身份服务的回调与云端发布不在本次本地验证范围内。

## 验证结果

- 旧实现已复现混合角色认证失败，修复后对应测试通过。
- `go test ./... -count=1` 与 `go vet ./...` 通过。
- Go 格式检查、diff 检查以及 29 个环境变量的模板完整性检查通过。
- 独立审查未发现需要修复的问题；竞态测试因未启用 CGO 未运行成功，真实 Casdoor 回调仍待部署验证。
