// Playback button changes made while an HLS seek is debouncing must survive the switch.
const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
async function main() {
  const result = await (await fetch(`${base}/api/search?q=transcode`)).json()
  const id = result.items.find(item => item.name === 'transcode' && item.kind === 'video').id
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  const context = await browser.newContext()
  const page = await context.newPage()
  try {
    await page.goto(`${base}/#/play/${id}`)
    await page.waitForFunction(() => document.querySelector('video')?.currentTime > 1, null, { timeout: 45000 })
    async function seekAndToggle(value, playing, useKeyboard = false) {
      await page.locator('.player').evaluate(el => el.classList.remove('controls-hidden'))
      const oldSrc = await page.locator('video').evaluate(v => v.src)
      const response = page.waitForResponse(r => r.url().endsWith(`/api/media/${id}/transcode`) && r.request().method() === 'POST')
      await page.locator('#seek').evaluate((el, val) => { el.value = String(val); el.dispatchEvent(new Event('input')); el.dispatchEvent(new Event('change')) }, value)
      if (useKeyboard) await page.keyboard.press('k')
      else await page.locator('.xgplayer-play').click()
      const immediatePlaying = await page.locator('video').evaluate(v => !v.paused)
      await response
      assert.equal(immediatePlaying, playing, 'control immediately changes the current playback state')
      await page.waitForFunction(src => document.querySelector('video')?.src.startsWith('blob:') && document.querySelector('video').src !== src, oldSrc, { timeout: 30000 })
      await sleep(1000)
      assert.equal(await page.locator('video').evaluate(v => !v.paused), playing, 'the latest user playback choice survived the source switch')
    }
    await seekAndToggle(300, false)
    await seekAndToggle(400, true)
    await seekAndToggle(500, false, true)
    await seekAndToggle(600, true, true)
    console.log('PASS player buttons and keyboard play/pause during HLS seek debounce')
  } finally { await context.close(); await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
