const assert = require('node:assert/strict')
const { test } = require('node:test')
const { setTimeout: delay } = require('node:timers/promises')
const { chromium } = require('playwright-core')

const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function directPage(t) {
  const { items } = await (await fetch(`${base}/api/search?q=direct`)).json()
  const id = items.find(item => item.name === 'direct' && item.kind === 'video')?.id
  assert.ok(id, 'missing direct fixture')
  const browser = await chromium.launch({ executablePath, args: ['--autoplay-policy=no-user-gesture-required'] })
  const context = await browser.newContext()
  t.after(async () => {
    await browser.close()
    await fetch(`${base}/api/progress/${id}`, { method: 'DELETE' })
  })
  return { page: await context.newPage(), id }
}

test('closing before five seconds preserves the existing resume position', async t => {
  const { page, id } = await directPage(t)
  const previous = await fetch(`${base}/api/progress`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mediaId: id, position: 35, duration: 45, device: 'previous-device' }),
  })
  assert.equal(previous.status, 204)
  await page.goto(`${base}/#/play/${id}`)
  await page.locator('video').evaluate(v => v.pause())
  assert.ok(await page.locator('video').evaluate(v => v.currentTime < 5))
  await page.close({ runBeforeUnload: true })
  await delay(500)
  const progress = await (await fetch(`${base}/api/progress/${id}`)).json()
  assert.equal(progress.position, 35)
  assert.equal(progress.device, 'previous-device')
})

test('leaving a finished video keeps its local and server progress cleared', async t => {
  const { page, id } = await directPage(t)
  await page.goto(`${base}/#/play/${id}`)
  await page.waitForFunction(() => Number.isFinite(document.querySelector('video')?.duration), null, { timeout: 15000 })
  const cleared = page.waitForResponse(r => r.url().endsWith(`/api/progress/${id}`) && r.request().method() === 'DELETE')
  await page.locator('video').evaluate(v => { v.currentTime = v.duration - 0.15 })
  await cleared
  await page.evaluate(() => { location.hash = '#/' })
  await page.waitForSelector('.player', { state: 'detached' })
  assert.equal(await page.evaluate(id => localStorage.getItem(`hestia.progress.${id}`), id), null)
  const progress = await (await fetch(`${base}/api/progress/${id}`)).json()
  assert.equal(progress.position, 0)
})
