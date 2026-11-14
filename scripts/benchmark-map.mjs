import { chromium } from '@playwright/test'
import { mkdir, readFile, writeFile } from 'node:fs/promises'

function checkBudget(results) {
  const interactions = results.filter(row => row.name === 'country' && ['drag', 'zoom'].includes(row.action))
  if (interactions.length !== 2) throw new Error('Report must include national drag and zoom results.')
  const failures = interactions.filter(row => !Number.isFinite(row.scriptMs) || row.scriptMs > 1200)
  if (failures.length) throw new Error(`National map exceeded 1200ms of script time per 60 inputs at 4x CPU throttle: ${JSON.stringify(failures)}`)
  console.log('National drag/zoom script budgets passed (1200ms per 60 inputs at 4x CPU throttle).')
}
if (process.argv.includes('--check-report')) {
  checkBudget(JSON.parse(await readFile(process.argv.at(-1), 'utf8')).results)
  process.exit(0)
}

// Replay real browser input against a production build; no district SDK or mail calls.
const browser = await chromium.launch({ channel: 'msedge', headless: true })
const results = []
try {
  const page = await browser.newPage({ viewport: { width: 1512, height: 1080 } })
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Performance.enable')
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 })
  for (const [name, code, label] of [['country', '', '浙江省'], ['province', '330000', '杭州市'], ['city', '330100', '西湖区']]) {
    for (const action of ['hover', 'drag', 'zoom']) {
      console.log(`Measuring ${name}: ${action}`)
      await page.goto(`${process.env.TEST_BASE_URL || 'http://127.0.0.1:8082'}/${code ? `?region=${code}` : ''}`)
      await page.getByRole('button', { name: `进入${label}`, exact: true }).waitFor()
      await page.waitForTimeout(400)
      const complexity = await page.locator('.map-camera').evaluate(el => ({ paths: el.querySelectorAll('path').length, pathCharacters: [...el.querySelectorAll('path')].reduce((sum, p) => sum + (p.getAttribute('d') || '').length, 0) }))
      await page.mouse.move(550, 400)
      if (action === 'drag') await page.mouse.down()
      await page.evaluate(() => {
        window.__mapFrames = []; window.__mapLast = performance.now()
        const frame = now => { window.__mapFrames.push(now - window.__mapLast); window.__mapLast = now; window.__mapRaf = requestAnimationFrame(frame) }
        window.__mapRaf = requestAnimationFrame(frame)
      })
      const before = Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(m => [m.name, m.value]))
      const start = performance.now()
      for (let i = 0; i < 60; i++) {
        if (action === 'zoom') await page.mouse.wheel(0, i < 30 ? -12 : 12)
        else await page.mouse.move(550 + i * 6, 400 + Math.sin(i / 10) * 70)
      }
      if (action === 'drag') await page.mouse.up()
      await page.waitForTimeout(100)
      const elapsedMs = performance.now() - start
      const after = Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(m => [m.name, m.value]))
      const frames = await page.evaluate(() => { cancelAnimationFrame(window.__mapRaf); return window.__mapFrames })
      frames.sort((a, b) => a - b)
      results.push({ name, action, ...complexity, elapsedMs: Math.round(elapsedMs), scriptMs: Math.round((after.ScriptDuration - before.ScriptDuration) * 1000), taskMs: Math.round((after.TaskDuration - before.TaskDuration) * 1000), frames: frames.length, p95FrameMs: Math.round(frames[Math.floor(frames.length * .95)] || 0), framesOver50Ms: frames.filter(t => t > 50).length })
      console.log(JSON.stringify(results.at(-1)))
    }
  }
} finally { await browser.close() }
await mkdir('.local', { recursive: true })
const output = process.argv.slice(2).find(arg => !arg.startsWith('--')) || '.local/map-benchmark.json'
await writeFile(output, JSON.stringify({ browser: 'Edge headless', viewport: '1512x1080', cpuThrottle: 4, inputEvents: 60, results }, null, 2))
console.table(results)
if (process.argv.includes('--check')) checkBudget(results)
