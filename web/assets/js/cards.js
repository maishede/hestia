// 共享卡片组件：文件夹卡 / 媒体卡（视频、图片）
import { esc, fmtSize, fmtDur, icon } from './util.js'

export function folderCard(f) {
  const cover = f.cover
    ? `<img loading="lazy" src="${esc(f.cover)}" alt="" onerror="this.style.display='none'">`
    : `<div class="cover-ph">${icon('folder', 34)}</div>`
  const badge = f.subVideos ? `<span class="badge">${f.subVideos} 部</span>` : ''
  const newBadge = f.isNew ? `<span class="badge new">新</span>` : ''
  const meta = (f.videoCount != null) ? `${f.videoCount} 视频 · ${f.imageCount} 图` : (f.where || '')
  return `<a class="card folder-card" href="#/folder/${f.id}">
    <div class="card-cover">${cover}${badge}${newBadge}</div>
    <div class="card-body">
      <div class="card-title" title="${esc(f.name)}">${esc(f.name)}</div>
      <div class="card-meta">${esc(meta)}</div>
    </div>
  </a>`
}

export function mediaCard(m) {
  if (m.kind === 'folder') {
    return `<a class="card folder-card" href="#/folder/${m.id}">
      <div class="card-cover"><div class="cover-ph">${icon('folder', 34)}</div></div>
      <div class="card-body">
        <div class="card-title" title="${esc(m.name)}">${icon('folder', 12)} ${esc(m.name)}</div>
        <div class="card-meta">${esc(m.where || '文件夹')}</div>
      </div>
    </a>`
  }
  const href = m.kind === 'image' ? `#/image/${m.id}` : `#/play/${m.id}`
  const cover = m.cardCover
    ? `<img loading="lazy" src="${esc(m.cardCover)}" alt="" onerror="this.style.display='none'">`
    : `<div class="cover-ph">${icon(m.kind === 'image' ? 'image' : 'film', 32)}</div>`
  const dur = m.duration ? `<span class="dur">${fmtDur(m.duration)}</span>` : ''
  const newBadge = m.isNew ? `<span class="badge new">新</span>` : ''
  const playHint = m.kind === 'video' ? `<span class="play-hint">${icon('play', 22)}</span>` : ''
  const metaBits = [m.where, fmtSize(m.size), m.mtime ? fmtDateLocal(m.mtime) : '', m.subs ? 'CC' : ''].filter(Boolean).join(' · ')
  return `<a class="card media-card" href="${href}">
    <div class="card-cover">${cover}${dur}${newBadge}${playHint}</div>
    <div class="card-body">
      <div class="card-title" title="${esc(m.name)}">${esc(m.name)}</div>
      <div class="card-meta">${esc(metaBits)}</div>
    </div>
  </a>`
}

function fmtDateLocal(unix) {
  const d = new Date(unix * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
