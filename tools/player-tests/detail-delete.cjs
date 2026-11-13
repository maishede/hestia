const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')
const { chromium } = require('playwright-core')

const root = process.env.HESTIA_DETAIL_DELETE_FIXTURE
const base = process.env.HESTIA_TEST_BASE || 'http://127.0.0.1:18100'
const executablePath = process.env.HESTIA_TEST_CHROME || 'C:/Program Files/Google/Chrome/Application/chrome.exe'

async function fixtureID(name, kind) {
  const { items } = await (await fetch(`${base}/api/search?q=${encodeURIComponent(name)}`)).json()
  const item = items.find(item => item.name === name && item.kind === kind)
  assert.ok(item, `missing ${name} fixture`)
  return item.id
}

async function main() {
  assert.ok(root && path.basename(root) === 'hestia-detail-delete-fixture', 'an isolated deletion fixture is required')
  assert.equal(new URL(base).hostname, '127.0.0.1')
  const source = path.join(root, 'DeleteFixture')
  assert.equal(await fs.readFile(path.join(source, 'fixture.txt'), 'utf8'), 'Hestia deletion test fixture')
  const videoID = await fixtureID('delete-sample', 'video')
  const imageID = await fixtureID('detail-image', 'image')
  const browser = await chromium.launch({ executablePath, args: ['--autoplay-policy=no-user-gesture-required'] })
  try {
    const page = await browser.newPage()
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`${base}/#/image/${imageID}`)
    await page.waitForFunction(() => document.querySelector('#img')?.naturalWidth === 480)
    assert.ok((await page.locator('#img').getAttribute('src')).endsWith('?w=480'))
    await page.getByRole('button', { name: '加载原图' }).click()
    await page.waitForFunction(() => document.querySelector('#img')?.naturalWidth > 480)
    await page.getByRole('button', { name: '删除所在文件夹媒体' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByText(/子文件夹或链接/).waitFor()
    await dialog.getByRole('button', { name: '取消' }).click()
    assert.ok(await fs.stat(path.join(source, 'detail-image.png')))

    await page.goto(`${base}/#/play/${videoID}`)
    await page.waitForFunction(() => document.querySelector('video')?.currentTime > 0.2)
    await page.getByRole('button', { name: '删除所在文件夹媒体' }).click()
    await dialog.getByRole('textbox', { name: '删除原因' }).fill('看完')
    await dialog.getByText('改名为「DeleteFixture-看完」').waitFor()
    const response = page.waitForResponse(r => r.url().endsWith('/delete') && r.request().method() === 'POST')
    await dialog.getByRole('button', { name: '确认删除' }).click()
    assert.equal((await response).status(), 200)
    await page.getByRole('heading', { name: 'DeleteFixture-看完' }).waitFor()
    const renamed = source + '-看完'
    for (const file of ['delete-sample.mp4', 'detail-image.png', 'delete-sample.srt']) {
      await assert.rejects(fs.stat(path.join(renamed, file)), { code: 'ENOENT' })
    }
    for (const file of ['fixture.txt', '子目录/keep.mp4']) assert.ok(await fs.stat(path.join(renamed, file)))
    assert.deepEqual(errors, [])
    console.log('PASS image preview and original, detail-page confirmation, and deletion while playing')
  } finally {
    await browser.close()
  }
}

main().catch(error => { console.error(error); process.exitCode = 1 })
