// Emulates the iOS gesture branch and verifies native-HLS fallback selection.
// This does not substitute for testing on an iPhone or Safari.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
async function mediaId(query) {
  const result = await (await fetch(`${base}/api/search?q=${query}`)).json()
  return result.items.find(item => item.name === query && item.kind === 'video').id
}
async function main() {
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  try {
    const ios = await browser.newContext({
      viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true,
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1',
    })
    const page = await ios.newPage()
    await page.goto(`${base}/#/play/${await mediaId('direct')}`)
    await page.waitForFunction(() => document.querySelector('video')?.currentTime > 1, null, { timeout: 20000 })
    await page.locator('video').evaluate(v => { v.pause(); v.currentTime = 10 })
    const cdp = await ios.newCDPSession(page)
    const touch = (type, x, y) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y, id: 1 }] })
    await touch('touchStart', 350, 350)
    for (let i = 1; i <= 8; i++) { await touch('touchMove', 350, 350 - i * 12.5); await sleep(20) }
    await touch('touchEnd', 350, 250)
    await sleep(300)
    assert.ok(await page.locator('video').evaluate(v => v.currentTime) > 20, 'iOS right-side vertical swipe seeks')
    assert.equal(await page.locator('.xgplayer-volume').count(), 0, 'iOS uses the device volume buttons')
    await ios.close()

    const native = await browser.newContext()
    const fallback = await native.newPage()
    await fallback.addInitScript(() => {
      const canPlay = HTMLMediaElement.prototype.canPlayType
      HTMLMediaElement.prototype.canPlayType = function(type) {
        return type === 'application/vnd.apple.mpegurl' ? 'probably' : canPlay.call(this, type)
      }
    })
    await fallback.route('**/assets/js/vendor/hls.min.js', route => route.abort())
    const id = await mediaId('transcode')
    const response = fallback.waitForResponse(r => r.url().endsWith(`/api/media/${id}/transcode`) && r.request().method() === 'POST')
    await fallback.goto(`${base}/#/play/${id}`)
    const session = await (await response).json()
    await fallback.waitForFunction(() => document.querySelector('video')?.src.endsWith('/index.m3u8'), null, { timeout: 15000 })
    await fallback.evaluate(() => { location.hash = '#/' })
    await fallback.waitForSelector('.player', { state: 'detached' })
    await sleep(300)
    assert.equal((await fetch(`${base}/api/transcode/${session.sessionId}/index.m3u8`)).status, 404)
    await native.close()
    console.log('PASS emulated iOS gesture and native-HLS fallback selection')

    // Exercise real HTTP media errors on the native branch. Feed a playable MP4
    // to the replacement session so Chromium can verify the resulting intent.
    const recovery = await browser.newContext()
    const recoveryPage = await recovery.newPage()
    const startURL = `/api/media/${id}/transcode`
    const mp4 = `${base}/api/media/${await mediaId('direct')}/stream`
    await recoveryPage.addInitScript(replacement => {
      const canPlay = HTMLMediaElement.prototype.canPlayType
      HTMLMediaElement.prototype.canPlayType = function(type) {
        return type === 'application/vnd.apple.mpegurl' ? 'probably' : canPlay.call(this, type)
      }
      // Chromium selects its demuxer from the URL, so assign the replacement
      // directly while Hestia still tracks the original session URL.
      const src = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'src')
      let assignments = 0
      Object.defineProperty(HTMLMediaElement.prototype, 'src', {
        ...src,
        set(url) {
          if (String(url).endsWith('/index.m3u8') && ++assignments % 2 === 0) url = replacement
          src.set.call(this, url)
        },
      })
      // Simulate a native media implementation that pauses before surfacing the error.
      document.addEventListener('error', event => {
        const video = event.target
        if (video instanceof HTMLVideoElement && video.src.endsWith('/index.m3u8')) video.pause()
      }, true)
    }, mp4)
    await recoveryPage.route('**/assets/js/vendor/hls.min.js', route => route.abort())
    const starts = []
    recoveryPage.on('request', r => {
      if (r.url().endsWith(startURL) && r.method() === 'POST') starts.push(r.postDataJSON().start)
    })
    const expired = new Set()
    await recoveryPage.route('**/api/transcode/*/index.m3u8', async route => {
      if (route.request().method() === 'HEAD') return route.continue()
      const sessionPath = new URL(route.request().url()).pathname.replace('/index.m3u8', '')
      if (!expired.has(sessionPath)) {
        expired.add(sessionPath)
        assert.equal((await fetch(`${base}${sessionPath}`, { method: 'DELETE' })).status, 204)
      }
      await route.continue()
    })
    await recoveryPage.goto(`${base}/#/play/${id}`)
    await recoveryPage.waitForFunction(() => document.querySelector('video')?.currentTime > 0.3, null, { timeout: 15000 })
    assert.equal(starts.length, 2, 'native HLS resumes playing after an initial 404')
    await recoveryPage.locator('video').evaluate(v => v.pause())
    const next = recoveryPage.waitForResponse(r => r.url().endsWith(startURL) && r.request().method() === 'POST' &&
      r.ok() && starts.length === 4, { timeout: 15000 })
    await recoveryPage.locator('#seek').evaluate(el => { el.value = '400'; el.dispatchEvent(new Event('change')) })
    await next
    await recoveryPage.waitForFunction(url => {
      const v = document.querySelector('video')
      return v?.src.endsWith(url) && v.readyState >= 2
    }, mp4)
    assert.ok(await recoveryPage.locator('video').evaluate(v => v.paused), 'a user pause survives native HLS recovery')
    assert.ok(Math.abs(starts[2] - starts[3]) < 0.25)
    await recoveryPage.locator('video').evaluate(v => {
      Object.defineProperty(v, 'error', { value: { code: 2 }, configurable: true })
      v.dispatchEvent(new Event('error'))
    })
    await recoveryPage.waitForSelector('.player.error #err-retry', { timeout: 12000 })
    assert.equal(starts.length, 4, 'native HLS also recovers at most once per start')
    await recovery.close()
    console.log('PASS simulated native-HLS session loss recovers once')
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
