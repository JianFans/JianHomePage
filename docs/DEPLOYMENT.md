# 把「遇健我」部署到腾讯云

部署好以后，你在后台编辑内容、保存、审核、点发布，网站就会更新。服务器会自己生成网页并上传到 EdgeOne Pages，不需要每次手工打包。

第一次仍需要你准备服务器、数据库、文件存储和登录服务。目前仓库没有一键开通这些云服务的脚本。

## 一、先分清哪些东西放在哪里

| 东西 | 放在哪里 | 用来干什么 |
| --- | --- | --- |
| 公开首页 | EdgeOne Pages | 给访客看，地址是 `https://yujian.me` |
| 管理页面 | 自己的服务器，通过 EdgeOne 访问 | 登录后编辑内容，建议地址是 `https://admin.yujian.me` |
| Go 后端 | 自己的服务器，Docker 容器 | 保存内容、处理登录、审核和发布请求，建议地址是 `https://api.yujian.me` |
| 发布程序 | 同一台服务器，另一个 Docker 容器 | 生成首页，调用腾讯云接口把文件上传到 Pages |
| PostgreSQL 数据库 | 云数据库或自己安装的数据库 | 保存内容版本、登录记录和发布记录 |
| 图片、音乐、视频 | 腾讯云 COS，通过 EdgeOne 访问 | 建议统一使用 `https://media.yujian.me`，以后搬文件也保留这个地址 |
| 登录服务 | 支持 OIDC 的账号系统 | 提供账号密码登录，例如自建 Keycloak |

除 `yujian.me` 外，上面的子域名是建议用法，可以换成自己的。本文按这些地址举例；更换时必须同步修改登录回调、后台配置和域名设置。

日常发布过程只有这一条：后台点发布 → 服务器生成网页 → 上传 Pages → 访客看到新首页。

## 二、在云控制台准备好这些信息

1. **服务器**：准备 Linux 服务器，安装 Docker、Git 和 Nginx。服务器要能访问数据库、COS 和 Pages。管理页面打包还需要 Node.js 和 pnpm，版本见根 `package.json`。
2. **数据库**：创建 PostgreSQL 数据库和账号，记下连接地址。Go 后端账号需要创建和修改本项目的数据表；首次启动会自动建表。发布程序必须连接同一数据库、同一组表，不能经过会在请求之间切换连接的「事务池」。
3. **COS**：创建桶，记下地域、完整桶名和访问密钥。桶名使用控制台显示的完整名称，包括账号后缀。后端需要上传、读取并检查素材和 `snapshots/` 下的内容文件；发布程序只需要读取 `snapshots/` 下的文件。不要公开这个目录，也不要直接把整个桶改成公开读。
4. **Pages**：创建类型为 **Direct Upload（直接上传）** 的项目，记下项目 ID 和 **Pages API Token**。这里不选 Git 自动构建。账号站点对应 `PAGES_REGION`：中国站填 `china`，国际站填 `global`；它和 COS 的地域是两回事。
5. **域名**：公开首页绑定到 Pages；后台和 API 通过 EdgeOne 转发到服务器；媒体域名通过 EdgeOne 读取 COS 的 `assets/` 文件。启用 HTTPS，按控制台提示完成 DNS 设置及所需备案等条件。COS 桶保持私有时，还要在 EdgeOne 配置访问私有桶所需的授权。

注意三种密钥不要混用：COS 密钥用于读写文件，Pages API Token 用于上传网站，后端和发布程序之间还要单独生成一个随机口令。

COS 的浏览器跨域设置（CORS）填：来源 `https://admin.yujian.me`，允许 `PUT`、`HEAD`，允许请求头 `Content-Type`、`X-Amz-Checksum-Sha256`。这是让浏览器能直接上传素材；只配置 API 的跨域设置还不够。

## 三、先配置账号密码登录

本项目不会自己创建账号和保存密码。点后台登录时会跳转到你配置的账号系统，登录成功后再回到后台。

