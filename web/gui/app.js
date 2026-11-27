// Hestia 控制台：通过本机回环 API 读取和修改服务状态。
const $ = id => document.getElementById(id)
const count = new Intl.NumberFormat('zh-CN')
let state = null

async function api(path, body) {
  const response = await fetch('/gui/api/' + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })
  if (!response.ok) {
    let message = 'HTTP ' + response.status
    try { message = (await response.json()).error || message } catch {}
    throw new Error(message)
  }
  return response.json()
}

function text(el, value) {
  if (el.textContent !== value) el.textContent = value
}

let notice
function toast(message, error = false) {
  notice?.remove()
  const node = document.createElement('p')
  node.className = error ? 'toast err' : 'toast'
  node.setAttribute('role', error ? 'alert' : 'status')
  node.textContent = message
  const parent = document.querySelector('dialog[open]') || document.body
  parent.append(node)
  notice = node
  setTimeout(() => node.remove(), 3800)
}

function renderLibraryRow(row, library) {
  const status = !library.enabled ? 'off' : library.scanning ? 'scan' : library.lastErr ? 'error' : 'ready'
  row.dataset.status = status
  text(row.querySelector('.libname'), library.label)
  text(row.querySelector('.libpath'), library.path)
  const message = row.querySelector('.libstat')
  message.classList.toggle('error', status === 'error')
  message.title = status === 'error' ? library.lastErr : ''
  text(message, {
    off: '已停用',
    scan: `扫描中… ${count.format(library.files)} 项`,
    error: '文件夹无法访问，内容已隐藏。请检查路径或磁盘后重新扫描，也可以在更多操作中移除。',
    ready: '',
  }[status])
  text(row.querySelector('.media-count'), status === 'ready' ? `${count.format(library.files)} 个媒体` : '—')
  text(row.querySelector('.folder-count'), status === 'ready' ? `${count.format(library.dirs)} 个文件夹` : '')
  const toggle = row.querySelector('.liben')
  toggle.setAttribute('aria-label', `${library.label} · 启用媒体库`)
  if (!toggle.disabled) toggle.checked = library.enabled
  row.querySelector('.libmore').setAttribute('aria-label', `${library.label} · 更多操作`)
}

function renderLibraries(libraries) {
  const list = $('libList')
  const rows = new Map(Array.from(list.rows, row => [row.dataset.id, row]))
  for (const [index, library] of libraries.entries()) {
    const row = rows.get(library.id) || $('libraryRow').content.firstElementChild.cloneNode(true)
    row.dataset.id = library.id
    renderLibraryRow(row, library)
    if (list.rows[index] !== row) list.insertBefore(row, list.rows[index] || null)
    rows.delete(library.id)
  }
  for (const row of rows.values()) {
    if (row.contains(actionTrigger)) closeActions()
    row.remove()
  }
  $('libTable').hidden = libraries.length === 0
  $('emptyLibraries').hidden = libraries.length !== 0
}

function primaryAddress(s) { return s.urls[0] || s.localUrl }

function renderAddresses(s) {
  const list = $('accessAddresses')
  list.replaceChildren()
  const addresses = [
    ...(s.localUrl ? [['本机访问', s.localUrl]] : []),
    ...s.urls.map(url => ['局域网访问', url]),
  ]
  if (!addresses.length) list.textContent = '服务未运行。'
  for (const [label, address] of addresses) {
    const row = document.createElement('div')
    row.className = 'connection-row'
    const details = document.createElement('div')
    const name = document.createElement('p')
    name.textContent = label
    const url = document.createElement('code')
    url.textContent = address
    details.append(name, url)
    const copy = document.createElement('button')
    copy.type = 'button'
    copy.className = 'btn border'
    copy.textContent = '复制'
    copy.setAttribute('aria-label', `复制${label}地址 ${address}`)
    copy.onclick = () => copyAddress(address)
    row.append(details, copy)
    list.append(row)
  }
  $('portNote').hidden = !s.localUrl || s.activePort === s.port
  text($('portNote'), `配置端口 ${s.port}，实际使用 ${s.activePort}。`)
}

