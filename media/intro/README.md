# 租房版图 · 项目介绍视频

主题：**直观看见，你想住的位置附近有什么房源。**

40 秒横屏连续地图演示，1920 × 1080、30 fps，科技黑与荧光蓝风格。HTML/CSS/JS 创建动画，Edge/Chromium 逐帧渲染，FFmpeg 编码为 H.264 + AAC。配乐为带间隔的原创短拨弦与操作提示音，无持续低频铺底，无旁白。

## 连续镜头设计

整片始终保留同一张街区地图、相同房源光点和目标位置。镜头用平滑插值移动及缩放，不重建地图、不重置房源位置。搜索和合区选择以小地图浮层出现，房源详情由原光点展开，照片在同一卡片中滑动。合区逐渐点亮邻区房源，地铁线路绘出后保留到结尾，发布的新光点也保留。说明文字在左侧固定区域柔和交接；GitHub 地址和 Star 提示全程固定。没有整屏标题卡或黑场切换。

| 时间 | 内容 |
|---|---|
| 0–4 秒 | 地图已可见，房源依次点亮；引入“附近有什么房” |
| 4–9 秒 | 搜索地区与中国 / 浙江 / 杭州的层级浮层 |
| 9–14 秒 | 在同一街区缓慢移动与放大，说明位置关系和房源颜色 |
| 14–21 秒 | 筛选出租类型，点击原房源光点，卡片展开并滑动照片 |
| 21–28 秒 | 卡片收起，镜头平滑拉远；选区浮层与邻区房源逐渐出现 |
| 28–34 秒 | 同一地图绘出地铁线路、站点，与两区房源同屏 |
| 34–38 秒 | 保留地铁，地图选点并展开发布资料，点亮新房源 |
| 38–40 秒 | 收起浮层，在原地图停留并回顾项目好处 |

动画介绍地区导航、搜索、地图拖动缩放、附近房源、类型筛选、房源详情与照片轮播、合区、地铁和发布，不涉及登录、编辑、删除。

本片是功能动画，不是操作录屏。行政轮廓来自项目原有 DataV.GeoAtlas 数据；街区、目标位置、房源、图片、地铁连线和站点为绘制示意，全程标注。目标位置与圆环解释空间关系，不代表地点收藏、通勤路线或精确距离计算功能。动画不请求高德 SDK、瓦片或地点服务，也不读取凭据。

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

输出位于 `media/intro/output/`：`rental-map-intro-40s-v5.mp4`、关键帧 PNG、背景音乐 WAV 和导出信息 JSON。生成文件不提交 Git，动画源文件可以继续编辑、重新导出。生成器只打开本地动画页并阻止第三方请求；不会操作用户浏览器的已有标签页。

第五版彻底重做连续镜头与页面布局，保留以前导出文件。导出前自动逐帧检查相机锚点位移，超过 5 像素 / 帧时停止导出；导出信息记录最大实际位移。所有位置、透明度与图层状态直接由绝对时间计算，支持乱序跳帧和重新渲染。
