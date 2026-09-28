// Hestia 控制台（桌面 GUI）逻辑：轮询状态 + 调用本机回环 API
const $ = id => document.getElementById(id)
let state = null

async function api(path, body) {
  // 与后端约定：全部走 POST + JSON（无参数发空对象），避免 GET/POST 路由错配
  const r = await fetch('/gui/api/' + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })
  if (!r.ok) {
    let msg = 'HTTP ' + r.status
    try { msg = (await r.json()).error || msg } catch {}
    throw new Error(msg)
  }
  const ct = r.headers.get('content-type') || ''
  return ct.includes('json') ? r.json() : null
}

function toast(msg, err) {
  const el = document.createElement('div')
  el.className = 'toast' + (err ? ' err' : '')
  el.textContent = msg
  document.body.appendChild(el)
  setTimeout(() => { el.style.opacity = '0'; el.style.transition = 'opacity .25s'; setTimeout(() => el.remove(), 260) }, 2400)
}

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

let lastLibsJson = ''

function render(s) {
  state = s
  // 状态卡
  $('dot').className = 'dot' + (s.running ? ' on' : '')
  $('stateText').textContent = s.running ? '服务运行中' : '服务已停止'
  $('btnToggle').textContent = s.running ? '关闭服务' : '启动服务'
  $('btnToggle').classList.toggle('primary', !s.running)
  $('urlText').textContent = s.running && s.urls.length
    ? '手机浏览器访问：' + s.urls.join('  或  ')
    : '服务未启动，手机无法访问'
  // 媒体库：数据未变化时跳过重建，避免打断交互（如两连击确认）
  const libsJson = JSON.stringify(s.libs)
  if (libsJson !== lastLibsJson) {
    lastLibsJson = libsJson
    const list = $('libList')
    list.innerHTML = s.libs.length ? s.libs.map(l => `
      <div class="libitem ${l.enabled ? '' : 'off'}" data-id="${l.id}">
        <span class="libdot ${l.scanning ? 'scan' : (l.enabled ? '' : 'off')}"></span>
        <div class="libmain">
          <div class="libname">${esc(l.label)}</div>
          <div class="libpath">${esc(l.path)}</div>
          <div class="libstat">${l.scanning ? '扫描中… ' + l.files + ' 项' : (l.lastErr ? '错误：' + esc(l.lastErr) : l.files + ' 文件 · ' + l.dirs + ' 文件夹')}</div>
        </div>
        <label class="switch" title="启用/停用"><input type="checkbox" class="liben" ${l.enabled ? 'checked' : ''}><i></i></label>
        <button class="btn danger libdel">移除</button>
      </div>`).join('')
      : '<div class="muted" style="padding:14px 4px">还没有媒体库，在上方输入视频文件夹路径添加</div>'
  }
  $('statText').textContent = s.scanning ? '后台扫描中…' : `索引：${s.folders} 文件夹 · ${s.media} 媒体`
  // 设置
  if (document.activeElement !== $('portInput')) $('portInput').value = s.port
  $('chkAuto').checked = s.autoStart
  $('ffText').textContent = s.ffmpeg
    ? 'ffmpeg 已就绪（' + s.ffmpegPath + '）'
    : '未检测到 ffmpeg：HEVC/MKV 等格式无法转码，建议放到程序同目录或加入 PATH'
}

async function refresh() {
  try { render(await api('state')) } catch (e) { console.error(e) }
}

// 页面首次渲染完成：通知 Go 显示窗口（消除启动白屏）
let readySignaled = false
function signalReady() {
  if (readySignaled) return
  readySignaled = true
  try { window.guiReady() } catch {}
}

function act(fn) {
  return async () => {
    try {
      const s = await fn()
      if (s) render(s)
    } catch (e) { toast(e.message, true) }
  }
}

$('btnToggle').onclick = act(() => api('toggle-service'))
$('btnOpen').onclick = act(() => { api('open'); return null })
$('btnLog').onclick = act(() => { api('openlog'); return null })
$('btnAdd').onclick = act(() => {
  const path = $('pathInput').value.trim()
  if (!path) { toast('请先填写路径或点「浏览」选择', true); return Promise.resolve(null) }
  return api('add', { path }).then(s => { $('pathInput').value = ''; toast('已添加，后台扫描中'); return s })
})
$('btnBrowse').onclick = async () => {
  const btn = $('btnBrowse')
  btn.disabled = true
  try {
    const r = await api('pickdir')
    if (r.path) $('pathInput').value = r.path
  } catch (e) { toast(e.message, true) }
  btn.disabled = false
}
$('btnRescan').onclick = act(() => api('rescan').then(s => { toast('已开始重新扫描'); return s }))
$('btnPort').onclick = act(() => api('port', { port: parseInt($('portInput').value, 10) }).then(s => { toast('端口已应用'); return s }))
$('chkAuto').onchange = e => act(() => api('autostart', { enabled: e.target.checked }))()

// 移除：两连击确认（第一次变为「确认?」，2.5 秒内再点生效）
let removeTimer = null
$('libList').addEventListener('click', e => {
  const del = e.target.closest('.libdel')
  if (!del) return
  const item = del.closest('.libitem')
  if (del.dataset.confirm) {
    clearTimeout(removeTimer)
    act(() => api('remove', { id: item.dataset.id }))()
    return
  }
  del.dataset.confirm = '1'
  del.textContent = '确认?'
  removeTimer = setTimeout(() => { del.textContent = '移除'; delete del.dataset.confirm }, 2500)
})
$('libList').addEventListener('change', e => {
  const en = e.target.closest('.liben')
  if (en) {
    const id = en.closest('.libitem').dataset.id
    act(() => api('toggle', { id, enabled: en.checked }))()
  }
})
$('pathInput').addEventListener('keydown', e => { if (e.key === 'Enter') $('btnAdd').click() })

refresh()
setInterval(refresh, 1500)
// 首个 state 到手即视为渲染完成
const _firstRender = setInterval(() => { if (state) { clearInterval(_firstRender); signalReady() } }, 60)
setTimeout(signalReady, 3000) // 兜底
