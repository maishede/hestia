const assert = require('node:assert/strict')
const { readFile } = require('node:fs/promises')
const path = require('node:path')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const webDir = path.resolve(__dirname, '../../web')
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

test('web language changes update the active view and survive reloads', async t => {
  const browser = await chromium.launch({ executablePath, headless: true })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } })
  page.setDefaultTimeout(7000)
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  const movie = { id: 'video', name: '林青-电影', kind: 'video', mtime: 1700000000 }
  const responses = {
    '/api/actors': { actors: [{ name: '林青', count: 1 }] },
    '/api/actors/videos': { items: [movie], total: 1, page: 1, size: 20 },
    '/api/progress/recent': { items: [] },
    '/api/progress/video': {},
    '/api/folders': { folders: [], total: 0, page: 1, size: 20 },
    '/api/search': { items: [movie], total: 1, page: 1, size: 20 },
    '/api/media/video': {
      ...movie,
      stream: '/assets/test-media.mp4',
      playback: { mode: 'direct', transcodeAvailable: false },
      subtitles: { embedded: [{ index: 3 }] },
    },
  }
  const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png' }
  await page.route('http://hestia.test/**', async route => {
    const { pathname } = new URL(route.request().url())
    if (pathname in responses) return route.fulfill({ json: responses[pathname] })
    if (pathname === '/assets/test-media.mp4') return route.fulfill({ status: 404 })
    const file = pathname === '/' ? 'index.html' : pathname.slice(1)
    return route.fulfill({ body: await readFile(path.join(webDir, file)), contentType: types[path.extname(file)] })
  })
  await page.goto('http://hestia.test/#/')
  await page.getByRole('heading', { name: '媒体库' }).waitFor()
  const language = page.locator('#languageSelect')
  assert.equal(await language.inputValue(), 'zh-CN')
  assert.equal(await language.getAttribute('aria-label'), '界面语言')

  await language.selectOption('en')
  await page.getByRole('heading', { name: 'Media Library' }).waitFor()
  assert.equal(await page.locator('html').getAttribute('lang'), 'en')
  assert.equal(await language.getAttribute('aria-label'), 'Display language')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 390)

  await page.getByRole('combobox', { name: 'Cast' }).selectOption('林青')
  await page.getByText('1 video', { exact: true }).waitFor()
  assert.equal(await page.locator('.card-title').textContent(), movie.name)

  const input = page.locator('#searchInput')
  await input.fill('电影')
  await input.press('Enter')
  await page.getByText('1 result', { exact: true }).waitFor()
  const search = page.url()
  await language.selectOption('ja')
  await page.getByText('1 件の結果', { exact: true }).waitFor()
  assert.equal(page.url(), search)
  assert.equal(await input.inputValue(), '电影')
  await page.reload()
  await page.getByText('1 件の結果', { exact: true }).waitFor()
  assert.equal(await page.locator('html').getAttribute('lang'), 'ja')
  assert.equal(await language.inputValue(), 'ja')

  await page.evaluate(() => {
    const input = document.getElementById('searchInput')
    input.value = '新片'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    const language = document.getElementById('languageSelect')
    language.value = 'en'
    language.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await page.waitForFunction(() => new URLSearchParams(location.hash.split('?')[1]).get('q') === '新片', undefined, { timeout: 5000 })
  assert.equal(await input.inputValue(), '新片')

  await language.selectOption('ja')
  await page.goto('http://hestia.test/#/play/video')
  await page.getByRole('heading', { name: movie.name }).waitFor()
  await page.getByRole('slider', { name: '再生位置' }).waitFor()
  await page.getByRole('button', { name: '字幕' }).press('Enter')
  await page.getByRole('button', { name: 'トラック 3' }).waitFor()
  assert.match(await page.locator('.xgplayer-play').textContent(), /再生|一時停止/)
  assert.deepEqual(errors, [])
})
