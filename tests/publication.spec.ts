import { test, expect, type Page } from '@playwright/test'

async function setup(page: Page, loggedIn = true) {
  await page.addInitScript(() => {
    const counts = { created: 0, destroyed: 0, fit: 0 }
    const state = { point: [120.08362, 30.200766], counts }
    Object.assign(window, { publicationMap: state })
    class Map {
      element: HTMLElement
      constructor(element: HTMLElement) { this.element = element; counts.created++; element.append(document.createElement('canvas')) }
      on(event: string, fn: (event?: unknown) => void) {
        if (event === 'complete') setTimeout(fn, 0)
        if (event === 'click') this.element.addEventListener('click', event => {
          if ((event.target as Element).closest('button')) return
          fn({ lnglat: { lng: state.point[0], lat: state.point[1] } })
        })
      }
      off() {}
      add(overlays: { content?: HTMLElement } | { content?: HTMLElement }[]) { for (const overlay of Array.isArray(overlays) ? overlays : [overlays]) if (overlay.content) this.element.append(overlay.content) }
      remove(overlays: { content?: HTMLElement } | { content?: HTMLElement }[]) { for (const overlay of Array.isArray(overlays) ? overlays : [overlays]) overlay.content?.remove() }
      setMask() {}
      setFitView() { counts.fit++ }
      getZoom() { return 12 }
      getCenter() { return { lng: 120.08362, lat: 30.200766 } }
      destroy() { counts.destroyed++; this.element.replaceChildren() }
    }
    class Marker { content?: HTMLElement; constructor(options: { content?: HTMLElement }) { this.content = options.content }; setMap() {} }
    Object.assign(window, { AMap: { Map, Marker, Polygon: Marker, Polyline: Marker, Pixel: class {} } })
  })
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: loggedIn ? { id: 'publisher', email: 'publisher@gmail.com' } : null } }))
}
const photo = { name: 'indoor.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j6GQAAAAASUVORK5CYII=', 'base64') }

