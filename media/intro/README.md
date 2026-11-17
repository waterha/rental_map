# 租房版图 · 项目介绍视频

主题：**直观看见，你想住的位置附近有什么房源。**

36 秒横屏介绍，1920 × 1080、30 fps，科技黑与荧光蓝风格。HTML/CSS/JS 创建画面和动画，Edge/Chromium 在指定时间逐帧渲染，FFmpeg 编码为 H.264 + AAC 的 MP4。视频包含中文画面文案、原创程序生成的短拨弦旋律和转场提示音，没有旁白。

## 分镜

| 时间 | 内容 |
|---|---|
| 0–4.5 秒 | 找房，先看位置：目标地点与周边房源 |
| 4.5–10 秒 | 中国 → 浙江 → 杭州，搜索直达西湖区 |
| 10–17.5 秒 | 上班地点附近，房源光点与价格同屏呈现 |
| 17.5–24 秒 | 点开房源，展示照片示意、户型、租金及联系方式入口 |
| 24–31.5 秒 | 合区查看西湖区与余杭区的房源，突出靠近区界的找房场景 |
| 31.5–36 秒 | 位置选对了，找房就清楚了 |

视频是产品功能动画，不是操作录屏。行政轮廓使用项目原有 DataV.GeoAtlas 数据；街道、上班地点、室内插画、房源和价格为自行绘制的示意，片中持续标注。上班地点标记与连线用于解释位置关系，不表示项目已经提供在线地点收藏或通勤路线规划。动画不请求高德 SDK / 瓦片 / 地点服务，也不读取本地凭据文件。

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

输出位于 `media/intro/output/`：`rental-map-intro-1080p-v2.mp4`、关键帧 PNG、背景音乐 WAV 和导出信息 JSON。生成文件不提交 Git，动画源文件可以继续编辑、重新导出。生成器只打开本地动画页并阻止第三方请求；不会操作用户浏览器的已有标签页。

第二版删除持续和弦、低音与低频鼓声，改为带间歇的短拨弦音，单音最长 0.30 秒、无延迟反馈。片尾加入 https://github.com/waterha/rental_map 与“觉得好的点点star”。旧版 MP4 保留，新版输出为带 v2 后缀的文件。
