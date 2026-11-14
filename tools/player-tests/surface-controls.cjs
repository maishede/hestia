const assert = require('node:assert/strict')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function openVideo(t, options = {}) {
  const { items } = await (await fetch(`${base}/api/search?q=direct`)).json()
  const id = items.find(item => item.kind === 'video' && item.name === 'direct')?.id
  assert.ok(id, 'missing direct fixture')
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport: { width: 1120, height: 800 }, ...options })
  await page.goto(`${base}/#/play/${id}`)
  await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0)
  return page
}

test('a desktop click on the video pauses and the next click resumes', async t => {
  const page = await openVideo(t)
  const picture = page.locator('#gesture')
  const video = page.locator('video')
  await picture.click()
  assert.equal(await video.evaluate(v => v.paused), true, 'one click should pause')
  await picture.click()
  await page.waitForFunction(() => !document.querySelector('video').paused)
})

test('the left fullscreen button fills the web viewport and restores the small player', async t => {
  const page = await openVideo(t)
  const player = page.locator('.player')
  const initial = await player.boundingBox()
  assert.ok(initial.height < 800, 'the video opens in a small player')
  const webFullscreen = page.locator('.xgplayer-cssfullscreen')
  const fullscreen = page.locator('.xgplayer-fullscreen')
  await webFullscreen.click()
  assert.deepEqual(await player.boundingBox(), { x: 0, y: 0, width: 1120, height: 800 })
  assert.equal(await page.evaluate(() => document.fullscreenElement), null, 'web fullscreen keeps the browser chrome')
  await page.locator('#gesture').click()
  assert.equal(await page.locator('video').evaluate(v => v.paused), true, 'the video remains clickable in web fullscreen')

  await fullscreen.click()
  await page.waitForFunction(() => document.fullscreenElement === document.querySelector('.player'))
  await fullscreen.click()
  await page.waitForFunction(() => !document.fullscreenElement)
  assert.deepEqual(await player.boundingBox(), initial, 'leaving native fullscreen restores the small player')

  await webFullscreen.click()
  await webFullscreen.click()
  assert.deepEqual(await player.boundingBox(), initial, 'the left button also exits web fullscreen')
  await webFullscreen.click()
  await page.keyboard.press('Escape')
  assert.deepEqual(await player.boundingBox(), initial, 'Escape exits web fullscreen')
})

test('a mobile tap toggles controls and a double tap in the center pauses', async t => {
  const page = await openVideo(t, { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
  const picture = page.locator('#gesture')
  const video = page.locator('video')
  await picture.tap()
  await page.locator('.player.controls-hidden').waitFor()
  assert.equal(await video.evaluate(v => v.paused), false, 'one touch keeps playing')
  const { x, y, width, height } = await picture.boundingBox()
  await page.touchscreen.tap(x + width / 2, y + height / 2)
  await page.touchscreen.tap(x + width / 2, y + height / 2)
  assert.equal(await video.evaluate(v => v.paused), true)
})
