// 共享卡片组件：文件夹卡 / 媒体卡（视频、图片）
import { esc, fmtSize, fmtDur, icon } from './util.js'
import { language, t } from './i18n.js'
import { withHome } from './navigation.js'

function detailLink(href, { newTab = false } = {}) {
  return `href="${esc(withHome(href))}"${newTab ? ' target="_blank" rel="noopener"' : ''}`
}

export function folderCard(f, options) {
  const cover = f.cover
    ? `<img loading="lazy" src="${esc(f.cover)}" alt="" onerror="this.style.display='none'">`
    : `<div class="cover-ph">${icon('folder', 34)}</div>`
  const badge = f.subVideos ? `<span class="badge">${esc(t('shortVideoCount', { count: f.subVideos }))}</span>` : ''
  const newBadge = f.isNew ? `<span class="badge new">${esc(t('newBadge'))}</span>` : ''
  const meta = (f.videoCount != null) ? t('mediaBreakdown', { videos: f.videoCount, images: f.imageCount }) : (f.where || '')
  return `<a class="card folder-card" ${detailLink(`#/folder/${f.id}`, options)}>
    <div class="card-cover">${cover}${badge}${newBadge}</div>
    <div class="card-body">
      <div class="card-title" title="${esc(f.name)}">${esc(f.name)}</div>
      <div class="card-meta">${esc(meta)}</div>
      ${updatedTime(f.mtime)}
    </div>
  </a>`
}

export function mediaCard(m, options) {
  const href = m.kind === 'image' ? `#/image/${m.id}` : `#/play/${m.id}`
  const ph = `<div class="cover-ph">${icon(m.kind === 'image' ? 'image' : 'film', 32)}</div>`
  const cover = m.cardCover
    ? `${ph}<img loading="lazy" src="${esc(m.cardCover)}" alt="" onerror="this.style.display='none'">`
    : ph
  const dur = m.kind === 'video' && m.duration > 1 ? `<span class="dur">${fmtDur(m.duration)}</span>` : ''
  const newBadge = m.isNew ? `<span class="badge new">${esc(t('newBadge'))}</span>` : ''
  const playHint = m.kind === 'video' ? `<span class="play-hint">${icon('play', 22)}</span>` : ''
  const metaBits = [m.where, fmtSize(m.size), m.subs ? 'CC' : ''].filter(Boolean).join(' · ')
  return `<a class="card media-card" ${detailLink(href, options)}>
    <div class="card-cover">${cover}${dur}${newBadge}${playHint}</div>
    <div class="card-body">
      <div class="card-title" title="${esc(m.name)}">${esc(m.name)}</div>
      <div class="card-meta">${esc(metaBits)}</div>
      ${updatedTime(m.mtime)}
    </div>
  </a>`
}

export function updatedTime(value) {
  if (!value) return ''
  const date = new Date(typeof value === 'number' ? value * 1000 : value)
  if (!Number.isFinite(date.getTime())) return ''
  const day = language === 'zh-CN'
    ? `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
    : date.toLocaleDateString(language, { year: 'numeric', month: 'short', day: 'numeric' })
  const time = date.toLocaleTimeString(language, { hour12: false })
  return `<time class="card-meta card-updated" datetime="${date.toISOString()}" title="${esc(`${day} ${time}`)}">${esc(t('updated', { date: day }))}</time>`
}
