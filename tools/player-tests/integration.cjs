// Run against tools/devserver with the direct/transcode fixtures documented in README.md.
// Install playwright-core separately; no Node dependencies are needed to build Hestia.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
const mobile = {
  viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true,
  userAgent: 'Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/157.0.0.0 Mobile Safari/537.36',
}
async function mediaId(query) {
  const result = await (await fetch(`${base}/api/search?q=${query}`)).json()
  const media = result.items.find(item => item.name === query && item.kind === 'video')
  assert.ok(media, `Missing fixture: ${query}`)
  return media.id
}
async function touch(cdp, type, x, y) {
  await cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y, id: 1 }] })
}
async function swipe(cdp, x1, y1, x2, y2) {
  await touch(cdp, 'touchStart', x1, y1)
  for (let i = 1; i <= 8; i++) {
    await touch(cdp, 'touchMove', x1 + (x2 - x1) * i / 8, y1 + (y2 - y1) * i / 8)
    await sleep(25)
  }
  await touch(cdp, 'touchEnd', x2, y2)
  await sleep(350)
}
async function activeCues(page) {
  return page.evaluate(() => Array.from(document.querySelector('video').textTracks)
    .flatMap(t => Array.from(t.activeCues || []).map(c => c.text)).join(' '))
}
async function waitForServerProgress(id, matches, message) {
  // The local position is written before the asynchronous progress POST completes.
  const deadline = Date.now() + 5000
  do {
    const response = await fetch(`${base}/api/progress/${id}`)
    if (response.ok && matches((await response.json()).position)) return
    await sleep(100)
  } while (Date.now() < deadline)
  assert.fail(message)
}
async function main() {
  const directID = await mediaId('direct')
  const transcodeID = await mediaId('transcode')
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  const context = await browser.newContext(mobile)
  const page = await context.newPage()
  const external = [], errors = [], requests = []
  page.on('pageerror', e => errors.push(e.message))
  await page.route('**/*', route => {
    const url = new URL(route.request().url())
    requests.push(url.pathname)
    if (/^https?:$/.test(url.protocol) && url.origin !== base) { external.push(url.href); return route.abort() }
    return route.continue()
  })
  try {
    await page.goto(`${base}/#/play/${directID}`)
    await page.waitForSelector('.player video', { timeout: 15000 })
    assert.equal(await page.locator('.xgplayer-controls').count(), 1, 'xgplayer provides the visible controls')
    await page.waitForFunction(() => document.querySelector('video').currentTime > 1, { timeout: 20000 })
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => (t.activeCues || []).length > 0), null, { timeout: 15000 })
    assert.equal(await page.locator('video').evaluate(v => [...v.textTracks].filter(t => t.mode === 'showing').length), 1, 'only the selected subtitle track is shown')
    assert.ok(await page.locator('video').evaluate(v => [...v.textTracks].some(t => [...(t.activeCues || [])].some(c => typeof c.line === 'number' && c.line <= -4))), 'captions are above the custom controls')
    await page.locator('#btn-cc').click()
    await page.locator('#cc-menu button').filter({ hasText: 'direct.srt' }).click()
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => [...(t.activeCues || [])].some(c => c.text.includes('EARLY_EXTERNAL_SUBTITLE'))), null, { timeout: 15000 })
    assert.equal(await page.locator('video').evaluate(v => [...v.textTracks].filter(t => t.mode === 'showing').length), 1, 'switching subtitles hides the other track')
    await page.locator('.xgplayer-fullscreen').tap()
    await page.waitForFunction(() => document.fullscreenElement?.classList.contains('player'), null, { timeout: 5000 })
    await page.evaluate(() => document.exitFullscreen())
    await page.locator('.xgplayer-playbackrate').tap()
    await page.waitForSelector('.xgplayer .xg-options-list.active')
    await sleep(3200)
    assert.ok(await page.locator('.xgplayer .xg-options-list.active').isVisible(), 'speed menu remains visible while open')
    await page.locator('#gesture').tap({ position: { x: 100, y: 250 } })
    await sleep(400)
    assert.equal(await page.locator('.xgplayer .xg-options-list.active').count(), 0, 'tapping the video closes the speed menu')
    if (await page.locator('.player.controls-hidden').count()) {
      await page.locator('#gesture').tap({ position: { x: 100, y: 250 } })
      await sleep(400)
    }
    await page.locator('.xgplayer-playbackrate').tap()
    await page.waitForSelector('.xgplayer .xg-options-list.active')
    await page.locator('.xgplayer .xg-options-list li[rate="1.5"]').tap()
    assert.equal(await page.locator('video').evaluate(v => v.playbackRate), 1.5, 'the xgplayer speed menu changes playback rate')
    await page.locator('video').evaluate(v => { v.playbackRate = 1 })
    const cdp = await context.newCDPSession(page)
    await touch(cdp, 'touchStart', 110, 250); await sleep(650)
    assert.equal(await page.locator('video').evaluate(v => v.playbackRate), 2, 'long press temporarily uses 2x')
    await touch(cdp, 'touchEnd', 110, 250); await sleep(150)
    assert.equal(await page.locator('video').evaluate(v => v.playbackRate), 1, 'long press restores the speed')
    await page.locator('video').evaluate(v => { v.playbackRate = 1.5 })
    await touch(cdp, 'touchStart', 110, 250); await sleep(650)
    assert.equal(await page.locator('video').evaluate(v => v.playbackRate), 2)
    await touch(cdp, 'touchEnd', 110, 250); await sleep(150)
    assert.equal(await page.locator('video').evaluate(v => v.playbackRate), 1.5, 'long press restores the selected speed')
    await page.locator('video').evaluate(v => { v.pause(); v.currentTime = 5; v.volume = 0.5 })
    await swipe(cdp, 90, 250, 200, 250)
    assert.ok(await page.locator('video').evaluate(v => v.currentTime) > 20, 'horizontal gesture seeks')
    await swipe(cdp, 30, 250, 30, 350)
    assert.match(await page.locator('video').evaluate(v => v.style.filter), /brightness\(0\./, 'left gesture adjusts brightness')
    await swipe(cdp, 350, 250, 350, 350)
    assert.ok(await page.locator('video').evaluate(v => v.volume) < 0.5, 'right gesture adjusts volume')
    await page.locator('video').evaluate(v => { v.play(); v.pause() })
    await page.waitForFunction(id => JSON.parse(localStorage.getItem(`hestia.progress.${id}`) || '{}').t > 20, directID)
    await waitForServerProgress(directID, position => position > 20, 'absolute progress is synchronized with the server')
    console.log('PASS direct playback, subtitles, touch gestures and progress synchronization')

    const initialResponse = page.waitForResponse(r => r.url().endsWith(`/api/media/${transcodeID}/transcode`) && r.request().method() === 'POST')
    await page.goto(`${base}/#/play/${transcodeID}`)
    const initialSession = await (await initialResponse).json()
    await page.waitForFunction(() => document.querySelector('video')?.currentTime > 1, null, { timeout: 45000 })
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => [...(t.activeCues || [])].some(c => c.text.includes('EARLY_EXTERNAL_SUBTITLE'))), null, { timeout: 15000 })
    assert.ok(await page.locator('video').evaluate(v => v.src.startsWith('blob:')), 'MSE/hls.js is preferred to ambiguous native HLS')
    assert.ok(requests.some(url => /\/seg\d+\.ts$/.test(url)), 'the actual HLS segments were requested')
    await page.locator('video').evaluate(v => v.pause())
    const seekResponse = page.waitForResponse(r => r.url().endsWith(`/api/media/${transcodeID}/transcode`) && r.request().method() === 'POST')
    await page.locator('#seek').evaluate(el => { el.value = '400'; el.dispatchEvent(new Event('input')); el.dispatchEvent(new Event('change')) })
    const seekSession = await (await seekResponse).json()
    assert.ok(seekSession.start >= 17 && seekSession.start <= 19, `server returned offset ${seekSession.start}`)
    await page.waitForFunction(() => document.querySelector('track')?.src.includes('offset='), null, { timeout: 10000 })
    assert.ok(await page.locator('video').evaluate(v => v.paused), 'seeking while paused stays paused')
    await page.locator('video').evaluate(v => v.play())
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => [...(t.activeCues || [])].some(c => c.text.includes('LATE_EXTERNAL_SUBTITLE'))), null, { timeout: 30000 })
    assert.ok(!(await activeCues(page)).includes('EARLY_EXTERNAL_SUBTITLE'), 'expired subtitle does not reappear after seeking')
    await page.locator('#btn-cc').click()
    const embeddedOption = page.locator('#cc-menu button[data-i="1"]')
    await embeddedOption.click()
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => [...(t.activeCues || [])].some(c => c.text.includes('LATE_EMBEDDED_SUBTITLE'))), null, { timeout: 30000 })
    await page.locator('video').evaluate(async v => { await v.play(); v.pause() })
    await page.waitForFunction(id => JSON.parse(localStorage.getItem(`hestia.progress.${id}`) || '{}').t >= 18, transcodeID)
    await waitForServerProgress(transcodeID, position => position >= 18, 'transcode progress uses source time')

    const secondSeek = page.waitForResponse(r => r.url().endsWith(`/api/media/${transcodeID}/transcode`) && r.request().method() === 'POST')
    await page.locator('#seek').evaluate(el => { el.value = '450'; el.dispatchEvent(new Event('input')); el.dispatchEvent(new Event('change')) })
    const secondSession = await (await secondSeek).json()
    await page.waitForFunction(() => [...document.querySelectorAll('video track')].some(t => t.src.includes('embeddedsub') && t.src.includes('offset=')), null, { timeout: 15000 })
    await page.locator('video').evaluate(v => v.play())
    await page.waitForFunction(() => [...document.querySelector('video').textTracks].some(t => [...(t.activeCues || [])].some(c => c.text.includes('LATE_EMBEDDED_SUBTITLE'))), null, { timeout: 30000 })
    await page.locator('video').evaluate(v => v.pause())
    await page.locator('#btn-cc').click()
    await page.locator('#cc-menu button[data-i="-1"]').click()
    const previousSrc = await page.locator('video').evaluate(v => v.src)
    const thirdSeek = page.waitForResponse(r => r.url().endsWith(`/api/media/${transcodeID}/transcode`) && r.request().method() === 'POST')
    await page.locator('#seek').evaluate(el => { el.value = '500'; el.dispatchEvent(new Event('input')); el.dispatchEvent(new Event('change')) })
    const thirdSession = await (await thirdSeek).json()
    await page.waitForFunction(oldSrc => document.querySelector('video')?.src.startsWith('blob:') && document.querySelector('video').src !== oldSrc, previousSrc, { timeout: 15000 })
    await sleep(500)
    assert.equal(await page.locator('video').evaluate(v => [...v.textTracks].filter(t => t.mode === 'showing').length), 0, 'subtitle-off choice survives a transcode seek')
    await page.evaluate(() => { location.hash = '#/' })
    await page.waitForSelector('.player', { state: 'detached' })
    for (const id of [initialSession.sessionId, seekSession.sessionId, secondSession.sessionId, thirdSession.sessionId]) {
      await sleep(300)
      assert.equal((await fetch(`${base}/api/transcode/${id}/index.m3u8`)).status, 404, 'transcode session is released')
    }
    assert.deepEqual(external, [], 'all runtime assets are served locally')
    assert.deepEqual(errors, [], 'no uncaught browser errors')
    console.log('PASS transcode, offset subtitles, progress, session cleanup and offline assets')
  } finally { await context.close(); await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
