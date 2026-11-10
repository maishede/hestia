const assert = require('node:assert/strict')
const { spawnSync } = require('node:child_process')
const { test } = require('node:test')
const { setTimeout: delay } = require('node:timers/promises')
const { chromium } = require('playwright-core')

const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function playerPage(t, fixture = 'transcode') {
  const { items } = await (await fetch(`${base}/api/search?q=${fixture}`)).json()
  const id = items.find(item => item.name === fixture && item.kind === 'video')?.id
  assert.ok(id, `missing ${fixture} fixture`)
  const browser = await chromium.launch({ executablePath, args: ['--autoplay-policy=no-user-gesture-required'] })
  t.after(() => browser.close())
  const context = await browser.newContext()
  const page = await context.newPage()
  return { page, id }
}

test('an in-progress HLS playlist starts at the first segment', async t => {
  const { page, id } = await playerPage(t)
  let firstManifest = true
  await page.route('**/api/transcode/*/index.m3u8', async route => {
    if (!firstManifest) return route.continue()
    firstManifest = false
    // Deliver a populated playlist without ENDLIST, as can happen on a slow
    // connection when the transcoder is several segments ahead of the client.
    const deadline = Date.now() + 15000
    let manifest = ''
    while (Date.now() < deadline) {
      const response = await fetch(route.request().url(), {
        headers: { 'If-Modified-Since': new Date(Date.now() + 60000).toUTCString() },
        signal: AbortSignal.timeout(3000),
      })
      assert.equal(response.status, 200, 'a growing playlist must not revalidate to an older snapshot')
      assert.equal(response.headers.get('cache-control'), 'no-store')
      manifest = await response.text()
      if (manifest.includes('#EXT-X-ENDLIST')) break
      await delay(100)
    }
    assert.ok(manifest.includes('#EXT-X-ENDLIST'), 'fixture transcode did not finish')
    assert.match(manifest, /^#EXT-X-START:TIME-OFFSET=0,PRECISE=YES$/m)
    const firstSegmentSeconds = Number(manifest.match(/^#EXTINF:([0-9.]+)/m)?.[1])
    assert.ok(firstSegmentSeconds >= 3.9 && firstSegmentSeconds <= 4.1, `first segment should be about four seconds, got ${firstSegmentSeconds}`)
    assert.ok((manifest.match(/^seg\d+\.ts$/gm) || []).length >= 4, 'fixture must have more than three segments')
    await route.fulfill({ contentType: 'application/vnd.apple.mpegurl', body: manifest.replace(/^#EXT-X-ENDLIST\s*$/m, '') })
  })
  const segment = page.waitForRequest(r => /\/seg\d+\.ts$/.test(r.url()), { timeout: 30000 })
  await page.goto(`${base}/#/play/${id}`)
  assert.match((await segment).url(), /\/seg00000\.ts$/, 'transcoded VOD must not jump to the live edge')
  await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0.2, null, { timeout: 15000 })
  assert.ok(await page.locator('video').evaluate(v => v.currentTime < 4), 'playback began near the requested source position')
  await page.close({ runBeforeUnload: true })
})

const manifestPattern = '**/api/transcode/*/index.m3u8'

async function removeSession(route) {
  const path = new URL(route.request().url()).pathname.replace('/index.m3u8', '')
  assert.equal((await fetch(`${base}${path}`, { method: 'DELETE' })).status, 204)
}

function trackStarts(page, id) {
  const starts = []
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith(`/api/media/${id}/transcode`)) {
      starts.push(request.postDataJSON().start)
    }
  })
  return starts
}

test('a missing HLS session recovers at the requested position without unpausing', async t => {
  const { page, id } = await playerPage(t)
  const started = page.waitForResponse(response => response.url().endsWith(`/api/media/${id}/transcode`) &&
    response.request().method() === 'POST' && response.ok())
  await page.goto(`${base}/#/play/${id}`)
  const initial = await (await started).json()
  await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0.3, null, { timeout: 20000 })
  await page.locator('video').evaluate(v => v.pause())
  const starts = trackStarts(page, id)
  let removed = false
  await page.route(manifestPattern, async route => {
    if (!removed && !route.request().url().includes(`/api/transcode/${initial.sessionId}/`)) {
      removed = true
      await removeSession(route)
    }
    await route.continue()
  })
  const restarted = page.waitForResponse(response => response.url().endsWith(`/api/media/${id}/transcode`) &&
    response.request().method() === 'POST' && response.ok() && starts.length === 2, { timeout: 18000 })
  await page.locator('#seek').evaluate(el => {
    el.value = '400'
    el.dispatchEvent(new Event('change'))
  })
  await restarted
  await page.waitForFunction(() => document.querySelector('video')?.readyState >= 2, null, { timeout: 12000 })
  assert.equal(starts.length, 2)
  assert.ok(Math.abs(starts[0] - 18) < 0.2)
  assert.ok(Math.abs(starts[0] - starts[1]) < 0.25, 'retry preserves the absolute seek position')
  assert.ok(await page.locator('video').evaluate(v => v.paused), 'retry preserves pause intent')
  await page.close({ runBeforeUnload: true })
})

test('a repeated HLS session loss shows a retry control and stops automatic restarts', async t => {
  const { page, id } = await playerPage(t)
  const starts = trackStarts(page, id)
  await page.route(manifestPattern, async route => {
    await removeSession(route)
    await route.continue()
  })
  await page.goto(`${base}/#/play/${id}`)
  await page.waitForSelector('.player.error #err-retry', { timeout: 18000 })
  assert.equal(starts.length, 2, 'only one automatic restart is allowed')
  await page.unroute(manifestPattern)
  await page.locator('#err-retry').click()
  await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0.3, null, { timeout: 18000 })
  assert.equal(starts.length, 3, 'manual retry starts one replacement session')
  await page.close({ runBeforeUnload: true })
})

test('odd-sized 10-bit HEVC transcodes to browser-compatible H.264', async t => {
  const { page, id } = await playerPage(t, 'transcode-10bit')
  const segment = page.waitForRequest(r => /\/seg\d+\.ts$/.test(r.url()), { timeout: 30000 })
  await page.goto(`${base}/#/play/${id}`)
  const url = (await segment).url()
  const probe = spawnSync(process.env.FFPROBE || 'ffprobe', [
    '-v', 'error', '-select_streams', 'v:0', '-show_entries', 'stream=codec_name,pix_fmt,width,height', '-of', 'json', url,
  ], { encoding: 'utf8', timeout: 10000 })
  assert.equal(probe.status, 0, probe.error?.message || probe.stderr)
  const stream = JSON.parse(probe.stdout).streams[0]
  assert.equal(stream.codec_name, 'h264')
  assert.equal(stream.pix_fmt, 'yuv420p')
  assert.equal(stream.width, 322)
  assert.equal(stream.height, 182)
  await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0.3, null, { timeout: 15000 })
  await page.close({ runBeforeUnload: true })
})
