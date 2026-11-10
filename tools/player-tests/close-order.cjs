// Exercise closing before either the reservation or the start request is delivered.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
async function within(promise, message) {
  let timer
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(message)), 30000) })]) }
  finally { clearTimeout(timer) }
}
async function main() {
  const found = await (await fetch(`${base}/api/search?q=transcode`)).json()
  const media = found.items.find(item => item.name === 'transcode' && item.kind === 'video')
  assert.ok(media, 'missing transcode fixture')
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  try {
    const context = await browser.newContext()
    const page = await context.newPage()
    let release, started, startBody
    const blocked = new Promise(resolve => { release = resolve })
    const ready = new Promise(resolve => { started = resolve })
    await page.route(`**/api/media/${media.id}/transcode`, async route => {
      startBody = JSON.parse(route.request().postData())
      started()
      await blocked
      try { await route.continue() } catch {}
    })
    try {
      await page.goto(`${base}/#/play/${media.id}`)
      await within(ready, 'start request was not sent')
      assert.ok(startBody.sessionId, 'start request carries the reserved ID')
      await page.close({ runBeforeUnload: true })
      await sleep(500)
      const response = await fetch(`${base}/api/media/${media.id}/transcode`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(startBody),
      })
      assert.equal(response.status, 410, 'a late start cannot launch a canceled reservation')
      console.log('PASS closing before start prevents late FFmpeg launch')
    } finally {
      release()
      if (startBody?.sessionId) await fetch(`${base}/api/transcode/${startBody.sessionId}`, { method: 'DELETE' }).catch(() => {})
      await context.close()
    }

    const reserveContext = await browser.newContext()
    const reservePage = await reserveContext.newPage()
    let releaseReserve, reserved, reservationId, startSeen = false
    const reserveBlocked = new Promise(resolve => { releaseReserve = resolve })
    const reserveReady = new Promise(resolve => { reserved = resolve })
    reservePage.on('request', req => { if (req.method() === 'POST' && req.url().endsWith(`/api/media/${media.id}/transcode`)) startSeen = true })
    await reservePage.route(`**/api/media/${media.id}/transcode/reserve`, async route => {
      const response = await route.fetch()
      reservationId = (await response.json()).sessionId
      reserved()
      await reserveBlocked
      try { await route.fulfill({ response }) } catch {}
    })
    try {
      await reservePage.goto(`${base}/#/play/${media.id}`)
      await within(reserveReady, 'reservation response was not received')
      assert.ok(reservationId)
      await reservePage.close({ runBeforeUnload: true })
      releaseReserve()
      await sleep(500)
      assert.equal(startSeen, false, 'no FFmpeg start is sent without a delivered reservation')
      console.log('PASS closing before reservation response starts no transcode')
    } finally {
      releaseReserve()
      if (reservationId) await fetch(`${base}/api/transcode/${reservationId}`, { method: 'DELETE' }).catch(() => {})
      await reserveContext.close()
    }

    const restoredContext = await browser.newContext()
    const restoredPage = await restoredContext.newPage()
    try {
      const first = restoredPage.waitForResponse(r => r.url().endsWith(`/api/media/${media.id}/transcode`) && r.request().method() === 'POST')
      await restoredPage.goto(`${base}/#/play/${media.id}`)
      const original = await (await first).json()
      await restoredPage.waitForFunction(() => document.querySelector('video')?.currentTime > 1, null, { timeout: 45000 })
      const currentTime = await restoredPage.locator('video').evaluate(v => v.currentTime)
      let restartCount = 0
      restoredPage.on('request', req => {
        if (req.method() === 'POST' && req.url().endsWith(`/api/media/${media.id}/transcode`)) restartCount++
      })
      const stopped = restoredPage.waitForResponse(r => r.url().endsWith(`/api/transcode/${original.sessionId}`) && r.request().method() === 'DELETE')
      await restoredPage.evaluate(() => {
        const seek = document.querySelector('#seek')
        seek.value = '400'
        seek.dispatchEvent(new Event('input'))
        seek.dispatchEvent(new Event('change'))
        window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
      })
      await stopped
      assert.equal((await fetch(`${base}/api/transcode/${original.sessionId}/index.m3u8`)).status, 404)
      const restarted = restoredPage.waitForResponse(r => r.url().endsWith(`/api/media/${media.id}/transcode`) && r.request().method() === 'POST' && r.status() === 200)
      await restoredPage.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })))
      const replacement = await (await restarted).json()
      assert.notEqual(replacement.sessionId, original.sessionId)
      assert.ok(replacement.start >= currentTime - 0.5, 'restored session continues at the previous source time')
      await restoredPage.waitForFunction(() => document.querySelector('video') && !document.querySelector('video').paused, null, { timeout: 30000 })
      await sleep(1000)
      assert.equal(restartCount, 1, 'the pre-hide seek timer must not start a duplicate session')
      console.log('PASS back-forward cache restoration restarts the session')
    } finally { await restoredContext.close() }
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
