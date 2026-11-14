// 图片详情：复用列表缩略图，需要时加载原图。
import { api } from '../api.js'
import { mountFolderDelete } from '../delete-dialog.js'
import { returnHomeHref, withHome } from '../navigation.js'
import { icon, esc } from '../util.js'

export async function ImageView(app, id, isCurrent = () => true) {
  app.innerHTML = '<div class="empty-state"><p>正在加载图片…</p></div>'
  const meta = await api(`/api/media/${id}`)
  if (!isCurrent()) return null
  const root = document.createElement('section')
  root.className = 'media-detail'
  const home = returnHomeHref()
  const folder = meta.folderId ? withHome(`#/folder/${meta.folderId}`, home) : ''
  root.innerHTML = `
    <div class="detail-header">
      <a class="btn ghost sm" href="${esc(home)}">返回主页</a>
      ${folder ? `<a class="btn ghost sm" href="${esc(folder)}">返回文件夹</a>` : ''}
      <div class="detail-title">
        <h1 id="iv-title">${esc(meta.name)}</h1>
        <p>${esc(meta.folderName || '')}</p>
      </div>
      <button type="button" class="btn ghost sm" id="iv-original">加载原图</button>
      ${meta.canDelete ? '<button type="button" class="btn danger sm" id="detail-delete">删除所在文件夹媒体</button>' : ''}
    </div>
    <div class="image-view">
      <div class="iv-img-wrap"><img id="img" alt="${esc(meta.name)}"></div>
      <p class="iv-status" role="status">正在加载图片…</p>
      <button type="button" class="iv-arrow left" id="iv-prev" aria-label="上一张">${icon('back', 20)}</button>
      <button type="button" class="iv-arrow right" id="iv-next" aria-label="下一张">${icon('fwd', 20)}</button>
    </div>`
  app.replaceChildren(root)

  const $ = selector => root.querySelector(selector)
  const img = $('#img')
  const wrap = $('.iv-img-wrap')
  const status = $('.iv-status')
  const originalButton = $('#iv-original')
  const prevButton = $('#iv-prev')
  const nextButton = $('#iv-next')
  let stopped = false
  let scale = 1

  function showImage(original = false) {
    if (stopped) return
    status.hidden = false
    status.textContent = '正在加载图片…'
    originalButton.disabled = original
    img.src = `/api/media/${meta.id}/image${original ? '' : '?w=480'}`
  }
  img.addEventListener('load', () => { status.hidden = true })
  img.addEventListener('error', () => {
    if (stopped) return
    status.hidden = false
    status.textContent = '图片加载失败'
    originalButton.disabled = false
  })
  function syncNavigation() {
    prevButton.hidden = !meta.prev
    nextButton.hidden = !meta.next
  }
  function navigate(dir) {
    if (!stopped && meta[dir]) location.hash = withHome(`#/image/${meta[dir].id}`, home)
  }
  prevButton.onclick = () => navigate('prev')
  nextButton.onclick = () => navigate('next')
  originalButton.onclick = () => showImage(true)
  const onKey = e => {
    if (stopped || e.target?.closest?.('input, textarea, select, dialog')) return
    if (e.key === 'ArrowLeft') navigate('prev')
    else if (e.key === 'ArrowRight') navigate('next')
    else if (e.key === 'Escape') location.hash = folder || home
  }
  document.addEventListener('keydown', onKey)

  let sx = null
  wrap.addEventListener('pointerdown', e => { sx = e.clientX })
  wrap.addEventListener('pointerup', e => {
    if (sx == null) return
    const dx = e.clientX - sx
    sx = null
    if (Math.abs(dx) > 60) navigate(dx > 0 ? 'prev' : 'next')
  })
  const applyZoom = (factor, ox, oy) => {
    scale = Math.min(4, Math.max(1, factor))
    if (scale <= 1.01) {
      img.style.transform = ''
      img.classList.remove('zoomed')
    } else {
      img.style.transformOrigin = `${ox}px ${oy}px`
      img.style.transform = `scale(${scale})`
      img.classList.add('zoomed')
      if (!originalButton.disabled) showImage(true)
    }
  }
  function toggleZoom(e) {
    const rect = img.getBoundingClientRect()
    applyZoom(scale > 1.01 ? 1 : 2.5, e.clientX - rect.left, e.clientY - rect.top)
  }
  let lastTap = 0
  img.addEventListener('pointerup', e => {
    if (e.pointerType === 'mouse' || stopped) return
    const now = performance.now()
    if (lastTap && now - lastTap < 300) {
      toggleZoom(e)
      lastTap = 0
    } else {
      lastTap = now
    }
  })
  img.addEventListener('dblclick', e => {
    if (!stopped) toggleZoom(e)
  })
  wrap.addEventListener('wheel', e => {
    if (stopped || (!e.ctrlKey && Math.abs(e.deltaY) < 2)) return
    e.preventDefault()
    const rect = img.getBoundingClientRect()
    applyZoom(scale * (e.deltaY < 0 ? 1.2 : 0.83), e.clientX - rect.left, e.clientY - rect.top)
  }, { passive: false })

  const releaseImage = () => {
    stopped = true
    img.removeAttribute('src')
    status.textContent = '图片已关闭'
    status.hidden = false
    originalButton.disabled = true
  }
  const deleteButton = $('#detail-delete')
  const disposeDelete = deleteButton && mountFolderDelete(root, deleteButton, meta.folderId, releaseImage)
  syncNavigation()
  showImage()
  return {
    destroy() {
      releaseImage()
      disposeDelete?.()
      document.removeEventListener('keydown', onKey)
      root.remove()
    },
  }
}
