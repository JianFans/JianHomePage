# 部署「遇健我」

服务器只运行一个 `yujian-server` 程序。管理页面、管理 API、生成首页、上传腾讯云和检查发布结果都包含在这个 Go 二进制里。服务器不用装 Node.js、pnpm 或 Go，也不用另外放一份管理页面。

访客访问 `https://yujian.me`，看到的是 EdgeOne Pages 上已经生成的网页。管理员访问自己的后台域名，例如 `https://admin.yujian.me`。后台关机不会影响已经发布的首页。

## 1. 先准备外部服务

| 服务 | 用途 | 需要准备什么 |
| --- | --- | --- |
| Linux 服务器 | 运行这个 Go 程序 | 能访问下面的数据库、账号系统和腾讯云 |
| PostgreSQL | 保存内容、会话和发布进度 | 数据库连接串；直接连接或会话级连接池，不能使用事务级连接池 |
| 账号系统 | 提供账号密码登录 | 支持 OIDC 的登录服务，例如已有的 Keycloak；管理客户端和带 `admin` 角色的账号 |
| 腾讯云 COS | 保存素材和发布快照 | 桶、地域、访问密钥、稳定素材域名 |
| EdgeOne Pages | 托管公开首页 | **Direct Upload** 项目、项目 ID、Pages API Token、项目区域 |
| HTTPS 入口 | 给后台提供 HTTPS | 后台域名、证书和反向代理；可以经 EdgeOne 回源到服务器 |

在 Pages 给项目绑定 `yujian.me`。素材域名可以用 `media.yujian.me`，经 EdgeOne 指向 COS；以后换存储服务时保留这个域名。

账号系统中的管理客户端配置：

