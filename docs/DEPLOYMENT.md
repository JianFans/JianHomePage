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

## 2. 在 GitHub Actions 编译并下载

不用在本地编译。代码合并到默认分支后，在 [GitHub Actions](https://github.com/JianFans/JianHomePage/actions/workflows/release.yml) 操作：

1. 左侧选择 **Release**，点 **Run workflow**，分支选默认分支，目前是 `master`。
2. `bump` 选择版本递增方式。第一次建议选 `minor`，得到 `v0.1.0`；以后修 Bug 选 `patch`，增加功能选 `minor`，有不兼容改动选 `major`。
3. 点绿色 **Run workflow**，等待这次运行全部成功。它先更新项目文件中的版本号，自动创建版本提交和 `vX.Y.Z` 标签，一起推送到默认分支；随后针对这个提交检查代码、测试、覆盖率、浏览器操作、数据库和容器，最后编译并发布文件。
4. 打开 [Releases](https://github.com/JianFans/JianHomePage/releases)，选择刚发布的版本，在 Assets 中下载服务器对应的 `.tar.gz` 和 `SHA256SUMS.txt`。

| 服务器上 `uname -m` 的结果 | 下载文件，以下以 0.1.0 为例 |
| --- | --- |
| `x86_64` | `yujian-server-0.1.0-linux-amd64.tar.gz` |
| `aarch64` 或 `arm64` | `yujian-server-0.1.0-linux-arm64.tar.gz` |

把这两个文件上传到服务器的一个空目录，在该目录执行：

```bash
sha256sum --check --ignore-missing SHA256SUMS.txt
```

只有下载的压缩包显示 `OK` 才继续。校验文件同时列出两个架构，`--ignore-missing` 会跳过没下载的另一个架构；它不会跳过已下载文件的校验失败。

第一次安装，解压后把程序放到固定位置。下面的版本与架构要与下载文件一致：

```bash
tar -xzf yujian-server-0.1.0-linux-amd64.tar.gz
sudo install -d /opt/yujian
sudo install -m 755 yujian-server /opt/yujian/yujian-server
/opt/yujian/yujian-server --help
/opt/yujian/yujian-server --version
```

压缩包里还带有 `BUILD-INFO.txt`，记录对应的版本、代码提交和架构。`--version` 可以直接查看程序版本，不需要先填服务器配置。程序已包含 Vue 管理页面、首页生成器和浏览器资源，不用再下载其他前端文件。

**只想临时测试某次提交**：选择 **Check Code → Run workflow**；push 和 PR 也会运行它。成功后，在这次运行页面底部的 Artifacts 下载 `yujian-server-linux-amd64` 或 `yujian-server-linux-arm64`，解开下载的 ZIP 后得到 `.tar.gz`。这些临时文件保留 7 天；正式部署使用 Releases 中带版本和校验和的文件。

**使用 Docker**：同一次 Release 还会发布 `ghcr.io/jianfans/jianhomepage:v0.1.0` 和 `latest`。目前镜像只支持 Linux amd64；ARM64 使用上面的单程序包。部署时固定版本标签，不用 `latest`。第一次发布后，在 GitHub 的 Packages 页面确认镜像可见性；公开镜像可以直接拉取，私有镜像需要有读取权限的账号登录 GHCR。运行时仍使用下一节的一份配置，不用在服务器构建镜像。

Release 只允许从最新默认分支启动。普通 push、PR 和手动创建标签不会发布正式版本。仓库需要允许 Actions 机器人向默认分支提交，并创建版本标签、Release 和 GHCR 包；如果分支或标签保护规则禁止这些操作，准备阶段就会失败，需要先调整仓库规则。构建过程不需要 COS、Pages、OIDC 或数据库的生产密钥。

自动同步的文件是根目录、`apps/web`、`apps/admin`、`packages/schema` 的四份 `package.json`，以及 `apps/server/cmd/api/version.go`。版本递增以这些文件与已有稳定标签中的最高版本为准。`go.mod` 中的 Go 工具链版本和内容快照的 `schemaVersion` 是独立版本，不随程序发版修改。版本提交与标签通过一次原子推送写入；分支被并发更新或标签冲突时，两者不会只更新一个。

运行失败时，查看红色任务的日志。准备阶段成功后，即使后续检查失败，版本提交和标签也会保留，尚不会公开 Release。需要修复代码时，合并修复后重新发起 Release，使用下一个版本；如果只是后续任务的临时网络或权限问题，使用该次运行的 **Re-run failed jobs**，继续原来已固定的提交和版本，不要重跑已成功的准备阶段。如果准备阶段推送响应丢失，先检查远端提交和标签是否已经写入；已写入时重新发起下一版本，不重写原标签。上传先保存为草稿，文件完整后才公开；重跑会核对已有文件，只补传草稿中缺失的文件。已有文件内容不同，或已公开版本文件缺失时，会停止并报错，不会覆盖。不要移动已有标签或手动替换 Release 文件。

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

**改程序**：先在 Actions 发布新版本，按第 2 节下载、校验和解压。处理完活动发布任务，备份数据库和旧二进制，再停止服务、替换程序并重启：

```bash
sudo systemctl stop yujian
sudo cp -p /opt/yujian/yujian-server /opt/yujian/yujian-server.previous
sudo install -m 755 yujian-server /opt/yujian/yujian-server
sudo systemctl start yujian
sudo journalctl -u yujian -n 50 --no-pager
curl --fail http://127.0.0.1:8080/healthz
```

升级前也要阅读该版本的变更说明和迁移前置条件。健康检查失败时保留日志，停止继续发布内容；不要清空数据库。任务冻结完整二进制的 SHA-256；更换程序后不会用新逻辑悄悄重建旧任务。保留原程序用于恢复未完成的旧任务。

**提示上传结果未知**：不要重复创建发布或删除数据库记录。先在 Pages 控制台核实有没有创建部署。管理员使用 [发布恢复接口](../apps/server/README.md#内置-pages-发布与恢复)，绑定核实过的部署 ID，或确认没有创建部署并填写备注。恢复会记录操作账号。

**从旧 Node 发布程序升级**：先用旧程序处理完活动任务，再停止旧发布程序和旧 Go API、备份数据库，启动新版 Go 程序。移除旧发布容器及其网关配置，新版使用本页的 `PAGES_*` 参数。`0008_publisher_builds` 表保留，无需清空历史。

首次跨过 `0003_publish_target_freeze` 迁移的旧数据库，升级前也必须没有活动发布任务。其他前置条件见 [安全与运维](../apps/server/README.md#安全与运维)。

真实云账号、域名、COS CORS 和登录服务仍需在你的环境中联调。本地门禁不代表已经部署成功。
