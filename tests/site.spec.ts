import { test, expect } from '@playwright/test'
import { works } from '../content/works'

test('首页筛选、预览与详情导航', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  await page.goto('/')
  await expect(page).toHaveTitle('个人软件作品集 · 师说 Laoshi')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('终身求知，以诚对话。')
  await expect(page.getByRole('link', { name: '师说 Laoshi 首页' })).toBeVisible()
  await expect(page.locator('.hero-seal')).toHaveAttribute('alt', '师说印记')
  await expect(page.locator('.hero-seal')).toBeVisible()
  await expect.poll(() => page.locator('.hero-seal').evaluate(image => (image as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)
  await expect(page.locator('.work-card')).toHaveCount(6)
  await page.getByRole('button', { name: '移动端 2' }).click()
  await expect(page.locator('.work-card')).toHaveCount(2)
  await expect(page.locator('.filter-status')).toHaveText('显示 2 件作品')
  await page.getByRole('button', { name: '快速预览岁音', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('heading', { name: '岁音', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).not.toBeVisible()
  await expect(page.getByRole('button', { name: '快速预览岁音', exact: true })).toBeFocused()
  await page.locator('[data-slug="suimelody"] .card-actions a').first().click()
  await expect(page).toHaveURL(/\/work\/suimelody\/$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('岁音')
  await page.reload()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('岁音')
  expect(errors).toEqual([])
})

test('所有详情、关于、旧深链与无效地址', async ({ page }) => {
  for (const work of works) {
    const response = await page.goto(`/work/${work.slug}/`)
    expect(response?.status()).toBe(200)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(work.nameZh)
    const broken = await page.locator('img').evaluateAll(images => images.filter(image => !(image as HTMLImageElement).complete || (image as HTMLImageElement).naturalWidth === 0).map(image => image.getAttribute('src')))
    expect(broken.filter(image => image?.startsWith('/brand/'))).toEqual([])
  }
  await page.goto('/about/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('关于师说')
  await page.goto('/#product=suimelody')
  await expect(page).toHaveURL(/\/work\/suimelody\/$/)
  if (!process.env.TEST_URL) {
    expect((await page.goto('/work/not-a-work/'))?.status()).toBe(404)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('这页还没有写下。')
    expect((await page.goto('/assets/missing.js'))?.status()).toBe(404)
  }
})

test('手机菜单、减少动效与布局', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/')
  const menu = page.getByRole('button', { name: '打开菜单', exact: true })
  await expect(menu).toBeVisible()
  await menu.click()
  await expect(page.locator('#site-nav')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(menu).toHaveAttribute('aria-expanded', 'false')
  await menu.click()
  await page.locator('#site-nav').getByRole('link', { name: '03 关于' }).click()
  await expect(page).toHaveURL(/\/about\/$/)
  await expect(menu).toHaveAttribute('aria-expanded', 'false')
  for (const route of ['/', '/work/suimelody/', '/about/']) {
    await page.goto(route)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  await page.goto('/')
  await page.screenshot({ path: '/tmp/laoshi-app-mobile.png', fullPage: true })
})

test('桌面布局与无脚本预渲染', async ({ page, browser }) => {
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: '/tmp/laoshi-app-desktop.png', fullPage: true })
  if (!process.env.TEST_URL) {
    const context = await browser.newContext({ javaScriptEnabled: false })
    const staticPage = await context.newPage()
    await staticPage.goto('http://127.0.0.1:4173/')
    await expect(staticPage.locator('.work-card')).toHaveCount(6)
    await staticPage.goto('http://127.0.0.1:4173/work/coscool/')
    await expect(staticPage.getByRole('heading', { level: 1 })).toHaveText('课思库')
    await context.close()
  }
})
