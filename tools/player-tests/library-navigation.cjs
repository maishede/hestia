const assert = require('node:assert/strict')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function openLibrary(t, viewport = { width: 1120, height: 800 }) {
  const browser = await chromium.launch({ executablePath, headless: true, args: ['--autoplay-policy=no-user-gesture-required'] })
  t.after(() => browser.close())
  const context = await browser.newContext({ viewport })
  const page = await context.newPage()
  await page.goto(`${base}/#/`)
  return { page, context }
}

async function openDetail(context, link) {
  const [detail] = await Promise.all([context.waitForEvent('page', { timeout: 5000 }), link.click()])
  return detail
}

test('clearing search restores the previous actor and results page', async t => {
  const { page } = await openLibrary(t)
  await page.getByRole('button', { name: /林青/ }).click()
  await page.getByRole('button', { name: '下一页' }).click()
  const last = page.locator('#grid').getByRole('link', { name: /林青-第21集/ })
  await last.waitFor()
  const home = page.url()
  const search = page.getByRole('searchbox')
  await search.fill('陈舟')
  await search.press('Enter')
  await page.locator('#grid').getByRole('link', { name: /陈舟-电影/ }).waitFor()

  await search.fill('')
  await page.waitForURL(home)
  await last.waitFor()
  assert.equal(await page.getByRole('button', { name: /林青/ }).getAttribute('aria-pressed'), 'true')
})

test('actor results paginate on the server and return from a new tab to the same actor page', async t => {
  const { page, context } = await openLibrary(t)
  await page.getByRole('button', { name: /林青/ }).click({ timeout: 5000 })
  const first = page.getByRole('link', { name: /林青-第1集/ })
  await first.waitFor()
  assert.equal(await page.locator('#grid .media-card').count(), 20)
  const date = first.locator('time')
  assert.match(await date.textContent(), /更新于 \d{4}-\d{2}-\d{2}/)
  assert.ok(Number.isFinite(Date.parse(await date.getAttribute('datetime'))))
  await page.getByRole('button', { name: '下一页' }).click()
  const last = page.getByRole('link', { name: /林青-第21集/ })
  await last.waitFor()
  const hash = new URL(page.url()).hash
  const detail = await openDetail(context, last)
  await detail.getByRole('heading', { name: '林青-第21集' }).waitFor()
  await detail.getByRole('link', { name: '返回主页' }).click()
  await detail.getByRole('link', { name: /林青-第21集/ }).waitFor()
  assert.equal(new URL(detail.url()).hash, hash)
  assert.equal(await detail.locator('#grid .media-card').count(), 1)
  assert.equal(await page.locator('#grid .media-card').count(), 1)

  await detail.reload()
  await detail.getByRole('link', { name: /林青-第21集/ }).waitFor()
  assert.equal(await detail.getByRole('button', { name: /林青/ }).getAttribute('aria-pressed'), 'true')
})

test('a collection opens in a new tab and its video opens in that tab', async t => {
  const { page, context } = await openLibrary(t)
  await page.getByRole('button', { name: '下一页' }).click()
  const collection = page.locator('#grid').getByRole('link', { name: /测试片库/ })
  const detail = await openDetail(context, collection)
  assert.match(await collection.locator('time').textContent(), /更新于 \d{4}-\d{2}-\d{2}/)
  const home = detail.getByRole('link', { name: '返回主页' })
  await home.waitFor()
  await detail.getByRole('link', { name: /林青-第1集/ }).click()
  await detail.getByRole('heading', { name: '林青-第1集' }).waitFor()
  assert.equal(context.pages().length, 2)
  await home.click()
  await detail.locator('#grid').getByRole('link', { name: /测试片库/ }).waitFor()
  assert.equal(new URL(detail.url()).hash, '#/?page=2')
  assert.equal(new URL(page.url()).hash, '#/?page=2')
})

test('choosing an actor keeps the sidebar scroll and supports browser history', async t => {
  const { page } = await openLibrary(t, { width: 1120, height: 250 })
  const sidebar = page.locator('.actor-list')
  const actor = sidebar.getByRole('button', { name: /陈舟/ })
  await actor.waitFor()
  const scrollTop = await sidebar.evaluate(list => {
    list.scrollTop = list.scrollHeight
    return list.scrollTop
  })
  assert.ok(scrollTop > 0, 'the sidebar is scrolled before selecting an actor')

  await actor.click()
  const film = page.locator('#grid').getByRole('link', { name: /陈舟-电影/ })
  await film.waitFor()
  await actor.waitFor()
  assert.equal(await sidebar.evaluate(list => list.scrollTop), scrollTop)
  assert.equal(await actor.getAttribute('aria-pressed'), 'true')
  assert.equal(await actor.evaluate(button => button === document.activeElement), true)

  await page.goBack()
  await page.locator('#grid').getByRole('link', { name: /Films/ }).waitFor()
  assert.equal(await sidebar.evaluate(list => list.scrollTop), scrollTop)
  await page.goForward()
  await film.waitFor()
  assert.equal(await sidebar.evaluate(list => list.scrollTop), scrollTop)
})

test('the mobile actor selector filters the library', async t => {
  const { page } = await openLibrary(t, { width: 390, height: 844 })
  const actors = page.getByRole('combobox', { name: '主演', exact: true })
  await actors.selectOption('陈舟', { timeout: 5000 })
  await page.getByRole('link', { name: /陈舟-电影/ }).waitFor()
  assert.equal(await page.locator('#grid .media-card').count(), 1)
  await actors.selectOption('')
  await page.locator('#grid').getByRole('link', { name: /Films/ }).waitFor()
})
