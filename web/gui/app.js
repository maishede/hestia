// Hestia 控制台：通过本机回环 API 读取和修改服务状态。
const $ = id => document.getElementById(id)
let state = null
setConsoleLanguage(window.hestiaConsoleLanguage)

async function guiRequest(path, body, signal) {
  const response = await fetch('/gui/api/' + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
    signal,
  })
  if (!response.ok) {
    let failure = {}
    try { failure = await response.json() } catch {}
    const code = Object.hasOwn(consoleMessages, failure?.code) ? failure.code : 'operationFailed'
    throw new Error(t(code, { detail: failure?.error || 'HTTP ' + response.status }))
  }
  return response
}

async function api(path, body) {
  return (await guiRequest(path, body)).json()
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
    off: t('disabled'),
    scan: t('scanningItems', { number: count.format(library.files) }),
    error: t('inaccessible'),
    ready: '',
  }[status])
  text(row.querySelector('.media-count'), status === 'ready' ? quantity(library.files, 'media') : '—')
  text(row.querySelector('.folder-count'), status === 'ready' ? quantity(library.dirs, 'folder') : '')
  const toggle = row.querySelector('.liben')
  toggle.setAttribute('aria-label', t('enableLibrary', { name: library.label }))
  if (!toggle.disabled) toggle.checked = library.enabled
  row.querySelector('.libmore').setAttribute('aria-label', t('moreActions', { name: library.label }))
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
    ...(s.localUrl ? [[t('localAccess'), s.localUrl]] : []),
    ...s.urls.map(url => [t('lanAccess'), url]),
  ]
  if (!addresses.length) list.textContent = t('serverNotRunning')
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
    copy.textContent = t('copy')
    copy.setAttribute('aria-label', t('copyFor', { label, address }))
    copy.onclick = () => copyAddress(address)
    row.append(details, copy)
    list.append(row)
  }
  $('portNote').hidden = !s.localUrl || s.activePort === s.port
  text($('portNote'), t('portNote', { configured: s.port, active: s.activePort }))
}

