const assert = require('node:assert/strict')
const { readFile } = require('node:fs/promises')
const path = require('node:path')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const guiDir = path.resolve(__dirname, '../../web/gui')
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

test('unavailable libraries remain manageable and show recovery guidance', async t => {
  const browser = await chromium.launch({ executablePath, headless: true })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport: { width: 600, height: 760 } })
  let state = {
    running: true, localUrl: 'http://127.0.0.1:8080', urls: [], activePort: 8080,
    port: 8080, autoStart: false, ffmpeg: true, ffmpegPath: 'ffmpeg',
    scanning: false, folders: 0, media: 0,
    update: { current: '0.0.0', status: 'idle', progress: -1 },
    libs: [{ id: 'one', label: 'Video library', path: 'E:\\videos', enabled: true,
      scanning: false, files: 0, dirs: 0, lastErr: 'The system cannot find the file specified.' }],
  }
  await page.route('http://hestia.test/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.startsWith('/gui/api/')) {
      if (url.pathname.endsWith('/toggle')) {
        const body = route.request().postDataJSON()
        state.libs[0].enabled = body.enabled
      }
      if (url.pathname.endsWith('/rescan')) {
        state.libs[0].lastErr = ''
        state.libs[0].files = 1
        state.libs[0].dirs = 1
      }
      return route.fulfill({ json: state })
    }
    const file = url.pathname === '/' ? 'index.html' : url.pathname.slice(1)
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png' }
    return route.fulfill({ body: await readFile(path.join(guiDir, file)), contentType: types[path.extname(file)] })
  })
  await page.goto('http://hestia.test/')
  const item = page.locator('.libitem')
  await item.waitFor()
  assert.match(await item.locator('.libstat').textContent(), /无法访问/)
  assert.match(await item.locator('.libstat').textContent(), /重新扫描/)
  assert.match(await item.locator('.libstat').textContent(), /移除/)
  assert.equal(await item.locator('.libdot').evaluate(el => el.classList.contains('error')), true)
  assert.equal(await item.locator('.liben').isChecked(), true)
  assert.equal(await item.locator('.libdel').isVisible(), true)

  await item.locator('.liben').uncheck()
  await page.waitForFunction(() => document.querySelector('.libstat')?.textContent.includes('已停用'))
  assert.equal(await item.locator('.libdot').evaluate(el => el.classList.contains('off')), true)

  await item.locator('.liben').check()
  await page.waitForFunction(() => document.querySelector('.libstat')?.textContent.includes('无法访问'))
  await page.locator('#btnRescan').click()
  await page.waitForFunction(() => document.querySelector('.libstat')?.textContent.includes('1 文件'))
  assert.equal(await item.locator('.libdot').evaluate(el => el.classList.contains('error')), false)
})
