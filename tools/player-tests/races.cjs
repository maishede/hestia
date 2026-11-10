// Regression tests for pending transcode requests and route changes.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
async function mediaId() {
  const result = await (await fetch(`${base}/api/search?q=transcode`)).json()
  return result.items.find(item => item.name === 'transcode' && item.kind === 'video').id
}
async function seek(page, value) {
  await page.locator('#seek').evaluate((el, val) => {
    el.value = String(val)
    el.dispatchEvent(new Event('input'))
    el.dispatchEvent(new Event('change'))
  }, value)
}
async function main() {
  const id = await mediaId()
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  const context = await browser.newContext()
  const page = await context.newPage()
  try {
    await page.goto(`${base}/#/play/${id}`)
    await page.waitForFunction(() => document.querySelector('video')?.currentTime > 1, null, { timeout: 45000 })
    let releaseFirst, firstSeen
    const seen = new Promise(resolve => { firstSeen = resolve })
    const blocked = new Promise(resolve => { releaseFirst = resolve })
    let calls = 0
    const pattern = `**/api/media/${id}/transcode`
    await page.route(pattern, async route => {
      if (route.request().method() !== 'POST' || ++calls !== 1) return route.continue()
      firstSeen()
      await blocked
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'obsolete request failed' }) })
    })
    await seek(page, 300)
    await seen
    const nextResponse = page.waitForResponse(r => r.url().endsWith(`/api/media/${id}/transcode`) && r.request().method() === 'POST' && r.status() === 200)
    await seek(page, 400)
    await nextResponse
    releaseFirst()
    await sleep(1000)
    assert.equal(await page.locator('.player.error').count(), 0, 'a failed obsolete seek must not replace the active playback state')
    await page.waitForFunction(() => {
      const video = document.querySelector('video')
      return video && !video.paused && video.currentTime > 0.2
    }, null, { timeout: 12000 })
    await page.unroute(pattern)

    let releasePending, pendingSeen, pendingSession
    const pendingReady = new Promise(resolve => { pendingSeen = resolve })
    const pendingBlocked = new Promise(resolve => { releasePending = resolve })
    await page.route(pattern, async route => {
      if (route.request().method() !== 'POST') return route.continue()
      const response = await route.fetch()
      pendingSession = await response.json()
      pendingSeen()
      await pendingBlocked
      await route.fulfill({ response })
    })
    await seek(page, 600)
    await pendingReady
    const deleted = page.waitForResponse(r => r.url().endsWith(`/api/transcode/${pendingSession.sessionId}`) && r.request().method() === 'DELETE', { timeout: 10000 })
    const beacon = await page.evaluate(async () => {
      let payload
      const original = navigator.sendBeacon
      navigator.sendBeacon = (_url, data) => { payload = data; return true }
      window.dispatchEvent(new Event('pagehide'))
      navigator.sendBeacon = original
      return JSON.parse(await payload.text())
    })
    assert.ok(beacon.position >= 26 && beacon.position <= 28, `pagehide reports the pending source position: ${beacon.position}`)
    await deleted
    await page.evaluate(() => { location.hash = '#/' })
    await page.waitForSelector('.player', { state: 'detached' })
    const localProgress = await page.evaluate(mediaID => JSON.parse(localStorage.getItem(`hestia.progress.${mediaID}`) || '{}'), id)
    assert.ok(localProgress.t >= 26 && localProgress.t <= 28, `pending seek progress must remain at the requested source time: ${localProgress.t}`)
    await page.waitForFunction(async mediaID => (await (await fetch(`/api/progress/${mediaID}`)).json()).position >= 26, id, { timeout: 10000 })
    releasePending()
    assert.equal((await fetch(`${base}/api/transcode/${pendingSession.sessionId}/index.m3u8`)).status, 404, 'late session is stopped after leaving the player')
    console.log('PASS stale seek errors and late session cleanup')
  } finally { await context.close(); await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