在账号系统里创建一个给本项目使用的 OIDC 应用，启用授权码登录和 S256 PKCE，使用 RS256 签名，并设置：

| 设置 | 填什么 |
| --- | --- |
| 登录回调地址 | `https://api.yujian.me/api/v1/auth/callback` |
| 登录后的后台地址 | `https://admin.yujian.me/` |
| 用户权限 | 登录返回的 ID token 顶层包含 `roles`；第一个管理员可设为 `["admin"]` |

记下发行方地址（issuer）、应用 ID（client ID），以及该应用需要时的 client secret。`roles` 不能只藏在 Keycloak 默认的嵌套角色字段里，需要映射到顶层。

`admin` 可以编辑、审核和发布。以后给其他人分配权限，可以使用 `editor`（编辑）、`reviewer`（审核）、`publisher`（发布）。

## 四、下载代码，打包两个服务器程序

以下 shell 命令在 Linux 服务器上执行。先选定一个测试已通过的代码提交；首次部署使用已经合入这些功能的 `master`。不要在发布进行中切换代码。

```bash
git clone https://github.com/JianFans/JianHomePage.git
cd JianHomePage
git checkout master
git pull --ff-only
git status --short
git rev-parse HEAD
```

`git status --short` 应没有输出。记下最后一条命令输出的提交编号，用它标记镜像，方便以后找回旧版。

```bash
deploy_revision=$(git rev-parse HEAD)
docker build --tag "yujian-server:$deploy_revision" .
docker build --file apps/publisher/Dockerfile \
  --build-arg "CODE_REVISION=$deploy_revision" \
  --tag "yujian-publisher:$deploy_revision" .
```

两个命令都应成功结束。发布镜像里已经装好了生成网站需要的工具，启动后不用再装 Node.js 或 pnpm。

## 五、填写两个配置文件

在服务器上建立 `/etc/yujian/server.env` 和 `/etc/yujian/publisher.env`。每行写 `变量名=实际值`，不写 `export`。这些文件包含密码，只允许部署账号读取，不要放进 Git 仓库。

### Go 后端：server.env

| 变量名 | 填什么 |
| --- | --- |
| `APP_ENV` | `production` |
| `HTTP_ADDRESS` | `0.0.0.0:8080` |
| `DATABASE_URL` | PostgreSQL 连接串；使用数据库提供商要求的 TLS 设置，密码中的特殊字符需要 URL 编码 |
| `OIDC_ISSUER` | 账号系统给出的完整 issuer 地址 |
| `OIDC_AUDIENCE` | 账号系统给后端访问令牌配置的 audience；这是服务校验令牌用途的标识，不能随便猜 |
| `ADMIN_OIDC_CLIENT_ID` | 第三步创建的应用 ID |
| `ADMIN_OIDC_CLIENT_SECRET` | 应用需要 client secret 时填写；不需要时不写这一行 |
| `ADMIN_OIDC_REDIRECT_URL` | `https://api.yujian.me/api/v1/auth/callback` |
| `ADMIN_RETURN_URL` | `https://admin.yujian.me/` |
| `ADMIN_ALLOWED_ORIGINS` | `https://admin.yujian.me` |
| `ADMIN_SESSION_KEY` | 运行 `openssl rand -base64 32` 生成的值，保存好，重启后沿用 |
| `S3_ENDPOINT` | COS 地域地址，例如 `https://cos.ap-singapore.myqcloud.com`，按桶的实际地域填写 |
| `S3_REGION` | COS 地域，例如 `ap-singapore` |
| `S3_BUCKET` | COS 完整桶名 |
| `S3_ACCESS_KEY_ID` | 后端使用的 COS SecretId |
| `S3_SECRET_ACCESS_KEY` | 对应的 COS SecretKey |
| `MEDIA_PUBLIC_BASE_URL` | `https://media.yujian.me` |
| `EDGEONE_TRIGGER_URL` | 服务器给发布程序提供的受限 HTTPS 地址，以 `/builds` 结尾，见下一步 |
| `EDGEONE_STATUS_URL` | 与上一项完全相同 |
| `EDGEONE_TOKEN` | 另运行一次 `openssl rand -hex 32` 生成的随机口令 |