function render(s) {
  const previous = state
  state = s
  const localeChanged = setConsoleLanguage(s.language)
  if (localeChanged) window.guiLocaleChanged?.()
  $('dot').classList.toggle('on', s.running)
  text($('stateText'), t(s.running ? 'running' : 'stopped'))
  text($('btnToggle'), t(s.running ? 'stopServer' : 'startServer'))
  $('btnToggle').disabled = false
  $('btnOpen').disabled = !s.running
  $('btnAccess').disabled = !s.running
  const address = primaryAddress(s)
  text($('btnAccess'), t(s.urls.length ? 'mobileAccess' : 'localAccess'))
  text($('urlText'), address ? address.replace(/^https?:\/\//, '') : '—')
  $('urlText').title = address || ''
  $('btnCopy').hidden = !address
  if ($('accessDialog').open && (localeChanged || s.localUrl !== previous?.localUrl || s.port !== previous?.port || s.activePort !== previous?.activePort ||
      s.urls.join('\n') !== previous?.urls.join('\n'))) renderAddresses(s)
  if (localeChanged && $('removeDialog').open) renderRemoveDescription(s.libs.find(item => item.id === removeID))

  renderLibraries(s.libs)
  const summary = [quantity(s.media, 'media'), quantity(s.folders, 'folder')]
  if (s.scanning) summary.push(t('scanning'))
  text($('statText'), summary.join(' · '))
  const hasLibrary = s.libs.some(lib => lib.enabled)
  $('btnRescan').disabled = s.scanning || !hasLibrary
  $('btnRescan').classList.toggle('busy', s.scanning)
  text($('btnRescan').querySelector('span'), t(s.scanning ? 'scanningEllipsis' : 'rescan'))
  renderCoverDialog()

  if (s.port !== previous?.port) $('portInput').value = s.port
  if (!$('chkAuto').disabled) $('chkAuto').checked = s.autoStart
  if (!previous) $('languageSelect').disabled = false
  if (!$('languageSelect').disabled) $('languageSelect').value = consoleLanguages[localeIndex]
  text($('ffText'), s.ffmpeg ? t('ffmpegReady', { path: s.ffmpegPath }) : t('ffmpegMissing'))
  const update = s.update
  text($('currentVersion'), 'v' + update.current)
  const updateMessages = {
    idle: t('updateIdle'),
    checking: t('updateChecking'),
    current: t('updateCurrent'),
    available: t('updateAvailable', { version: update.latest }),
    downloading: t('updateDownloading') + (update.progress >= 0 ? ' ' + count.format(update.progress) + '%' : ''),
    restarting: t('updateRestarting'),
    error: t('updateFailed') + (update.error ? ' · ' + update.error : ''),
  }
  text($('updateText'), updateMessages[update.status] || t('updateUnknown'))
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
    toast(t('copied'))
  } catch {
    if (!$('accessDialog').open && state) {
      openAddresses()
    }
    toast(t('copyFailed'), true)
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

let coverRequest = null
let coverSnapshot = null
let coverPhase = ''
let coverError = ''
let coverIssues = []
let coverIssueLocale = -1
let coverResultPending = false

function renderCoverIssues() {
  const details = $('coverIssues')
  const list = $('coverIssueList')
  details.hidden = coverIssues.length === 0
  if (!details.open) return
  if (coverIssueLocale !== localeIndex) {
    list.replaceChildren()
    coverIssueLocale = localeIndex
  }
  const reasonKeys = {
    sourceMissing: 'coverIssueSource',
    imageRead: 'coverIssueImage',
    videoFrame: 'coverIssueVideo',
    cacheWrite: 'coverIssueCache',
    other: 'coverIssueOther',
  }
  const next = document.createDocumentFragment()
  for (const issue of coverIssues.slice(list.children.length)) {
    const li = document.createElement('li')
    const name = document.createElement('strong')
    const reason = document.createElement('span')
    name.textContent = issue.path.split(/[\\/]/).at(-1) || issue.path
    name.title = issue.path
    reason.className = 'muted'
    reason.textContent = t(reasonKeys[issue.reason] || 'coverIssueOther')
    li.append(name, reason)
    next.append(li)
  }
  list.append(next)
}

function resetCoverResult() {
  coverSnapshot = null
  coverPhase = coverError = ''
  coverIssues = []
  $('coverIssues').open = false
  $('coverIssueList').replaceChildren()
}

function renderCoverDialog() {
  const active = coverRequest !== null
  const trigger = $('btnGenerateCovers')
  const hasLibrary = state?.libs.some(lib => lib.enabled)
  const progress = $('coverProgress')
  const status = $('coverStatus')
  const processedText = $('coverProcessed')
  const counts = $('coverCounts')
  const phase = coverPhase || (state?.scanning ? 'coverWaitForScan' : '')
  const triggerLabel = t(active ? 'coverView' : 'generateCovers')
  const percent = active && coverSnapshot?.total
    ? coverSnapshot.processed / coverSnapshot.total : null
  trigger.disabled = !active && !hasLibrary
  trigger.setAttribute('aria-label', triggerLabel)
  text(trigger, percent === null ? triggerLabel : triggerLabel + ' · ' + percent.toLocaleString(consoleLanguages[localeIndex], { style: 'percent' }))
  $('coverIntro').hidden = active || !!coverSnapshot
  text(status, coverError || (phase ? t(phase) : ''))
  status.hidden = !status.textContent
  progress.hidden = !active && !coverSnapshot
  if (coverSnapshot) {
    const { total, processed, generated, cached, skipped, failed } = coverSnapshot
    progress.max = total || 1
    progress.value = total ? processed : 1
    text(processedText, t('coverProcessed', { done: count.format(processed), total: count.format(total) }))
    const details = [
      t('coverGenerated', { number: count.format(generated) }),
      t('coverCached', { number: count.format(cached) }),
    ]
    if (skipped) details.push(t('coverSkipped', { number: count.format(skipped) }))
    if (failed) details.push(t('coverFailed', { number: count.format(failed) }))
    counts.replaceChildren(...details.map(value => {
      const item = document.createElement('span')
      item.textContent = value
      return item
    }))
  } else {
    progress.removeAttribute('value')
  }
  processedText.hidden = !coverSnapshot
  counts.hidden = !coverSnapshot
  renderCoverIssues()
  const dismissLabel = t(active ? 'coverHide' : 'close')
  $('btnCoverX').setAttribute('aria-label', dismissLabel)
  $('btnCoverX').title = dismissLabel
  $('btnCoverClose').hidden = active
  $('btnCoverStop').hidden = !active
  $('btnCoverStop').disabled = active && coverRequest.signal.aborted
  $('btnCoverHide').hidden = !active
  $('btnCoverStart').hidden = active || !!coverSnapshot?.done
  $('btnCoverStart').disabled = !!state?.scanning || !hasLibrary
}

$('btnGenerateCovers').onclick = () => {
  if (!coverRequest && !coverResultPending) resetCoverResult()
  coverResultPending = false
  renderCoverDialog()
  $('coverDialog').showModal()
  if (coverRequest) $('btnCoverHide').focus()
}

function hideCoverDialog() {
  $('coverDialog').close()
  if (coverRequest) toast(t('coverInBackground'))
}

function showCoverResult() {
  if (!coverResultPending || document.hidden || document.querySelector('dialog[open]')) return
  coverResultPending = false
  $('coverDialog').showModal()
  $('btnCoverClose').focus()
}

document.addEventListener('close', showCoverResult, true)
document.addEventListener('visibilitychange', showCoverResult)
window.addEventListener('focus', showCoverResult)
$('btnCoverX').onclick = hideCoverDialog
$('btnCoverHide').onclick = hideCoverDialog
$('coverIssues').addEventListener('toggle', renderCoverIssues)
$('btnCoverStop').onclick = () => {
  coverRequest?.abort()
  renderCoverDialog()
}
$('coverDialog').addEventListener('cancel', event => {
  if (coverRequest) {
    event.preventDefault()
    hideCoverDialog()
  }
})

$('btnCoverStart').onclick = async () => {
  if (coverRequest) return
  const controller = new AbortController()
  resetCoverResult()
  coverRequest = controller
  coverPhase = 'coverPreparing'
  renderCoverDialog()
  $('btnCoverHide').focus()
  try {
    const response = await guiRequest('covers/generate', {}, controller.signal)
    const reader = response.body.pipeThrough(new TextDecoderStream()).getReader()
    let buffer = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      const lines = (buffer + value).split('\n')
      buffer = lines.pop()
      for (const line of lines) {
        if (!line) continue
        const { issues = [], ...progress } = JSON.parse(line)
        coverIssues.push(...issues)
        coverSnapshot = progress
        coverPhase = coverSnapshot.done ? (coverSnapshot.total ? 'coverDone' : 'coverEmpty') : 'coverGenerating'
      }
      renderCoverDialog()
    }
    if (!coverSnapshot?.done) throw new Error(t('coverInterrupted'))
  } catch (error) {
    coverPhase = controller.signal.aborted ? 'coverStopped' : 'coverInterrupted'
    if (!controller.signal.aborted) coverError = error.message
  } finally {
    controller.abort()
    coverRequest = null
    renderCoverDialog()
    if ($('coverDialog').open) $('btnCoverClose').focus()
    else if (coverPhase !== 'coverStopped') {
      coverResultPending = true
      showCoverResult()
    }
  }
}

$('addForm').onsubmit = async event => {
  event.preventDefault()
  const path = $('pathInput').value.trim()
  if (!path) {
    $('pathInput').focus()
    toast(t('enterFolder'), true)
    return
  }
  $('btnAdd').disabled = true
  try {
    const result = await api('add', { path })
    $('pathInput').value = ''
    $('addDialog').close()
    render(result)
    toast(t('folderAdded'))
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
  toast(t('rescanStarted'))
  return result
})
$('portForm').onsubmit = event => {
  event.preventDefault()
  act(async () => {
    const result = await api('port', { port: $('portInput').valueAsNumber })
    toast(t('portApplied'))
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

$('languageSelect').onchange = async event => {
  const select = event.target
  select.disabled = true
  await act(() => api('language', { language: select.value }))()
  select.disabled = false
  select.value = consoleLanguages[localeIndex]
}

$('btnCheckUpdate').onclick = async () => {
  $('btnCheckUpdate').disabled = true
  text($('updateText'), t('updateChecking'))
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
  renderRemoveDescription(library)
  $('removeDialog').showModal()
}

function renderRemoveDescription(library) {
  text($('removeDescription'), library ? t('confirmRemoval', { name: library.label }) : t('missingLibrary'))
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
    toast(t('libraryRemoved'))
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
