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
