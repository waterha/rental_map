# 租房版图 · 项目介绍视频

主题：**直观看见，你想住的位置附近有什么房源。**

40 秒横屏功能介绍，1920 × 1080、30 fps，科技黑与荧光蓝风格。本版重新编排为完整 40 秒时间轴，保留开头项目介绍、GitHub 地址和 Star 提示。HTML/CSS/JS 创建画面和动画，Edge/Chromium 在指定时间逐帧渲染，FFmpeg 编码为 H.264 + AAC 的 MP4。配乐为有间隔的原创短拨弦旋律和转场提示音，没有持续低频铺底与旁白。

## 分镜

| 时间 | 内容 |
|---|---|
| 0–4 秒 | 项目介绍、GitHub 地址与“觉得好的点点star” |
| 4–10 秒 | 中国 → 浙江 → 杭州；搜索省、市、区县直达目标区域 |
| 10–16 秒 | 附近房源光点与类型颜色；拖动、缩放查看周边位置关系 |
| 16–23 秒 | 按整租、转租、合租筛选；点击光点查看详情，左右滑动照片 |
| 23–30 秒 | 返回市级选择西湖区与余杭区，合区查看跨区附近房源 |
| 30–35 秒 | 区县级开启地铁图层，点亮线路与站点，同时查看房源分布 |
| 35–38 秒 | 点击加号、地图选点，添加照片与房源资料后发布光点 |
| 38–40 秒 | “想住哪里，附近好房，一眼看见”与功能回顾 |

本版根据当前地图导航、地区搜索、出租类型筛选、房源详情与轮播、合区、地铁和发布功能制作，不介绍登录、编辑或删除房源。

视频是产品功能动画，不是操作录屏。行政轮廓使用项目原有 DataV.GeoAtlas 数据；街道、上班地点、地铁线路与站点、室内插画、房源和价格为自行绘制的示意，片中持续标注。上班地点标记和附近圆环用于解释位置关系，不表示项目已经提供在线地点收藏、通勤路线规划或精确距离计算。地铁使用标注为“线路 A / B”的示意连线，未冒充实际运营线路或站名。视频中的价格标签仅用于展示房源信息，实际网站点击光点后查看价格。动画不请求高德 SDK / 瓦片 / 地点服务，也不读取本地凭据文件。

## 源文件

- `index.html`：分镜结构、文案、SVG 画面。
- `styles.css`：布局、颜色、字体与视觉样式。
- `animation.js`：确定性时间轴，`window.renderAt(seconds)` 可直接跳到任意帧。
- `map-data.js`：离线 SVG 轮廓，维护者通过 `node media/intro/generate-assets.mjs` 从仓库地图数据重新生成。
- `soundtrack.mjs`：程序合成的原创器乐与音效，不使用采样素材。
- `render.mjs`：本地预览服务、逐帧渲染和 FFmpeg 导出。

双击 `index.html` 即可播放动画，底部控制条支持暂停和拖动时间。浏览器端不依赖安装包或外部网络。预览会按窗口等比缩放；导出固定为 1920 × 1080。

## 重新导出

需要 Node.js 22+、Playwright、Edge/Chromium，以及带 libx264 / AAC 编码器的 FFmpeg。导出工具放在忽略目录，不影响项目生产依赖：

```powershell
npm install --prefix .local/video-tools --no-package-lock playwright
# 若未安装 Edge，可在 VIDEO_BROWSER_PATH 指定本地 Chromium 可执行文件。
$env:FFMPEG_PATH = 'C:\path\to\ffmpeg.exe'
node media/intro/render.mjs
```

FFmpeg 也可以位于 PATH，或使用 `npm install --prefix .local/video-tools --no-package-lock ffmpeg-static` 安装的本地二进制。后者下载位置取决于网络环境；不成功时指定已安装的 FFmpeg 即可。非 Windows 默认使用 Playwright 的 Chromium，需事先安装相应浏览器。

只生成关键帧：`node media/intro/render.mjs --stills`。只导出视频：`node media/intro/render.mjs --video-only`。

输出位于 `media/intro/output/`：`rental-map-intro-40s-v4.mp4`、关键帧 PNG、背景音乐 WAV 和导出信息 JSON。生成文件不提交 Git，动画源文件可以继续编辑、重新导出。生成器只打开本地动画页并阻止第三方请求；不会操作用户浏览器的已有标签页。

第二版删除持续和弦、低音与低频鼓声，改为带间歇的短拨弦音，单音最长 0.30 秒、无延迟反馈。第三版将项目介绍、https://github.com/waterha/rental_map 与“觉得好的点点star”移到开头，整体 1.25 倍速。网页预览与视频导出共用加速后的时间轴；配乐按相同顺序重排，再由 FFmpeg 的 atempo 同步加速。第四版重新编排 40 秒功能全览，加入地铁高亮、合区选区过程、照片轮播和选点发布，保留旧版 MP4。
