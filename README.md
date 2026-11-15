# 租房版图 · Rental Atlas

科技黑与荧光蓝风格的全屏地图找房应用。首页只有中国地图和省级名称；逐级点击，最终在区县的高德街道地图上看到房源光点。

## 邮箱账号

左上角账号入口支持邮箱注册、密码登录与退出。允许 QQ、163、126、Gmail、Outlook、Hotmail、iCloud 的精确邮箱域名，后端校验邮箱格式与白名单；注册必须验证邮箱收到的验证码。验证码 2 分钟内有效、使用一次即失效，支持重发冷却、错误次数与请求限流。

QQ SMTP 配置使用后端专用 `.env.server.local`，自动加载，绝不使用 `VITE_` 前缀。配置邮箱与授权码后重启 Go 服务即可；SMTP 不可用时明确报错，不返回虚假验证码。密码使用 bcrypt 保存，登录使用 HttpOnly Cookie。详情见 [账号系统说明](docs/accounts.md)。

## 当前交互

1. 全国：全屏中国行政区地图，显示省名，不显示导航栏、侧栏、房源、统计或筛选器。
2. 省级：点击省份后自动将该省最大化，显示所属城市；不请求或显示房源。
3. 市级：点击城市后自动将该市最大化，显示区县；不请求或显示房源。
4. 区县：点击区县后切换至真实高德地图，自动适配区县范围，显示道路、地名、建筑与区县边界。区县外的底图被遮罩。
5. 房源以荧光光点展示，点击光点才展开详情，关闭后恢复纯地图。

右上角仅保留当前位置，例如 `中国 / 浙江省 / 杭州市 / 西湖区`，点击前级返回。全国首页不显示路径栏。直辖市直接进入区县。

全国、省、市支持以鼠标位置为中心的滚轮缩放，以及左键按住拖动；拖动不会误触下钻。每次进入下一级都会重新适配视野。区县支持滚轮缩放、左键拖动、高德底图细节按缩放加载。支持浏览器返回和带 `?region=330106` 的区县链接。

区县级左上角（账号按钮下方）提供地铁图层开关，点击后在高德街道地图上高亮当前区县内的线路与站点，放大后显示站名。直辖市下辖区县同样支持；全国、省、市层级隐藏开关。线路按高德真实站点坐标连接，明确标注为“站点连线示意”。杭州内置线路快照，其他已收录城市按需加载。详情及更新方法见 [地铁图层说明](docs/metro-layer.md)。

区县底部可切换全部、整租、转租、合租，只请求房源数据并更新光点。房源详情、类型筛选、列表和详情重试均独立于底图，保留地图位置、缩放及地铁图层。实现与验证见 [房源独立加载说明](docs/rental-loading.md)。

## 技术与数据

| 层 | 选择 |
|---|---|
| 前端 | React 19、TypeScript、Vite 7 |
| 全国 / 省 / 市 | D3 Geo + SVG，DataV.GeoAtlas 行政边界；全国使用两档简化 TopoJSON 与共享省界 |
| 区县 | 高德 Web JS API 2.0，深蓝底图、区县遮罩、道路 / 建筑 / 地名、房源标记 |
| 后端 | Go 1.24+，标准库 net/http、database/sql |
| 数据库 | 本地 SQLite；正式部署支持 PostgreSQL 17 / pgx |
| 测试 | Go testing、Playwright / Microsoft Edge |

内置 18 城市共 216 条示例房源。示例位置和室内展示图片不代表真实出租信息；详情内明确标识。示例坐标若因行政中心偏移超出区县边界，仅将其示意光点移至该区县内部，真实房源坐标不会自动移动。没有接入真实房源来源。

此前的侧栏、搜索、收藏和发布界面已从当前页面移除。原 Go 数据库与 API 保留，现有记录不会因为界面调整而删除。

## 本地运行

需要 Node.js 22.12+ 和 Go 1.24+。

```powershell
./scripts/start.ps1
```

打开 http://127.0.0.1:8080 。默认使用 `data/rental.db`，首次启动建表并写入示例。前端已经构建且不改地图配置时，可运行 `./scripts/start.ps1 -SkipBuild`。

分开开发：

```powershell
# 终端一
go run ./cmd/server

# 终端二
npm ci
npm run dev
```

开发页面：http://127.0.0.1:5173 。

## 高德地图配置

项目根目录 `.env.local` 保存高德 Web JS API 配置，该文件已排除在 Git 和 Docker 构建上下文之外。模板字段见 `.env.example`：