这里的两个 `EDGEONE_*_URL` 指向**你自己的发布程序**。腾讯云的 Pages Token 填在下面的发布程序配置里。

### 发布程序：publisher.env

| 变量名 | 填什么 |
| --- | --- |
| `PUBLISHER_DATABASE_URL` | 同一 PostgreSQL 数据库和同一组表的连接串；可以使用另建的数据库账号 |
| `PUBLISHER_TOKEN` | 与 `server.env` 中的 `EDGEONE_TOKEN` 完全相同 |
| `PAGES_API_TOKEN` | Pages 控制台生成的 API Token |
| `PAGES_PROJECT_ID` | Direct Upload 项目 ID |
| `PAGES_REGION` | `china` 或 `global`，按第二步的 Pages 账号站点填写 |
| `PAGES_PUBLIC_URL` | `https://yujian.me/` |
| `S3_ENDPOINT`、`S3_REGION`、`S3_BUCKET` | 与后端相同，每个变量各写一行 |
| `S3_ACCESS_KEY_ID`、`S3_SECRET_ACCESS_KEY` | 发布程序使用的 COS 读取密钥，每个变量各写一行 |

`PUBLISHER_CODE_REVISION` 已写在镜像里，不要另填一个值覆盖它。若 COS 使用临时密钥，两个程序各自还需配置对应的 `S3_SESSION_TOKEN`，并在到期前更换。

## 六、启动服务器程序，并接通 HTTPS

在同一 shell、仓库根目录执行，保持第四步的 `deploy_revision` 变量；换了终端就先重新运行 `deploy_revision=$(git rev-parse HEAD)`。

```bash
docker run --detach --name yujian-server --restart unless-stopped \
  --env-file /etc/yujian/server.env \
  --publish 127.0.0.1:8080:8080 "yujian-server:$deploy_revision"
curl --fail http://127.0.0.1:8080/healthz
```

先启动 Go 后端，因为它会自动建立发布程序需要的表。健康检查成功后再启动发布程序：

```bash
docker run --detach --name yujian-publisher --restart unless-stopped \
  --env-file /etc/yujian/publisher.env \
  --publish 127.0.0.1:8091:8091 "yujian-publisher:$deploy_revision"
curl --fail http://127.0.0.1:8091/healthz
```

若容器反复退出，先看 `docker logs yujian-server` 或 `docker logs yujian-publisher`。通常是配置缺失、数据库连不上或登录配置不完整；不要改成开发模式绕过错误。

在 Nginx 配置以下转发，并为外部使用的地址配置有效的 HTTPS 证书：

| 请求地址 | 服务器上转发到 | 要点 |
| --- | --- | --- |
| `https://api.yujian.me/api/*` | `http://127.0.0.1:8080`，保留完整路径 | 不缓存；保留 Cookie、`Authorization`、`Origin`、`X-CSRF-Token`、`If-Match`、`Idempotency-Key` 和预检请求 |
| 自选的受限 HTTPS 地址的 `/builds` 及其子路径 | `http://127.0.0.1:8091`，保留完整路径 | 只允许 Go 后端和运维访问，不对浏览器开放；不缓存；正文上限 16 KiB；保留 `Authorization`、`Idempotency-Key` |

第二行就是前面 `EDGEONE_TRIGGER_URL` 和 `EDGEONE_STATUS_URL` 要填的地址。可以使用只有服务器能访问的域名；不要把它混进对外的 `/api/*`。即使两个容器在同一台机器，当前生产配置也要求这个地址使用 HTTPS。

