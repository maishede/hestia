// Close a tab after the server starts transcoding but before its response reaches JavaScript.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
const dropCancel = process.argv.includes('--lost-cancel')
async function main() {
  const found = await (await fetch(`${base}/api/search?q=transcode`)).json()
  const media = found.items.find(item => item.name === 'transcode' && item.kind === 'video')
  assert.ok(media, 'missing transcode fixture')
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  const context = await browser.newContext()
  const page = await context.newPage()
  let releaseResponse, responseReady, sessionId
  const blocked = new Promise(resolve => { releaseResponse = resolve })
  const ready = new Promise(resolve => { responseReady = resolve })
  let readyTimer
  try {
    if (dropCancel) {
      await page.addInitScript(() => {
        const send = window.fetch
        window.fetch = (url, options) => {
          if (String(url).startsWith('/api/transcode/') && options?.method === 'DELETE') {
            return Promise.reject(new TypeError('simulated lost cancellation request'))
          }
          return send(url, options)
        }
      })
    }
    await page.route(`**/api/media/${media.id}/transcode`, async route => {
      if (route.request().method() !== 'POST') return route.continue()
      const response = await route.fetch()
      assert.equal(response.status(), 200)
      sessionId = (await response.json()).sessionId
      responseReady()
      await blocked
      try { await route.fulfill({ response }) } catch {} // The tab may already be gone.
    })
    await page.goto(`${base}/#/play/${media.id}`)
    await Promise.race([ready, new Promise((_, reject) => { readyTimer = setTimeout(() => reject(new Error('transcode did not start')), 45000) })])
    clearTimeout(readyTimer)
    const manifestURL = `${base}/api/transcode/${sessionId}/index.m3u8`
    const method = dropCancel ? 'HEAD' : 'GET'
    assert.equal((await fetch(manifestURL, { method })).status, 200, 'session exists before the tab closes')
    const startedWaiting = Date.now()
    await page.close({ runBeforeUnload: true })
    releaseResponse()
    let status = 0
    const deadline = startedWaiting + (dropCancel ? 28000 : 5000)
    while (Date.now() < deadline) {
      status = (await fetch(manifestURL, { method })).status
      if (status === 404) break
      await sleep(dropCancel ? 500 : 100)
    }
    assert.equal(status, 404, 'closing the tab promptly releases a session whose response is still pending')
    console.log(`PASS pending transcode reclaimed in ${Date.now() - startedWaiting}ms${dropCancel ? ' with cancellation lost' : ''}`)
  } finally {
    clearTimeout(readyTimer)
    releaseResponse()
    if (sessionId) await fetch(`${base}/api/transcode/${sessionId}`, { method: 'DELETE' }).catch(() => {})
    await context.close().catch(() => {})
    await browser.close()
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