```dotenv
VITE_AMAP_KEY=your_web_js_api_key
VITE_AMAP_SECURITY_CODE=your_security_js_code
```

Vite 在启动 / 构建时读取配置。更改后重启开发服务或执行 `npm run build`。Web JS API 的客户端凭据会用于浏览器 SDK 请求，不能把其他服务端密钥填入此处。高德控制台的域名授权需包含实际访问地址；`localhost`、`127.0.0.1` 与正式域名应按各自使用情况配置。

高德 SDK 只在区县层级按需加载。未配置、网络失败或超过加载时间时，界面提示原因并提供重试，路径导航仍可返回市级。高德 Logo 与版权信息按 SDK 默认保留。

## PostgreSQL / Docker

已有数据库：

```powershell
$env:DATABASE_URL='postgres://atlas:YOUR_PASSWORD@127.0.0.1:5432/rental_atlas?sslmode=disable'
go run ./cmd/server
```

Docker Compose：

```powershell
$env:POSTGRES_PASSWORD='replace_with_a_strong_password'
docker compose up --build -d
```

Compose 使用 BuildKit 的构建 secret 读取 `.env.local`，构建过程中不打印凭据；高德 Web JS 客户端配置按需包含在前端产物中。应用端口仅绑定本机，数据库采用持久卷且不暴露端口。当前环境没有 Docker，尚未执行容器集成验证。外部数据库应按实际部署配置 TLS。

Go 通过进程环境读取 `ADDR`（默认 `127.0.0.1:8080`）、`DATA_DIR`（默认 `data`）、`STATIC_DIR`（默认 `dist`）、`DATABASE_URL`、`ALLOW_PUBLISH`；启动时额外读取 `.env.server.local`，不会读取 `.env`。左上角「+」提供发布入口；浏览房源无需登录，发布必须登录。对外部署需设置 `ALLOW_PUBLISH=true`。

## 验证

```powershell
go test ./...
go vet ./...
npm run build
npm run test:e2e:isolated
```

隔离端到端测试自动使用 8081 端口和独立 SQLite 目录，完成后停止测试服务，不影响体验数据。需要 Microsoft Edge；高德实景用例使用真实 SDK 和网络，而非绘制假的道路。覆盖全屏首页、逐级下钻、房源只在区县请求、路径与历史恢复、滚轮与拖动、高德光点与详情、手机尺寸以及地图失败恢复。

全国地图的卡顿原因、优化前后实测及性能复测方法见 [地图性能说明](docs/map-performance.md)。

全国地图的轻量数据已包含在源码中。更新原始全国边界后执行 `npm run maps:build` 重新生成，再运行 `npm run test:maps` 检查省份、岛屿、边界误差和体积预算。

## 数据来源与后续范围

行政边界：[DataV.GeoAtlas](https://datav.aliyun.com/portal/school/atlas/area_selector)。常用城市边界已缓存，未缓存区域按需请求。部分地区暂无子级边界，不虚构下级行政区。区县街道底图：[高德地图 Web JS API](https://lbs.amap.com/api/javascript-api-v2/summary)。室内展示图片：[Unsplash](https://unsplash.com)。

当前为本地体验版本，已实现邮箱验证账号。已支持登录发布与图片上传；发布审核、房源真实性和地址核验尚未接入。正式公开地图和数据前需确认相应授权与适用要求。初版选型记录和本次地图改版说明见 [调研文档](docs/research.md)。


## 用户发布房源

点击左上角「+」，登录后进入房源所在区县，在街道地图中点击选定位置，填写资料并上传 1–5 张 JPEG / PNG 照片（每张最多 5 MB、1200 万像素）。照片支持左右滑动；信息包括出租类型、出租身份、住宅 / 公寓类型、室厅卫、面积、月租和联系方式。确认发布成功后才生成正式光点。列表筛选、发布和详情请求不会重建高德底图。

服务端校验登录状态、同源请求、字段、照片内容及区县边界内的位置，不接受匿名发布。每个账号每小时最多发起 20 次发布请求。图片重新编码为 JPEG，去掉原文件名和 EXIF / GPS 信息；元数据与房源一同持久化。地图点位是用户自行提供，未经地址真实性核验。

用户照片位于 `DATA_DIR/runtime/uploads`，已排除在 Git、Docker 构建上下文之外。Compose 的 `auth_runtime` 持久卷同时保存照片；备份数据库时应同步备份此目录。照片和联系方式在发布后可被所有访客查看，表单有公开提示。生产入口反向代理请允许至少 26 MB 请求体，适当调高上传超时；应用默认上传读取超时为 90 秒。
