const assert = require('node:assert/strict')
const { readFile } = require('node:fs/promises')
const path = require('node:path')
const { test } = require('node:test')
const { chromium } = require('playwright-core')

const guiDir = path.resolve(__dirname, '../../web/gui')
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function openConsole(t, respond, viewport = { width: 620, height: 590 }) {
  const browser = await chromium.launch({ executablePath, headless: true })
  t.after(() => browser.close())
  const page = await browser.newPage({ viewport })
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: async address => { window.copiedAddress = address } },
    })
  })
  await page.route('http://hestia.test/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.startsWith('/gui/api/')) {
      return route.fulfill({ json: respond(url.pathname.slice('/gui/api/'.length), route.request().postDataJSON()) })
    }
    const file = url.pathname === '/' ? 'index.html' : url.pathname.slice(1)
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png' }
    return route.fulfill({ body: await readFile(path.join(guiDir, file)), contentType: types[path.extname(file)] })
  })
  await page.goto('http://hestia.test/')
  return page
}

function consoleState() {
  return {
    running: true, localUrl: 'http://127.0.0.1:8081', urls: ['http://192.168.1.10:8081'], activePort: 8081,
    port: 8081, autoStart: false, ffmpeg: true, ffmpegPath: 'D:\\ffmpeg\\bin\\ffmpeg.exe',
    scanning: false, folders: 876, media: 1273,
    update: { current: '0.4.2', status: 'idle', progress: -1 },
    libs: [{ id: 'one', label: '迅雷下载', path: 'E:\\迅雷下载', enabled: true,
      scanning: false, files: 1273, dirs: 876, lastErr: '' }],
  }
}

test('unavailable libraries remain manageable and show recovery guidance', async t => {
  const state = consoleState()
  state.libs[0].lastErr = 'The system cannot find the file specified.'
  const page = await openConsole(t, (action, body) => {
    if (action === 'toggle') state.libs[0].enabled = body.enabled
    if (action === 'rescan') {
      state.libs[0].lastErr = ''
      state.libs[0].files = 1
      state.libs[0].dirs = 1
    }
    return state
  })
  const item = page.locator('.libitem')
  await item.waitFor()
  assert.match(await item.locator('.libstat').textContent(), /无法访问/)
  assert.match(await item.locator('.libstat').textContent(), /重新扫描/)
  assert.match(await item.locator('.libstat').textContent(), /移除/)
  assert.equal(await item.getAttribute('data-status'), 'error')
  assert.equal(await item.locator('.liben').isChecked(), true)
  assert.equal(await item.getByRole('button', { name: /更多操作/ }).isVisible(), true)

  await item.locator('.liben').uncheck()
  await page.waitForFunction(() => document.querySelector('.libstat')?.textContent.includes('已停用'))
  assert.equal(await item.getAttribute('data-status'), 'off')

  await item.locator('.liben').check()
  await page.waitForFunction(() => document.querySelector('.libstat')?.textContent.includes('无法访问'))
  await page.locator('#btnRescan').click()
  await page.getByText('1 个媒体', { exact: true }).waitFor()
  assert.equal(await item.getAttribute('data-status'), 'ready')
})

