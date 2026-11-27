const assert = require('node:assert/strict')
const { readFile } = require('node:fs/promises')
const path = require('node:path')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const webDir = path.resolve(__dirname, '../../web')
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

test('actor search filters locally and keeps the selected actor', async t => {
  const browser = await chromium.launch({ executablePath, headless: true })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport: { width: 1120, height: 800 } })
  const apiRequests = []
  await page.route('http://hestia.test/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.startsWith('/api/')) {
      apiRequests.push(url.pathname)
      let json
      switch (url.pathname) {
        case '/api/actors':
          json = { actors: [{ name: '林青霞', count: 4 }, { name: '陈舟', count: 2 }, { name: 'Alice', count: 1 }] }
          break
        case '/api/progress/recent':
          json = { items: [] }
          break
        case '/api/folders':
          json = { folders: [], total: 0, page: 1, size: 20 }
          break
        case '/api/actors/videos':
          json = { items: [], total: 0, page: 1, size: 20 }
          break
        default:
          throw new Error(`unexpected API request: ${url.pathname}`)
      }
      return route.fulfill({ json })
    }
    const file = url.pathname === '/' ? 'index.html' : url.pathname.slice(1)
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png' }
    return route.fulfill({ body: await readFile(path.join(webDir, file)), contentType: types[path.extname(file)] })
  })
  await page.goto('http://hestia.test/#/')
  const list = page.locator('#actorList')
  const all = list.locator('button[data-actor=""]')
  const lin = list.locator('button[data-actor="林青霞"]')
  const chen = list.locator('button[data-actor="陈舟"]')
  const alice = list.locator('button[data-actor="Alice"]')
  await lin.waitFor()
  const search = page.getByRole('searchbox', { name: '搜索主演' })
  await search.waitFor({ timeout: 5000 })
  const initialRequests = apiRequests.length

  await search.fill('  LI  ')
  assert.equal(await alice.isVisible(), true)
  assert.equal(await lin.isVisible(), false)
  assert.equal(await chen.isVisible(), false)
  assert.equal(await all.isVisible(), true, 'all collections should remain accessible')
  assert.equal(apiRequests.length, initialRequests, 'typing should not request the server')

  await search.fill('不存在')
  assert.equal(await alice.isVisible(), false)
  assert.equal(await lin.isVisible(), false)
  assert.equal(await page.getByText('没有匹配的主演').isVisible(), true)

  await search.fill('青')
  assert.equal(await lin.isVisible(), true)
  assert.equal(await chen.isVisible(), false)
  assert.equal(await page.getByText('没有匹配的主演').isVisible(), false)
  assert.equal(apiRequests.length, initialRequests, 'changing the search should stay local')

  await lin.click()
  await page.waitForURL(/actor=/)
  assert.equal(await lin.getAttribute('aria-pressed'), 'true')
  assert.equal(await search.inputValue(), '青')
  await search.fill('陈')
  assert.equal(await lin.isVisible(), false)
  await search.fill('')
  assert.equal(await lin.isVisible(), true)
  assert.equal(await lin.getAttribute('aria-pressed'), 'true')
  assert.equal(await chen.isVisible(), true)
})

test('search keeps the cast filter and the clear button returns to all collections', async t => {
  const browser = await chromium.launch({ executablePath, headless: true })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport: { width: 1120, height: 800 } })
  const movie = (id, name) => ({ id, name, kind: 'video' })
  const actors = [{ name: '林青', count: 21 }, { name: '陈舟', count: 1 }]
  await page.route('http://hestia.test/**', async route => {
    const url = new URL(route.request().url())
    const { pathname, searchParams } = url
    const pagination = { page: 1, size: 20 }
    switch (pathname) {
      case '/api/actors':
        return route.fulfill({ json: { actors: [...actors, { name: 'Alice', count: 4 }] } })
      case '/api/folders':
        return route.fulfill({ json: { ...pagination, total: 0, folders: [] } })
      case '/api/progress/recent':
      case '/api/actors/videos':
        return route.fulfill({ json: { ...pagination, total: 0, items: [] } })
      case '/api/search': {
        const actor = searchParams.get('actor')
        const items = actor === '陈舟' ? [movie('chen', '陈舟-25')] : [movie('lin', '林青-25')]
        return route.fulfill({ json: { ...pagination, total: actor === '陈舟' ? 1 : 22, items, actors } })
      }
      default: {
        const file = pathname === '/' ? 'index.html' : pathname.slice(1)
        const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png' }
        return route.fulfill({ body: await readFile(path.join(webDir, file)), contentType: types[path.extname(file)] })
      }
    }
  })

  await page.goto('http://hestia.test/#/')
  await page.getByRole('button', { name: /Alice/ }).click()
  const input = page.locator('#searchInput')
  await input.fill('25')
  await input.press('Enter')
  const list = page.locator('#actorList')
  const clear = page.getByRole('button', { name: '清除搜索' })
  await page.waitForURL(/#\/search\?q=25/)
  await page.locator('#grid').getByText('林青-25', { exact: true }).waitFor()
  await clear.waitFor({ timeout: 5000 })
  const chen = list.locator('button[data-actor="陈舟"]')
  await chen.waitFor()
  assert.equal(await list.locator('button[data-actor="Alice"]').count(), 0)
  assert.match(await list.locator('button[data-actor="林青"]').textContent(), /21/)
  await chen.click()
  await page.locator('#grid').getByText('陈舟-25', { exact: true }).waitFor()
  assert.equal(await chen.getAttribute('aria-pressed'), 'true')
  assert.equal(await input.inputValue(), '25')
  assert.equal(await list.locator('button[data-actor="林青"]').isVisible(), true)

  await page.setViewportSize({ width: 390, height: 844 })
  const picker = page.getByRole('combobox', { name: '主演', exact: true })
  assert.equal(await picker.inputValue(), '陈舟')
  await picker.selectOption('')
  await page.locator('#grid').getByText('林青-25', { exact: true }).waitFor()
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 390)

  await page.evaluate(() => {
    const input = document.getElementById('searchInput')
    input.value = 'pending search'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    document.getElementById('clearSearch').click()
  })
  await page.waitForURL('http://hestia.test/#/')
  await page.getByRole('heading', { name: '媒体库' }).waitFor()
  assert.equal(await input.inputValue(), '')
  assert.equal(await clear.isVisible(), false)
  assert.equal(await input.evaluate(element => element === document.activeElement), true)
})
