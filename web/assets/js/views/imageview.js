// 图片查看器：大图 + 左右切换（同文件夹图片）
import { api } from '../api.js'
import { icon, esc } from '../util.js'

export async function ImageView(app, id) {
  document.body.classList.add('player-mode')
  const meta = await api(`/api/media/${id}`)

  const root = document.createElement('div')
  root.className = 'image-view'
  root.innerHTML = `
    <div class="iv-bar">
      <button class="pbtn" id="iv-back" title="返回">${icon('back', 21)}</button>
      <div class="iv-title" id="iv-title">${esc(meta.name)}</div>
      <span style="width:40px"></span>
    </div>
    <div class="iv-img-wrap" id="wrap">
      <img id="img" alt="">
    </div>
    <button class="iv-arrow left" id="iv-prev">${icon('back', 20)}</button>
    <button class="iv-arrow right" id="iv-next">${icon('fwd', 20)}</button>`
  app.appendChild(root)

  const img = root.querySelector('#img')
  const title = root.querySelector('#iv-title')

  let cur = id
  const setSrc = (m) => {
    if (!m) return
    cur = m.id
    title.textContent = m.name
    img.classList.add('switching')
    const pre = new Image()
    pre.onload = () => { img.src = pre.src; img.classList.remove('switching') }
    pre.onerror = () => img.classList.remove('switching')
    pre.src = `/api/media/${m.id}/image`
  }
  setSrc(meta)
  // 预加载相邻
  if (meta.prev) new Image().src = `/api/media/${meta.prev.id}/image`
  if (meta.next) new Image().src = `/api/media/${meta.next.id}/image`

  const go = async m => { if (m) setSrc(m) }
  const prevBtn = root.querySelector('#iv-prev')
  const nextBtn = root.querySelector('#iv-next')
  if (!meta.prev) prevBtn.style.display = 'none'
  if (!meta.next) nextBtn.style.display = 'none'

  root.querySelector('#iv-back').addEventListener('click', () => {
    if (history.length > 1) history.back()
    else location.hash = '#/'
  })
  const loadSib = async dir => {
    const m = await api(`/api/media/${cur}`)
    go(dir === 'prev' ? m.prev : m.next)
  }
  prevBtn.addEventListener('click', () => loadSib('prev'))
  nextBtn.addEventListener('click', () => loadSib('next'))

  const onKey = e => {
    if (e.key === 'ArrowLeft') loadSib('prev')
    else if (e.key === 'ArrowRight') loadSib('next')
    else if (e.key === 'Escape') { if (history.length > 1) history.back(); else location.hash = '#/' }
  }
  document.addEventListener('keydown', onKey)

  // 触屏左右滑动切换 + 双击/滚轮缩放
  const wrap = root.querySelector('#wrap')
  let sx = null
  wrap.addEventListener('pointerdown', e => { sx = e.clientX })
  wrap.addEventListener('pointerup', e => {
    if (sx == null) return
    const dx = e.clientX - sx
    sx = null
    if (Math.abs(dx) > 60) loadSib(dx > 0 ? 'prev' : 'next')
  })

  let scale = 1
  const applyZoom = (factor, ox, oy) => {
    scale = Math.min(4, Math.max(1, factor))
    if (scale <= 1.01) {
      img.style.transform = ''
      img.classList.remove('zoomed')
    } else {
      img.style.transformOrigin = `${ox}px ${oy}px`
      img.style.transform = `scale(${scale})`
      img.classList.add('zoomed')
    }
  }
  let lastTap = 0
  img.addEventListener('pointerup', e => {
    const now = performance.now()
    if (now - lastTap < 300) {
      const r = img.getBoundingClientRect()
      applyZoom(scale > 1.01 ? 1 : 2.5, e.clientX - r.left, e.clientY - r.top)
      lastTap = 0
    } else {
      lastTap = now
    }
  })
  img.addEventListener('dblclick', e => {
    e.preventDefault()
    const r = img.getBoundingClientRect()
    applyZoom(scale > 1.01 ? 1 : 2.5, e.clientX - r.left, e.clientY - r.top)
  })
  wrap.addEventListener('wheel', e => {
    if (!e.ctrlKey && Math.abs(e.deltaY) < 2) return
    e.preventDefault()
    const r = img.getBoundingClientRect()
    applyZoom(scale * (e.deltaY < 0 ? 1.2 : 0.83), e.clientX - r.left, e.clientY - r.top)
  }, { passive: false })

  return {
    destroy() {
      document.removeEventListener('keydown', onKey)
      root.remove()
    },
  }
}
