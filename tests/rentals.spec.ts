import { test, expect, type Page } from '@playwright/test'

// Lifecycle seam: emulate SDK constructors/overlays, not the rental behavior.
// Any map reinitialization, recentering or simulated tile load is counted.
async function instrumentMap(page: Page) {
  await page.addInitScript(() => {
    const counts = { created: 0, destroyed: 0, fit: 0, mask: 0, camera: 0, tile: 0 }
    Object.assign(window, { mapCounts: counts })
    class Map {
      element: HTMLElement; zoom = 12; center = { lng: 120.1, lat: 30.2 }
      constructor(element: HTMLElement) { this.element = element; counts.created++; counts.tile++; element.append(document.createElement('canvas')) }
      on(event: string, fn: () => void) { if (event === 'complete') setTimeout(fn, 0) }
      off() {}
      add(overlays: { content?: HTMLElement } | { content?: HTMLElement }[]) {
        for (const item of Array.isArray(overlays) ? overlays : [overlays]) if (item.content) this.element.append(item.content)
      }
      remove(overlays: { content?: HTMLElement }[]) { for (const item of overlays) item.content?.remove() }
      setMask() { counts.mask++ }
      setFitView() { counts.fit++ }
      getZoom() { return this.zoom }
      getCenter() { return this.center }
      setZoom(zoom: number) { this.zoom = zoom; counts.camera++; counts.tile++ }
      destroy() { counts.destroyed++; this.element.replaceChildren() }
    }
    class Marker {
      content?: HTMLElement
      constructor(options: { content?: HTMLElement }) { this.content = options.content }
      setMap() {}
    }
    Object.assign(window, { AMap: { Map, Marker, Polygon: Marker, Polyline: Marker, Pixel: class {} } })
  })
}
const markers = [
  { id: 'whole', type: '整租', price: 3000 },
  { id: 'sublet', type: '转租', price: 2500 },
  { id: 'shared', type: '合租', price: 1500 },
].map(marker => ({ ...marker, longitude: 120.1, latitude: 30.2, isDemo: true }))
const detail = { id: 'whole', title: '独立详情请求', type: '整租', community: '测试小区', city: '杭州市', district: '西湖区', rooms: 2, area: 60, price: 3000, description: '测试房源', image: '/images/home-1.jpg', isDemo: true }
async function counts(page: Page) { return page.evaluate(() => (window as unknown as { mapCounts: Record<string, number> }).mapCounts) }

for (const closeMethod of ['button', 'Escape'] as const) test(`closing a listing after panning with ${closeMethod} keeps the camera and map DOM in place`, async ({ page }) => {
  test.setTimeout(90000)
  await page.goto('/?region=330106')
  const map = page.locator('.district-map')
  const pin = page.locator('.rental-light').first()
  await expect(pin).toBeVisible({ timeout: 55000 })
  // Keep the original trigger focused so closing exercises native focus restoration.
  await pin.focus()
  await pin.click()
  await expect(page.getByRole('dialog', { name: '房源详情' })).toBeVisible()
  const canvas = await map.locator('canvas').first().elementHandle()
  await page.mouse.move(650, 450)
  await page.mouse.wheel(0, -650)
  await expect.poll(async () => Number(await map.getAttribute('data-zoom'))).toBeGreaterThan(12)
  for (let i = 0; i < 2; i++) {
    const center = await map.getAttribute('data-center')
    await page.mouse.move(110, 450)
    await page.mouse.down()
    await page.mouse.move(1000, 650, { steps: 25 })
    await page.mouse.up()
    await expect(map).not.toHaveAttribute('data-center', center!)
  }
  await expect.poll(async () => {
    const bounds = await pin.boundingBox()
    return bounds ? bounds.x < 0 || bounds.y > 1080 || bounds.x > 1512 || bounds.y < 0 : false
  }).toBe(true)
  const snapshot = () => map.evaluate(element => ({
    center: element.getAttribute('data-center'), zoom: element.getAttribute('data-zoom'),
    ancestors: [...document.querySelectorAll('.district-map,.district-map *,.district-layer,main,body,html')]
      .filter(node => node instanceof HTMLElement && (node.scrollLeft || node.scrollTop))
      .map(node => ({ className: node.className, left: node.scrollLeft, top: node.scrollTop })),
    canvas: [...element.querySelectorAll('canvas')].map(node => {
      const box = node.getBoundingClientRect()
      return { x: box.x, y: box.y, width: box.width, height: box.height }
    }),
  }))
  let lastCenter = '', stable = 0
  await expect.poll(async () => {
    const center = await map.getAttribute('data-center')
    stable = center === lastCenter ? stable + 1 : 0
    lastCenter = center || ''
    return stable
  }, { intervals: [150] }).toBeGreaterThanOrEqual(4)
  const before = await snapshot()
  if (closeMethod === 'button') await page.getByRole('button', { name: '关闭房源详情', exact: true }).click()
  else await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: '房源详情' })).toHaveCount(0)
  await expect(pin).toBeFocused()
  expect(await snapshot()).toEqual(before)
  expect(await canvas!.evaluate(element => element.isConnected)).toBe(true)
})