test('the console exposes addresses, management dialogs and settings at a narrow width', async t => {
  const state = consoleState()
  const requested = []
  const page = await openConsole(t, (action, body) => {
    requested.push({ action, body })
    switch (action) {
      case 'pickdir': return { path: 'E:\\电影' }
      case 'add': state.libs.push({ ...state.libs[0], id: 'two', label: '电影', path: body.path, files: 0, dirs: 0 }); break
      case 'remove': state.libs = state.libs.filter(item => item.id !== body.id); break
      case 'port': state.port = body.port; break
      case 'autostart': state.autoStart = body.enabled; break
      case 'update/check': state.update.status = 'current'; return state.update
    }
    return state
  }, { width: 430, height: 460 })

  await page.locator('.libitem').waitFor()
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 430)
  await page.getByRole('button', { name: '复制访问地址' }).click()
  assert.equal(await page.evaluate(() => window.copiedAddress), 'http://192.168.1.10:8081')
  await page.getByRole('button', { name: '手机访问' }).click()
  const access = page.getByRole('dialog', { name: '访问地址' })
  await access.getByText('http://127.0.0.1:8081', { exact: true }).waitFor()
  await access.getByText('http://192.168.1.10:8081', { exact: true }).waitFor()
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: '添加文件夹' }).click()
  const add = page.getByRole('dialog', { name: '添加文件夹' })
  await add.getByRole('button', { name: '浏览…' }).click()
  await page.waitForFunction(() => document.querySelector('#pathInput').value === 'E:\\电影')
  await add.getByRole('button', { name: '添加', exact: true }).click()
  await page.getByText('电影', { exact: true }).waitFor()

  await page.getByRole('button', { name: '电影 · 更多操作' }).click()
  await page.getByRole('button', { name: '移除媒体库' }).click()
  const confirm = page.getByRole('dialog', { name: '移除媒体库' })
  await confirm.waitFor()
  assert.equal(requested.some(request => request.action === 'remove'), false)
  await confirm.getByRole('button', { name: '移除', exact: true }).click()
  await page.getByText('电影', { exact: true }).waitFor({ state: 'detached' })
  assert.deepEqual(requested.find(request => request.action === 'remove').body, { id: 'two' })

  await page.getByRole('button', { name: '设置' }).click()
  const settings = page.getByRole('dialog', { name: '设置' })
  await settings.getByRole('spinbutton', { name: '服务端口' }).fill('8091')
  await settings.getByRole('button', { name: '应用' }).click()
  await settings.getByText('端口已应用').waitFor()
  assert.deepEqual(requested.find(request => request.action === 'port').body, { port: 8091 })
  await settings.getByRole('switch', { name: '开机自启' }).check()
  await settings.getByRole('button', { name: '检查更新' }).click()
  await settings.getByText('已是最新版本').waitFor()
  assert.deepEqual(requested.find(request => request.action === 'autostart').body, { enabled: true })
  await page.keyboard.press('Escape')
  await settings.waitFor({ state: 'hidden' })
  assert.equal(await page.getByRole('button', { name: '设置' }).evaluate(button => button === document.activeElement), true)
})

test('the service bar stays visible when the library list scrolls', async t => {
  const state = consoleState()
  state.libs = Array.from({ length: 24 }, (_, index) => ({ ...state.libs[0], id: `library-${index}`, label: `媒体库 ${index + 1}` }))
  const page = await openConsole(t, () => state)
  await page.getByText('媒体库 24', { exact: true }).waitFor({ state: 'attached' })
  const serviceBar = page.locator('.service-bar')
  const bounds = await serviceBar.boundingBox()
  await page.getByText('媒体库 24', { exact: true }).scrollIntoViewIfNeeded()
  assert.ok(await page.locator('.workspace').evaluate(element => element.scrollTop > 0))
  assert.deepEqual(await serviceBar.boundingBox(), bounds)
})

test('console settings save the locale', async t => {
  const state = consoleState()
  const page = await openConsole(t, (action, body) => {
    if (action === 'language') state.language = body.language
    return state
  }, { width: 430, height: 460 })

  await page.getByRole('button', { name: '设置' }).click()
  await page.getByRole('combobox', { name: '语言' }).selectOption('en')
  const settings = page.getByRole('dialog', { name: 'Settings' })
  await settings.getByRole('combobox', { name: 'Language' }).waitFor()
  assert.equal(await page.locator('html').getAttribute('lang'), 'en')
  await settings.getByRole('combobox', { name: 'Language' }).selectOption('ja')
  await page.getByRole('heading', { name: '設定' }).waitFor()
  await page.reload()
  await page.getByRole('heading', { name: 'メディアライブラリ' }).waitFor()
  assert.equal(await page.locator('html').getAttribute('lang'), 'ja')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 430)
})

