// 通用工具：格式化、DOM、toast、图标、排序偏好
export function fmtSize(n) {
  if (!n && n !== 0) return ''
  if (n < 1024) return n + ' B'
  if (n < 1024 ** 2) return (n / 1024).toFixed(0) + ' KB'
  if (n < 1024 ** 3) return (n / 1024 ** 2).toFixed(1) + ' MB'
  return (n / 1024 ** 3).toFixed(2) + ' GB'
}

export function fmtDur(sec) {
  if (!sec || sec <= 0 || !isFinite(sec)) return ''
  sec = Math.floor(sec)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const mm = String(m).padStart(2, '0')
  const ss = String(s).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${m}:${ss}`
}

export function fmtDate(ms) {
  if (!ms) return ''
  const d = new Date(ms * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function clamp(v, lo, hi) { return Math.min(hi, Math.max(lo, v)) }

export function debounce(fn, ms) {
  let t
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms) }
}

export function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

// ---------- Toast ----------
export function toast(msg, type = 'info', ms = 2600) {
  const root = document.getElementById('toasts')
  const el = document.createElement('div')
  el.className = 'toast' + (type === 'err' ? ' err' : '')
  el.textContent = msg
  root.appendChild(el)
  setTimeout(() => { el.style.opacity = '0'; el.style.transition = 'opacity .3s'; setTimeout(() => el.remove(), 320) }, ms)
}

// ---------- 图标（stroke 风格） ----------
const ICONS = {
  folder: '<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>',
  film: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4"/>',
  image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="m21 15-4.5-4.5L7 20"/>',
  play: '<path d="M8 5.5v13a.6.6 0 0 0 .9.5l10.6-6.5a.6.6 0 0 0 0-1L8.9 5a.6.6 0 0 0-.9.5z" fill="currentColor" stroke="none"/>',
  pause: '<path d="M7 5h3.4v14H7zM13.6 5H17v14h-3.4z" fill="currentColor" stroke="none"/>',
  back: '<path d="m15 18-6-6 6-6"/>',
  fwd: '<path d="m9 18 6-6-6-6"/>',
  left10: '<path d="m11 17-5-5 5-5"/><path d="M18 6v12"/>',
  vol: '<path d="M11 5 6 9H3v6h3l5 4z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9.5 9.5 0 0 1 0 13"/>',
  volMute: '<path d="M11 5 6 9H3v6h3l5 4z"/><path d="m16 9 5 6M21 9l-5 6"/>',
  cc: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M10.5 10.2a2.6 2.6 0 1 0 0 3.6M16 10.2a2.6 2.6 0 1 0 0 3.6"/>',
  expand: '<path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M6 6l1 15h10l1-15"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-2.6-6.4L21 8"/><path d="M21 3v5h-5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20.5 20.5-4.2-4.2"/>',
}

export function icon(name, size = 22, sw = 2) {
  return `<svg viewBox="0 0 24 24" width="${size}" height="${size}" fill="none" stroke="currentColor" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round">${ICONS[name] || ''}</svg>`
}

// ---------- 排序偏好 ----------
const SORT_KEY = 'hestia.sort'
export function getSort(def = { key: 'name', order: 'asc' }) {
  try {
    const v = JSON.parse(localStorage.getItem(SORT_KEY))
    if (v && v.key && v.order) return v
  } catch {}
  return def
}
export function setSort(v) { localStorage.setItem(SORT_KEY, JSON.stringify(v)) }

// 排序选择器（名称/日期/大小 × 正逆序），返回 DOM 元素
export function sortBarEl(current, fields, onChange) {
  const wrap = document.createElement('div')
  wrap.className = 'sortbar'
  const opts = fields.map(f => `<option value="${f.key}" ${f.key === current.key ? 'selected' : ''}>${f.label}</option>`).join('')
  wrap.innerHTML = `
    <select class="sort-sel" title="排序方式">${opts}</select>
    <button class="sort-dir" title="${current.order === 'asc' ? '正序（点击切换倒序）' : '倒序（点击切换正序）'}">${current.order === 'asc' ? '↑' : '↓'}</button>`
  const sel = wrap.querySelector('select')
  const dir = wrap.querySelector('.sort-dir')
  const fire = () => onChange({ key: sel.value, order: dir.textContent.trim() === '↑' ? 'asc' : 'desc' })
  sel.addEventListener('change', fire)
  dir.addEventListener('click', () => {
    const asc = dir.textContent.trim() === '↑'
    dir.textContent = asc ? '↓' : '↑'
    dir.title = asc ? '倒序（点击切换正序）' : '正序（点击切换倒序）'
    fire()
  })
  return wrap
}

// 骨架屏
export function skeleton(n, wide = false) {
  let out = ''
  for (let i = 0; i < n; i++) {
    out += `<div class="skeleton-card ${wide ? 'wide' : ''}"><div class="sk-cover sk"></div><div class="card-body" style="padding:10px 12px 12px"><div class="sk sk-body"></div></div></div>`
  }
  return out
}

export function emptyHTML(title, hint = '', ico = 'folder') {
  return `<div class="empty-state">${icon(ico, 46)}<p>${esc(title)}</p>${hint ? `<p class="hint">${esc(hint)}</p>` : ''}</div>`
}