function render(s) {
  const previous = state
  state = s
  $('dot').classList.toggle('on', s.running)
  text($('stateText'), s.running ? '运行中' : '已停止')
  text($('btnToggle'), s.running ? '停止服务' : '启动服务')
  $('btnToggle').disabled = false
  $('btnOpen').disabled = !s.running
  $('btnAccess').disabled = !s.running
  const address = primaryAddress(s)
  text($('btnAccess'), s.urls.length ? '手机访问' : '本机访问')
  text($('urlText'), address ? address.replace(/^https?:\/\//, '') : '—')
  $('urlText').title = address || ''
  $('btnCopy').hidden = !address
  if ($('accessDialog').open && (s.localUrl !== previous?.localUrl || s.port !== previous?.port || s.activePort !== previous?.activePort ||
      s.urls.join('\n') !== previous?.urls.join('\n'))) renderAddresses(s)

  renderLibraries(s.libs)
  text($('statText'), `${count.format(s.media)} 个媒体 · ${count.format(s.folders)} 个文件夹${s.scanning ? ' · 扫描中' : ''}`)
  $('btnRescan').disabled = s.scanning || !s.libs.some(lib => lib.enabled)
  $('btnRescan').classList.toggle('busy', s.scanning)
  text($('btnRescan').querySelector('span'), s.scanning ? '扫描中…' : '重新扫描')

  if (s.port !== previous?.port) $('portInput').value = s.port
  if (!$('chkAuto').disabled) $('chkAuto').checked = s.autoStart
  text($('ffText'), s.ffmpeg ? `已就绪 · ${s.ffmpegPath}` : '未检测到 FFmpeg。播放需要转码时，请将它放到程序目录或加入 PATH。')
  const update = s.update
  text($('currentVersion'), 'v' + update.current)
  const updateMessages = {
    idle: '点击「检查更新」查看是否有新版本',
    checking: '正在从 GitHub 检查最新版本…',
    current: '已是最新版本',
    available: `发现新版本 v${update.latest}，建议下载安装`,
    downloading: `正在下载并校验新版本…${update.progress >= 0 ? ' ' + update.progress + '%' : ''}`,
    restarting: '校验完成，正在重启到新版本…',
    error: update.error || '更新失败',
  }
  text($('updateText'), updateMessages[update.status] || '更新状态未知')
  $('btnCheckUpdate').disabled = ['checking', 'downloading', 'restarting'].includes(update.status)
  $('btnInstallUpdate').hidden = update.status !== 'available'
  $('updateProgress').hidden = update.status !== 'downloading' || update.progress < 0
  $('updateProgress').value = Math.max(0, update.progress)
}

async function refresh() {
  const previous = state
  try {
    const next = await api('state')
    if (state === previous) render(next)
  } catch (error) { console.error(error) }
}

function act(action) {
  return async () => {
    try {
      const result = await action()
      if (result) render(result)
    } catch (error) { toast(error.message, true) }
  }
}

async function copyAddress(address) {
  try {
    await navigator.clipboard.writeText(address)
    toast('已复制访问地址')
  } catch {
    if (!$('accessDialog').open && state) {
      openAddresses()
    }
    toast('无法自动复制，请选中地址并按 Ctrl+C', true)
  }
}

function openAddresses() {
  renderAddresses(state)
  $('accessDialog').showModal()
}

for (const button of document.querySelectorAll('dialog [data-close]')) {
  button.onclick = () => button.closest('dialog').close()
}

$('btnToggle').onclick = act(() => api('toggle-service'))
$('btnOpen').onclick = act(() => api('open'))
$('btnLog').onclick = act(() => api('openlog'))
$('btnAccess').onclick = openAddresses
$('btnCopy').onclick = () => copyAddress(primaryAddress(state))
$('btnSettings').onclick = () => {
  if (state) $('portInput').value = state.port
  $('settingsDialog').showModal()
}
$('btnAddFolder').onclick = () => $('addDialog').showModal()

$('addForm').onsubmit = async event => {
  event.preventDefault()
  const path = $('pathInput').value.trim()
  if (!path) {
    $('pathInput').focus()
    toast('请先填写文件夹路径', true)
    return
  }
  $('btnAdd').disabled = true
  try {
    const result = await api('add', { path })
    $('pathInput').value = ''
    $('addDialog').close()
    render(result)
    toast('已添加，后台扫描中')
  } catch (error) { toast(error.message, true) }
  finally { $('btnAdd').disabled = false }
}

$('btnBrowse').onclick = async () => {
  const button = $('btnBrowse')
  button.disabled = true
  try {
    const result = await api('pickdir')
    if (result.path) $('pathInput').value = result.path
  } catch (error) { toast(error.message, true) }
  finally { button.disabled = false }
}

$('btnRescan').onclick = act(async () => {
  const result = await api('rescan')
  toast('已开始重新扫描')
  return result
})
$('portForm').onsubmit = event => {
  event.preventDefault()
  act(async () => {
    const result = await api('port', { port: $('portInput').valueAsNumber })
    toast('端口已应用')
    return result
  })()
}
$('chkAuto').onchange = async event => {
  const input = event.target
  input.disabled = true
  await act(() => api('autostart', { enabled: input.checked }))()
  input.disabled = false
  if (state) input.checked = state.autoStart
}

$('btnCheckUpdate').onclick = async () => {
  $('btnCheckUpdate').disabled = true
  text($('updateText'), '正在从 GitHub 检查最新版本…')
  try { await api('update/check') }
  catch (error) { toast(error.message, true) }
  finally {
    await refresh()
    if (!['checking', 'downloading', 'restarting'].includes(state?.update.status)) $('btnCheckUpdate').disabled = false
  }
}
$('btnInstallUpdate').onclick = async () => {
  const button = $('btnInstallUpdate')
  button.disabled = true
  try { await api('update/install'); await refresh() }
  catch (error) { toast(error.message, true) }
  finally { button.disabled = false }
}

const actions = $('libraryActions')
let actionTrigger = null
let removeID = ''

function closeActions() {
  if (actionTrigger) actions.hidePopover()
}

actions.addEventListener('beforetoggle', event => {
  if (event.newState === 'closed') {
    actionTrigger?.setAttribute('aria-expanded', 'false')
    actionTrigger = null
  }
})

function openRemove(id) {
  const library = state?.libs.find(item => item.id === id)
  if (!library) return
  removeID = id
  text($('removeDescription'), `将「${library.label}」从媒体库移除？磁盘中的文件会保留。`)
  $('removeDialog').showModal()
}

$('libList').addEventListener('click', event => {
  const button = event.target.closest('.libmore')
  if (!button) return
  const alreadyOpen = actionTrigger === button
  closeActions()
  if (alreadyOpen) return
  if (!actions.showPopover) {
    openRemove(button.closest('.libitem').dataset.id)
    return
  }
  actionTrigger = button
  button.setAttribute('aria-expanded', 'true')
  actions.showPopover()
  const anchor = button.getBoundingClientRect()
  const { width, height } = actions.getBoundingClientRect()
  const top = anchor.bottom + height + 4 <= window.innerHeight ? anchor.bottom + 4 : anchor.top - height - 4
  actions.style.left = Math.max(4, anchor.right - width) + 'px'
  actions.style.top = Math.max(4, top) + 'px'
  $('btnRemove').focus({ preventScroll: true })
})
$('btnRemove').onclick = () => {
  const button = actionTrigger
  const id = button?.closest('.libitem').dataset.id
  closeActions()
  button?.focus({ preventScroll: true })
  openRemove(id)
}
$('btnConfirmRemove').onclick = async () => {
  $('btnConfirmRemove').disabled = true
  try {
    const result = await api('remove', { id: removeID })
    $('removeDialog').close()
    render(result)
    $('btnAddFolder').focus()
    toast('已移除媒体库')
  } catch (error) { toast(error.message, true) }
  finally { $('btnConfirmRemove').disabled = false }
}

$('libList').addEventListener('change', async event => {
  const toggle = event.target.closest('.liben')
  if (!toggle) return
  const { id } = toggle.closest('.libitem').dataset
  toggle.disabled = true
  await act(() => api('toggle', { id, enabled: toggle.checked }))()
  toggle.disabled = false
  const library = state?.libs.find(item => item.id === id)
  if (library) toggle.checked = library.enabled
})
window.addEventListener('resize', closeActions)
document.querySelector('.workspace').addEventListener('scroll', closeActions)

// The window can render its loading state while the backend is still starting.
window.guiReady?.()

async function pollState() {
  if (!state || !document.hidden) await refresh()
  const update = state?.update.status
  setTimeout(pollState, !state || state.scanning || update === 'checking' || update === 'downloading' ? 1500 : 5000)
}
pollState()