test('guest browsing stays public; plus requests login and resumes district location selection', async ({ page }) => {
  await setup(page, false)
  await page.route('**/api/auth/login', route => route.fulfill({ json: { user: { id: 'publisher', email: 'publisher@gmail.com' } } }))
  await page.goto('/?region=330106')
  await expect(page.locator('.rental-light').first()).toBeVisible()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.locator('.rental-light').first().click()
  await expect(page.getByRole('dialog', { name: '房源详情' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '发布房源', exact: true }).click()
  const account = page.getByRole('dialog', { name: '邮箱账号' })
  await account.getByLabel('邮箱地址', { exact: true }).fill('publisher@gmail.com')
  await account.getByLabel('密码', { exact: true }).fill('Test-only-password123!')
  await account.getByRole('button', { name: '登录', exact: true }).click()
  await expect(account).toHaveCount(0)
  await expect(page.locator('.district-map')).toHaveAttribute('data-selecting', 'true')
  await page.locator('.district-map').click({ position: { x: 500, y: 400 } })
  await expect(page.getByRole('dialog', { name: '发布房源资料' })).toBeVisible()
  await page.getByRole('button', { name: '取消发布', exact: true }).click()
  await expect(page.locator('.draft-light')).toHaveCount(0)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('publication preserves draft on retry and reselection, then adds a real point without reloading the map', async ({ page }) => {
  await setup(page)
  const listing = { id: 'new-listing', title: '西湖阳光一房一卫住宅', community: '阳光花园', description: '交通方便采光良好，周边生活配套齐全', type: '转租', rentalSource: '个人转租', propertyType: '住宅', rooms: 1, halls: 0, bathrooms: 1, area: 45, price: 2800, contact: 'contact@example.com', city: '杭州市', district: '西湖区', longitude: 120.08362, latitude: 30.200766, isDemo: false, image: '/images/home-1.jpg', images: ['/images/home-1.jpg', '/images/home-2.jpg'] }
  let published = false, attempts = 0
  await page.route('**/api/auth/login', route => route.fulfill({ json: { user: { id: 'publisher', email: 'publisher@gmail.com' } } }))
  await page.route('**/api/listings?**', route => route.fulfill({ json: { markers: published ? [listing] : [], total: published ? 1 : 0 } }))
  await page.route('**/api/listings', async route => {
    attempts++
    expect(route.request().headers()['content-type']).toContain('multipart/form-data')
    const body = route.request().postDataBuffer()!.toString()
    expect(body).toContain('name="photos"')
    expect(body).toContain('"longitude":120.08362')
    expect(body).toContain('"bathrooms":1')
    if (attempts === 1) return route.fulfill({ status: 401, json: { error: '登录已过期，请重新登录' } })
    published = true
    return route.fulfill({ status: 201, json: listing })
  })
  await page.goto('/?region=330106')
  await expect(page.getByRole('button', { name: '我的账号' })).toBeVisible()
  await page.getByRole('button', { name: '发布房源', exact: true }).click()
  await page.evaluate(() => { (window as unknown as { publicationMap: { point: number[] } }).publicationMap.point = [100, 40] })
  await page.locator('.district-map').click({ position: { x: 500, y: 400 } })
  await expect(page.locator('.publication-guide')).toContainText('区县边界内')
  await expect(page.locator('.draft-light')).toHaveCount(0)
  await page.evaluate(() => { (window as unknown as { publicationMap: { point: number[] } }).publicationMap.point = [120.08362, 30.200766] })
  await page.locator('.district-map').click({ position: { x: 500, y: 400 } })
  const form = page.getByRole('dialog', { name: '发布房源资料' })
  await expect(page.locator('.draft-light')).toHaveCount(1)
  await form.getByLabel('房源标题', { exact: true }).fill(listing.title)
  await form.getByLabel('小区 / 公寓名称', { exact: true }).fill(listing.community)
  await form.getByLabel('出租类型', { exact: true }).selectOption('转租')
  await form.getByLabel('出租身份', { exact: true }).selectOption('个人转租')
  await form.getByLabel('月租（元）', { exact: true }).fill('2800')
  await form.getByLabel('面积（m²）', { exact: true }).fill('45')
  await form.getByLabel('房源描述', { exact: true }).fill(listing.description)
  await form.getByLabel('联系方式', { exact: true }).fill(listing.contact)
  await form.getByRole('button', { name: '发布房源，点亮地图' }).click()
  await expect(form.getByRole('alert')).toContainText('1–5')
  await form.getByLabel('上传房源照片', { exact: true }).setInputFiles(Array.from({ length: 6 }, (_, i) => ({ ...photo, name: `image-${i}.png` })))
  await expect(form.getByRole('alert')).toContainText('最多上传 5 张')
  await form.getByLabel('上传房源照片', { exact: true }).setInputFiles([photo, { ...photo, name: 'room2.png' }])
  await expect(form.locator('.publication-photo')).toHaveCount(2)
  await form.getByRole('button', { name: '重新选点' }).click()
  await expect(form).not.toBeVisible()
  await page.locator('.district-map').click({ position: { x: 500, y: 400 } })
  await expect(form.getByLabel('房源标题', { exact: true })).toHaveValue(listing.title)
  await expect(form.locator('.publication-photo')).toHaveCount(2)
  await page.setViewportSize({ width: 390, height: 844 })
  const bounds = await form.boundingBox()
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  await page.screenshot({ path: '.local/publication-mobile.png' })
  await form.getByRole('button', { name: '发布房源，点亮地图' }).click()
  await expect(form.getByRole('alert')).toContainText('登录已过期')
  await form.getByRole('button', { name: '重新登录' }).click()
  const account = page.getByRole('dialog', { name: '邮箱账号' })
  await account.getByLabel('邮箱地址', { exact: true }).fill('publisher@gmail.com')
  await account.getByLabel('密码', { exact: true }).fill('Test-only-password123!')
  await page.keyboard.press('Tab')
  await expect(account.getByRole('button', { name: '登录', exact: true })).toBeFocused()
  await account.getByRole('button', { name: '登录', exact: true }).click()
  await expect(account).toHaveCount(0)
  await expect(form).toBeVisible()
  await expect(form.getByLabel('联系方式', { exact: true })).toHaveValue(listing.contact)
  await expect(page.locator('[data-listing="new-listing"]')).toHaveCount(0)
  await form.getByRole('button', { name: '发布房源，点亮地图' }).click()
  await expect(form).toHaveCount(0)
  await expect(page.locator('[data-listing="new-listing"]')).toHaveCount(1)
  await expect(page.locator('.draft-light')).toHaveCount(0)
  await expect(page.locator('.listing-contact')).toContainText(listing.contact)
  await expect(page.locator('.listing-facts')).toContainText('1 室 0 厅 1 卫')
  await page.getByRole('button', { name: '下一张照片' }).click()
  await expect(page.locator('.photo-counter')).toHaveText('2 / 2')
  const plus = await page.getByRole('button', { name: '发布房源', exact: true }).boundingBox()
  const trail = await page.getByRole('navigation', { name: '当前行政区路径' }).boundingBox()
  expect(plus!.x + plus!.width).toBeLessThanOrEqual(trail!.x)
  const counts = await page.evaluate(() => (window as unknown as { publicationMap: { counts: Record<string, number> } }).publicationMap.counts)
  expect(counts).toEqual({ created: 1, destroyed: 0, fit: 1 })
})

test('country level publication guides navigation without loading the district SDK', async ({ page }) => {
  await setup(page)
  await page.goto('/')
  await expect(page.getByRole('button', { name: '我的账号' })).toBeVisible()
  await page.getByRole('button', { name: '发布房源', exact: true }).click()
  await expect(page.locator('.publication-guide')).toContainText('省、市、区县')
  await expect(page.locator('.district-map')).toHaveCount(0)
  await page.getByRole('button', { name: '取消发布', exact: true }).click()
  await expect(page.locator('.publication-guide')).toHaveCount(0)
})