- 登录回调：`https://admin.yujian.me/api/v1/auth/callback`。
- 登录完成后回到：`https://admin.yujian.me/`。
- ID token 的 `aud` 包含管理客户端 ID，`roles` 包含 `admin`。完整字段见 [服务端说明](../apps/server/README.md#浏览器登录与会话)。

## 2. 在开发电脑编译一次

这里才需要 Node.js、pnpm 和 Go，版本以 `package.json` 与 `apps/server/go.mod` 为准。

Linux 或 macOS，在仓库根目录执行：

```bash
pnpm install --frozen-lockfile
GOOS=linux GOARCH=amd64 pnpm build:server
```

Windows PowerShell：

```powershell
pnpm install --frozen-lockfile
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
pnpm build:server
```

产物是 `dist/yujian-server`。ARM64 服务器把 `GOARCH` 改为 `arm64`。命令会先编译 Vue 页面和资源，再把它们嵌入 Go 程序；不需要给构建机器配置云密钥。

把文件上传到服务器的 `/opt/yujian/yujian-server`：

```bash
chmod 755 /opt/yujian/yujian-server
/opt/yujian/yujian-server --help
```

也可以在构建机器执行 `docker build -t yujian-server:local .`，把镜像传到服务器。运行层只有 Go 程序及系统证书，仍使用下面的一份配置。

## 3. 填一份配置

在服务器建立 `/etc/yujian/server.env`。每行是 `变量名=实际值`，不写 `export`。只给运行程序的用户读取权限，不把文件提交到仓库。

| 变量 | 填什么 |
| --- | --- |
| `APP_ENV` | `production` |
| `HTTP_ADDRESS` | `127.0.0.1:8080`，由本机代理转发 |
| `DATABASE_URL` | PostgreSQL 连接串 |
| `OIDC_ISSUER` | 账号系统的发行地址 |
| `OIDC_AUDIENCE` | API token 中本服务使用的 audience |
| `ADMIN_OIDC_CLIENT_ID` | 管理登录客户端 ID |
| `ADMIN_OIDC_CLIENT_SECRET` | 账号系统要求客户端密钥时填写 |
| `ADMIN_OIDC_REDIRECT_URL` | `https://admin.yujian.me/api/v1/auth/callback` |
| `ADMIN_RETURN_URL` | `https://admin.yujian.me/` |
| `ADMIN_ALLOWED_ORIGINS` | `https://admin.yujian.me`，不加末尾斜杠 |
| `ADMIN_SESSION_KEY` | 用 `openssl rand -base64 32` 生成的值 |
| `S3_ENDPOINT` | COS 地域端点，例如 `https://cos.ap-singapore.myqcloud.com` |
| `S3_REGION` | 桶的地域，例如 `ap-singapore` |
| `S3_BUCKET` | 完整桶名，含账号后缀 |
| `S3_ACCESS_KEY_ID` | 访问这个桶的 SecretId |
| `S3_SECRET_ACCESS_KEY` | 对应 SecretKey |
| `MEDIA_PUBLIC_BASE_URL` | `https://media.yujian.me` |
| `PAGES_API_TOKEN` | 腾讯云 Pages 的 API Token |
| `PAGES_PROJECT_ID` | Direct Upload 项目的 ID |
| `PAGES_REGION` | `china` 或 `global`，与项目区域一致 |
| `PAGES_PUBLIC_URL` | `https://yujian.me`，与快照的 canonical URL 一致 |

COS 临时密钥还要填 `S3_SESSION_TOKEN`，并在到期前更新。密钥只存在服务器配置中，不会传到前端。

COS 上传 CORS 需要允许后台的精确 Origin、`PUT` 方法和签名上传请求头，详见 [服务端说明](../apps/server/README.md)。不要公开存放 `snapshots/*`；公开读取只需要素材。

## 4. 启动并接通后台域名

建立专用系统用户 `yujian`，让它能执行程序、读取配置。程序不需要写代码目录。

创建 `/etc/systemd/system/yujian.service`：

```ini
[Unit]
Description=Yujian management server
Wants=network-online.target
After=network-online.target

[Service]
User=yujian
Group=yujian
EnvironmentFile=/etc/yujian/server.env
ExecStart=/opt/yujian/yujian-server
Restart=on-failure
RestartSec=5
TimeoutStopSec=20
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now yujian
sudo journalctl -u yujian -n 50 --no-pager
curl --fail http://127.0.0.1:8080/healthz
```

数据库表会在启动时自动迁移。连接失败、配置缺失或页面资源不完整时，程序拒绝启动；用日志定位原因，不要改成开发模式绕过。

将后台域名的**整个站点**转发给 `http://127.0.0.1:8080`，包括 `/`、`/preview`、`/_nuxt/*` 和 `/api/*`。在已有 HTTPS 配置的 Nginx `server` 中加入：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

如果经过 EdgeOne，不缓存后台 HTML、登录接口和管理 API；`/_nuxt/*` 哈希资源可以长期缓存。不要把后台代理规则放进公开首页的 Pages 项目。

打开 `https://admin.yujian.me/`，点登录，用账号密码进入。管理页面和 API 使用同一个域名，没有第二个发布程序地址要填。

## 5. 第一次发布首页

1. 登录后台，创建内容版本。开发快照的示例图片和音频不能直接用于生产。
2. 在素材页上传获准使用的图片、视频和音频，插入内容，确保地址是稳定 HTTPS 地址。
3. 按左侧模块编辑，检查中英文、首页顺序、链接和试听；预览桌面与手机效果。
4. 保存，提交审核，通过审核，然后点发布。
5. 等显示发布成功，再访问 `https://yujian.me`。

点发布后，Go 程序读取审核快照，在进程内生成完整 HTML、CSS、JS、robots 和 sitemap，再通过腾讯云接口上传 Pages。发布进度保存在数据库，正常重启后继续检查；访客端不会临时向后台请求内容。

首次上线还要实测：登录、素材上传、首页与媒体、语言切换、试听、发布、回滚、`/robots.txt` 和 `/sitemap.xml`。缓存规则见 [EdgeOne 说明](../apps/web/EDGEONE.md)。本地测试不能替代真实云检查。

## 6. 更新和恢复

**改内容**：后台编辑、保存、审核、发布，不用重新编译。需要退回之前的内容时，在发布操作台选择历史版本回滚。

**改程序**：先处理完活动发布任务，备份数据库和旧二进制，编译新程序，再替换文件并重启。任务冻结完整二进制的 SHA-256；更换程序后不会用新逻辑悄悄重建旧任务。保留原程序用于恢复未完成的旧任务。

**提示上传结果未知**：不要重复创建发布或删除数据库记录。先在 Pages 控制台核实有没有创建部署。管理员使用 [发布恢复接口](../apps/server/README.md#内置-pages-发布与恢复)，绑定核实过的部署 ID，或确认没有创建部署并填写备注。恢复会记录操作账号。

**从旧 Node 发布程序升级**：先用旧程序处理完活动任务，再停止旧发布程序和旧 Go API、备份数据库，启动新版 Go 程序。移除旧发布容器及其网关配置，新版使用本页的 `PAGES_*` 参数。`0008_publisher_builds` 表保留，无需清空历史。

首次跨过 `0003_publish_target_freeze` 迁移的旧数据库，升级前也必须没有活动发布任务。其他前置条件见 [安全与运维](../apps/server/README.md#安全与运维)。

真实云账号、域名、COS CORS 和登录服务仍需在你的环境中联调。本地门禁不代表已经部署成功。