test('cover generation starts only from the dialog and displays cache hits', async t => {
  const page = await openConsole(t, () => consoleState(), { width: 430, height: 460 })
  const requested = []
  let reply
  const response = new Promise(resolve => { reply = resolve })
  await page.route('**/gui/api/covers/generate', async route => {
    requested.push(route.request().method())
    await response
    await route.fulfill({
      contentType: 'application/x-ndjson',
      body: [
        { total: 4, processed: 0, generated: 0, cached: 0, skipped: 0, failed: 0, done: false },
        { total: 4, processed: 4, generated: 1, cached: 3, skipped: 0, failed: 0, done: true },
      ].map(snapshot => JSON.stringify(snapshot)).join('\n') + '\n',
    })
  })
  t.after(() => reply())

  await page.getByRole('button', { name: '生成封面' }).click({ timeout: 5000 })
  const dialog = page.getByRole('dialog', { name: '生成封面' })
  await dialog.waitFor()
  const progress = dialog.locator('progress')
  assert.equal(await progress.isVisible(), false)
  assert.deepEqual(requested, [])
  await dialog.getByRole('button', { name: '开始', exact: true }).click()
  await progress.waitFor()
  assert.equal(await progress.getAttribute('value'), null)
  await dialog.getByRole('button', { name: '停止', exact: true }).waitFor()
  reply()
  await dialog.getByText('封面生成完成').waitFor()
  assert.equal(await progress.getAttribute('value'), '4')
  await dialog.getByText('已缓存 3 项', { exact: false }).waitFor()
  assert.deepEqual(requested, ['POST'])
  await dialog.getByRole('button', { name: '关闭', exact: true }).last().click()
  await dialog.waitFor({ state: 'hidden' })
})

test('stopping cover generation cancels the request and keeps the dialog open', async t => {
  const page = await openConsole(t, () => consoleState())
  let entered, release
  const started = new Promise(resolve => { entered = resolve })
  const pending = new Promise(resolve => { release = resolve })
  await page.route('**/gui/api/covers/generate', async route => {
    entered()
    await pending
    await route.abort().catch(() => {})
  })

  try {
    await page.getByRole('button', { name: '生成封面' }).click()
    const dialog = page.getByRole('dialog', { name: '生成封面' })
    await dialog.getByRole('button', { name: '开始', exact: true }).click()
    await started
    await dialog.getByRole('button', { name: '停止', exact: true }).click()
    await dialog.getByText('已停止生成', { exact: false }).waitFor()
    assert.equal(await dialog.isVisible(), true)
    assert.equal(await dialog.getByRole('button', { name: '开始', exact: true }).isEnabled(), true)
  } finally { release() }
})

test('cover generation keeps running when dismissed and shows the result with failure details', async t => {
  const page = await openConsole(t, () => consoleState())
  let entered, finish
  const started = new Promise(resolve => { entered = resolve })
  const response = new Promise(resolve => { finish = resolve })
  await page.route('**/gui/api/covers/generate', async route => {
    entered()
    await response
    await route.fulfill({
      contentType: 'application/x-ndjson',
      body: JSON.stringify({
        total: 3, processed: 3, generated: 1, cached: 1, skipped: 0, failed: 1, done: true,
        issues: [{ path: 'D:\\媒体\\损坏.jpg', reason: 'imageRead' }],
      }) + '\n',
    })
  })

  try {
    await page.getByRole('button', { name: '生成封面' }).click()
    const dialog = page.getByRole('dialog', { name: '生成封面' })
    await dialog.getByRole('button', { name: '开始', exact: true }).click()
    await started
    await dialog.getByRole('button', { name: '收起', exact: true }).last().click({ timeout: 5000 })
    await dialog.waitFor({ state: 'hidden' })
    await page.getByText('封面正在后台生成', { exact: false }).waitFor()
    await page.getByRole('button', { name: '设置' }).click()
    finish()
    await page.waitForFunction(() => document.getElementById('coverStatus').textContent === '封面生成完成')
    assert.equal(await dialog.isVisible(), false)
    await page.keyboard.press('Escape')
    await dialog.waitFor()
    await dialog.getByText('未生成 1 项', { exact: true }).waitFor()
    await dialog.getByText('查看未生成原因').click()
    await dialog.getByText('损坏.jpg', { exact: true }).waitFor()
    await dialog.getByText('图片无法读取或解码').waitFor()
  } finally { finish() }
})