test('list filters and list/detail retries never reload geography or the base map', async ({ page }) => {
  await instrumentMap(page)
  let failList = false, failDetail = true, listRequests = 0, detailRequests = 0, geographyRequests = 0
  page.on('request', request => { if (/^\/api\/(regions|maps)(\/|$)/.test(new URL(request.url()).pathname)) geographyRequests++ })
  await page.route('**/api/listings?**', route => {
    listRequests++
    if (failList) return route.fulfill({ status: 502, json: { error: '房源列表暂不可用' } })
    const type = new URL(route.request().url()).searchParams.get('type')
    const selected = markers.filter(marker => !type || marker.type === type)
    return route.fulfill({ json: { markers: selected, total: selected.length } })
  })
  await page.route('**/api/listings/whole', route => {
    detailRequests++
    return failDetail ? route.fulfill({ status: 502, json: { error: '详情暂不可用' } }) : route.fulfill({ json: detail })
  })
  await page.goto('/?region=330106')
  await expect(page.locator('.rental-light')).toHaveCount(3)
  const before = await counts(page), geographyBefore = geographyRequests
  expect(before.created).toBe(1)
  const canvas = await page.locator('.district-map canvas').elementHandle()
  await page.locator('[data-listing="whole"]').click()
  await expect(page.getByRole('dialog', { name: '房源详情' })).toContainText('详情暂不可用')
  failDetail = false
  await page.getByRole('button', { name: '重新加载房源', exact: true }).click()
  await expect(page.locator('.listing-subtitle')).toHaveText('独立详情请求')
  expect(detailRequests).toBe(2); expect(listRequests).toBe(1)
  await page.keyboard.press('Escape')
  const filters = page.getByRole('group', { name: '租房类型' })
  await filters.getByRole('button', { name: '合租', exact: true }).click()
  await expect(page.locator('.rental-light')).toHaveCount(1)
  await expect(page.locator('.rental-light')).toHaveAttribute('data-listing', 'shared')
  await filters.getByRole('button', { name: '合租', exact: true }).click()
  expect(listRequests).toBe(2)
  failList = true
  await filters.getByRole('button', { name: '转租', exact: true }).click()
  await expect(page.locator('.rental-results-status')).toContainText('房源列表暂不可用')
  await expect(page.locator('.rental-light')).toHaveCount(0)
  failList = false
  await page.getByRole('button', { name: '重试房源', exact: true }).click()
  await expect(page.locator('.rental-light')).toHaveAttribute('data-listing', 'sublet')
  await filters.getByRole('button', { name: '全部', exact: true }).click()
  await expect(page.locator('.rental-light')).toHaveCount(3)
  expect(await counts(page)).toEqual(before)
  expect(geographyRequests).toBe(geographyBefore)
  expect(await canvas!.evaluate(el => el.isConnected)).toBe(true)
})