EdgeOne 再把 API 域名转发到这台服务器的 Nginx。8080、8091 仅绑定本机，不直接开放公网。不要给两个发布容器挂载同一个网页生成目录。

## 七、把管理页面放到服务器上

管理页面也需要单独打包；它不在 Go 镜像里，也不会随首页内容发布而更新。

在已经安装清单要求的 Node.js 和 pnpm 的构建机器上，从同一份代码的仓库根目录执行：

```bash
pnpm install --frozen-lockfile
ADMIN_API_BASE_URL=https://api.yujian.me pnpm --filter @yujian/admin generate
```

把 `apps/admin/.output/public` **里面的全部文件**放到服务器的管理站点目录，例如 `/var/www/yujian-admin`。Nginx 用这个目录提供 `admin.yujian.me`，再通过 EdgeOne 访问。不要上传源码、`.env` 或整个仓库。

管理页面的 API 地址是在打包时写进去的。以后换 API 地址，要重新打包和替换这些文件，只改服务器环境变量不会改变已经生成的页面。

打开 `https://admin.yujian.me`，点击登录。成功后应回到后台，能看到左侧内容导航和顶部版本栏。如果页面说登录不可用，检查 Go 后端的 `ADMIN_*` 配置；登录后没权限，检查第三步的 `roles`。

## 八、第一次发布首页

1. 数据库刚建好时没有内容版本。先在后台的高级工具导入一份完整内容 JSON，再保存为第一份草稿。现阶段没有从完全空白逐项创建整站的向导。
2. 可以用 `content/fixtures/homepage.json` 的副本了解格式，但必须替换其中的示例内容和样例媒体引用。示例文件不能直接发布，发布程序会拒绝已知样例媒体。
3. 在素材库上传真实图片、音乐或视频，填好来源和使用权信息。上传完成后插入当前内容，再到各模块选择这些素材，保存草稿。
4. 检查网站地址为 `https://yujian.me`，预览手机和桌面页面。
5. 在版本记录模块提交审核、通过审核，再点发布。可以由有 `admin` 权限的账号完成首次操作。
6. 等任务显示成功。发布程序会生成首页并调用 Pages 接口上传；通常不用手动刷新，后台也提供「刷新状态」。

在 Pages 中绑定 `yujian.me` 并关闭 SPA fallback（找不到文件时返回首页的功能）。首次发布成功后，在浏览器实际打开首页、图片、试听，以及 `https://yujian.me/robots.txt` 和 `https://yujian.me/sitemap.xml`。确认内容正确、文件能访问，再认为首次部署完成。

首页和文件的缓存设置见 [EdgeOne 设置说明](../apps/web/EDGEONE.md#缓存建议)。不要缓存管理 API 或发布程序的响应。

## 九、以后怎么更新，出问题怎么退回

- **改内容**：从历史版本复制新草稿，编辑、保存、审核、发布，不用重启服务器。
- **退回旧内容**：在版本记录里选择要恢复的已发布版本，发起回滚。程序会用旧内容重新生成首页，再上传一次。
- **升级程序**：先暂停新的发布和回滚，确认后台和发布程序都没有未结束的任务，再备份数据库、打包新镜像、替换容器和管理页面。保留旧镜像。已有数据库的特别注意事项见 [服务端升级说明](../apps/server/README.md#安全与运维)。
- **提示上传结果未知**：先去 Pages 控制台确认有没有创建部署，不要反复点发布或删数据库记录。核实后按 [发布恢复步骤](../apps/publisher/README.md#超时恢复与升级) 处理。

两处 `/healthz` 成功只表示服务能响应及其各自检查通过，不表示腾讯云上传、登录和素材访问都已接通。首次登录、上传、发布和回滚仍需在你自己的云环境各做一次。

完整参数需要查阅时，再看 [Go 后端](../apps/server/README.md)、[发布程序](../apps/publisher/README.md) 和 [管理页面](../apps/admin/README.md)。
