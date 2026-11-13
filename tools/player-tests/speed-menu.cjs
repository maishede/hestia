const assert = require('node:assert/strict')
const { chromium } = require('playwright-core')

const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function main() {
  const { items } = await (await fetch(`${base}/api/search?q=direct`)).json()
  const id = items.find(item => item.kind === 'video' && item.name === 'direct')?.id
  assert.ok(id, 'missing direct fixture')
  const browser = await chromium.launch({ executablePath, args: ['--autoplay-policy=no-user-gesture-required'] })
  try {
    const page = await browser.newPage({ viewport: { width: 1120, height: 720 } })
    await page.goto(`${base}/#/play/${id}`)
    const button = page.locator('.xgplayer-playbackrate')
    await button.waitFor()
    await page.locator('video').evaluate(video => video.pause())
    await button.click()
    const option = page.locator('.xg-options-list.active li[rate="1.5"]')
    await option.waitFor({ timeout: 3000 })
    await page.mouse.move(560, 340, { steps: 8 })
    assert.ok(await option.isVisible(), 'leaving the button keeps the clicked speed menu open')
    await option.click()
    await page.waitForFunction(() => document.querySelector('video').playbackRate === 1.5)
    assert.equal(await page.locator('.xg-options-list.active').count(), 0)
    await button.click()
    await page.keyboard.press('Escape')
    assert.equal(await page.locator('.xg-options-list.active').count(), 0)
    console.log('PASS desktop speed menu stays open and accepts a selection')
  } finally { await browser.close() }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