test('late list and detail results cannot override a new filter or reopened detail', async ({ page }) => {
  await instrumentMap(page)
  let releaseList!: () => void, releaseDetail!: () => void
  const waitList = new Promise<void>(resolve => { releaseList = resolve })
  const waitDetail = new Promise<void>(resolve => { releaseDetail = resolve })
  let detailCount = 0
  await page.route('**/api/listings?**', async route => {
    const type = new URL(route.request().url()).searchParams.get('type')
    if (type === '合租') await waitList
    const selected = markers.filter(marker => !type || marker.type === type)
    await route.fulfill({ json: { markers: selected, total: selected.length } }).catch(() => {})
  })
  await page.route('**/api/listings/whole', async route => {
    const attempt = ++detailCount
    if (attempt === 1) await waitDetail
    await route.fulfill({ json: { ...detail, title: attempt === 1 ? '旧详情' : '新详情' } }).catch(() => {})
  })
  await page.goto('/?region=330106')
  await expect(page.locator('.rental-light')).toHaveCount(3)
  const filters = page.getByRole('group', { name: '租房类型' })
  await filters.getByRole('button', { name: '合租', exact: true }).click()
  await filters.getByRole('button', { name: '转租', exact: true }).click()
  await expect(page.locator('.rental-light')).toHaveAttribute('data-listing', 'sublet')
  releaseList()
  await expect(page.locator('.rental-light')).toHaveAttribute('data-listing', 'sublet')
  await filters.getByRole('button', { name: '全部', exact: true }).click()
  await page.locator('[data-listing="whole"]').click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.locator('[data-listing="whole"]').click()
  await expect(page.locator('.listing-subtitle')).toHaveText('新详情')
  releaseDetail()
  await expect(page.locator('.listing-subtitle')).toHaveText('新详情')
  expect((await counts(page)).created).toBe(1)
})

test('real AMap canvas, camera and metro survive rental filter and detail retry', async ({ page }) => {
  test.setTimeout(90000)
  let failDetail = true
  await page.route('**/api/listings/*', async route => {
    if (failDetail) await route.fulfill({ status: 502, json: { error: '详情暂不可用' } })
    else await route.continue()
  })
  await page.goto('/?region=330106')
  await expect(page.locator('.rental-light').first()).toBeVisible({ timeout: 55000 })
  const canvas = await page.locator('.district-map canvas').first().elementHandle()
  const camera = { zoom: await page.locator('.district-map').getAttribute('data-zoom'), center: await page.locator('.district-map').getAttribute('data-center') }
  await page.getByRole('button', { name: '地铁图层', exact: true }).click()
  await expect.poll(async () => Number(await page.locator('.district-map').getAttribute('data-metro-lines'))).toBeGreaterThan(0)
  const station = await page.locator('.metro-station-pin').first().elementHandle()
  await page.locator('.rental-light').first().click()
  await expect(page.locator('.detail-error')).toContainText('详情暂不可用')
  failDetail = false
  await page.getByRole('button', { name: '重新加载房源', exact: true }).click()
  await expect(page.locator('.listing-contact')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('group', { name: '租房类型' }).getByRole('button', { name: '合租', exact: true }).click()
  await expect(page.locator('.rental-types')).toHaveAttribute('aria-busy', 'false')
  expect(await canvas!.evaluate(el => el.isConnected)).toBe(true)
  expect(await station!.evaluate(el => el.isConnected)).toBe(true)
  await expect(page.locator('.district-map')).toHaveAttribute('data-zoom', camera.zoom!)
  await expect(page.locator('.district-map')).toHaveAttribute('data-center', camera.center!)
  await expect(page.getByRole('button', { name: '地铁图层', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await page.screenshot({ path: '.local/rentals-independent.png' })
  await page.setViewportSize({ width: 390, height: 844 })
  const filters = await page.getByRole('group', { name: '租房类型' }).boundingBox()
  expect(filters!.x).toBeGreaterThanOrEqual(0)
  expect(filters!.x + filters!.width).toBeLessThanOrEqual(390)
  await page.screenshot({ path: '.local/rentals-independent-mobile.png' })
})
