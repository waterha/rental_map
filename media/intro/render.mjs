import { createRequire } from 'node:module'
import { readFile, mkdir, writeFile, readdir, access } from 'node:fs/promises'
import { createServer } from 'node:http'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { createSoundtrack } from './soundtrack.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(here, '../..')
const args = new Set(process.argv.slice(2))
const output = path.join(here, 'output')
const fps = 30, width = 1920, height = 1080, duration = 36
await mkdir(output, { recursive: true })
const packageRoot = process.env.VIDEO_NODE_MODULES || path.join(root, '.local/video-tools/node_modules')
const localRequire = createRequire(path.join(packageRoot, '__video_runtime__.cjs'))
const { chromium } = localRequire('playwright')
async function exists(name) { try { await access(name); return true } catch { return false } }
let ffmpeg = process.env.FFMPEG_PATH
if (!ffmpeg) {
  try { const candidate = localRequire('ffmpeg-static'); if (await exists(candidate)) ffmpeg = candidate } catch {}
}
if (!ffmpeg) {
  const dir = path.join(root, '.local/video-python/imageio_ffmpeg/binaries')
  if (await exists(dir)) ffmpeg = (await readdir(dir)).filter(name => /^ffmpeg.*\.exe$/.test(name)).map(name => path.join(dir, name))[0]
}
ffmpeg ||= 'ffmpeg'
const mime = { '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8', '.js': 'text/javascript; charset=utf-8' }
const allowed = new Set(['index.html','styles.css','animation.js','map-data.js'])
const server = createServer(async (req, res) => {
  const name = new URL(req.url,'http://localhost').pathname.slice(1) || 'index.html'
  if (!allowed.has(name)) { res.writeHead(404).end(); return }
  try { res.writeHead(200, { 'Content-Type':mime[path.extname(name)],'Cache-Control':'no-store' });res.end(await readFile(path.join(here,name))) }
  catch { res.writeHead(500).end() }
})
server.listen(0,'127.0.0.1');await once(server,'listening')
let browser, encoder
const browserOptions = { headless:true, args:['--hide-scrollbars','--disable-background-timer-throttling','--disable-renderer-backgrounding','--font-render-hinting=none'] }
if (process.env.VIDEO_BROWSER_PATH) browserOptions.executablePath = process.env.VIDEO_BROWSER_PATH
else if (process.platform === 'win32') browserOptions.channel = 'msedge'
try {
  browser = await chromium.launch(browserOptions)
  const page = await browser.newPage({viewport:{width,height},deviceScaleFactor:1})
  const errors=[]
  page.on('pageerror',error=>errors.push(error.message))
  await page.route('**/*',route => {
    if (route.request().url().startsWith(`http://127.0.0.1:${server.address().port}/`)) return route.continue()
    return route.abort()
  })
  await page.goto(`http://127.0.0.1:${server.address().port}/?export=1`,{waitUntil:'networkidle'})
  await page.evaluate(()=>document.fonts.ready)
  if(errors.length) throw new Error(errors.join('\n'))
  if (!args.has('--video-only')) {
    for (const t of [2.3,5.6,7.3,9.5,14.7,22.2,29.5,34.5]) {
      await page.evaluate(time=>window.renderAt(time),t)
      await page.screenshot({path:path.join(output,`frame-${t.toFixed(1).replace('.','-')}.png`),type:'png'})
      console.log(`Preview frame: ${t}s`)
    }
  }
  if (!args.has('--stills')) {
    const audio = await createSoundtrack(path.join(output,'soundtrack-v2.wav'))
    const video = path.join(output,'rental-map-intro-1080p-v2.mp4')
    const logs=[]
    encoder = spawn(ffmpeg,['-y','-hide_banner','-loglevel','warning','-f','image2pipe','-framerate',String(fps),'-vcodec','mjpeg','-i','pipe:0','-i',audio,'-map','0:v:0','-map','1:a:0','-vf','scale=in_range=full:out_range=tv:out_color_matrix=bt709,format=yuv420p','-c:v','libx264','-preset','medium','-crf','18','-pix_fmt','yuv420p','-color_range','tv','-colorspace','bt709','-color_primaries','bt709','-color_trc','bt709','-af','volume=8dB','-c:a','aac','-b:a','192k','-ar','48000','-t',String(duration),'-movflags','+faststart',video],{stdio:['pipe','ignore','pipe']})
    const completed = new Promise((resolve,reject)=>{
      encoder.once('error',reject);encoder.once('exit',code=>code===0?resolve():reject(new Error(`FFmpeg exited ${code}: ${logs.join('').slice(-1800)}`)))
    })
    // Attach a rejection handler immediately; errors are rethrown when awaited.
    completed.catch(()=>{})
    encoder.stderr.on('data',chunk=>logs.push(chunk.toString()))
    encoder.stdin.on('error',()=>{})
    for(let frame=0;frame<duration*fps;frame++) {
      await page.evaluate(time=>window.renderAt(time),frame/fps)
      const jpg=await page.screenshot({type:'jpeg',quality:95,animations:'allow',caret:'hide'})
      if(encoder.exitCode !== null) await completed
      if(!encoder.stdin.write(jpg)) await once(encoder.stdin,'drain')
      if(frame%90===0) console.log(`Rendering ${frame}/${duration*fps} frames (${Math.round(frame/duration/fps*100)}%)`)
    }
    encoder.stdin.end();await completed
    if(errors.length) throw new Error(errors.join('\n'))
    await writeFile(path.join(output,'render-info-v2.json'),JSON.stringify({title:'租房版图 · 位置附近，一眼找房',width,height,fps,duration,frames:duration*fps,video:'rental-map-intro-1080p-v2.mp4',audio:'Sparse short plucks; no sustained pad, bass or drone; no voiceover',github:'https://github.com/waterha/rental_map',callToAction:'觉得好的点点star',externalRequests:0,source:'HTML/CSS/JS deterministic animation + FFmpeg'},null,2))
    console.log(`Exported ${video}`)
  }
} finally {
  if(encoder && encoder.exitCode===null) encoder.kill()
  await browser?.close();server.close()
}
