// 首页：继续观看（多设备进度）+ 母文件夹封面墙
import { api, del } from '../api.js'
import { folderCard, mediaCard } from '../cards.js'
import { skeleton, emptyHTML, sortBarEl, getSort, setSort, esc, fmtDur, icon } from '../util.js'

function timeAgo(ms) {
  const d = Date.now() - ms
  if (d < 3600e3) return Math.max(1, Math.round(d / 60e3)) + ' 分钟前'
  if (d < 86400e3) return Math.round(d / 3600e3) + ' 小时前'
  return Math.round(d / 86400e3) + ' 天前'
}

export async function HomeView(app) {
  const sort = getSort()
  app.innerHTML = `<div class="page">
    <div id="recentWrap"></div>
    <div class="page-head">
      <h2>媒体库</h2>
      <span class="count" id="count"></span>
      <span class="spacer"></span>
      <span id="sortSlot"></span>
    </div>
    <div id="grid" class="grid">${skeleton(10)}</div>
  </div>`

  const grid = document.getElementById('grid')
  const count = document.getElementById('count')

  async function loadRecent() {
    const wrap = document.getElementById('recentWrap')
    let d
    try { d = await api('/api/progress/recent?n=12') } catch { return }
    const items = (d && d.items) || []
    if (!items.length) { wrap.innerHTML = ''; return }
    // 并发拉封面信息（最多 12 个）
    const metas = await Promise.all(items.map(it =>
      api(`/api/media/${it.mediaId}`).catch(() => null)))
    const cards = items.map((it, i) => {
      const m = metas[i]
      if (!m) return '' // 媒体已被移除
      const pct = it.duration > 0 ? Math.min(100, Math.round(it.position / it.duration * 100)) : 0
      const remain = it.duration > 0 ? fmtDur(Math.max(0, it.duration - it.position)) : ''
      const cover = m.cardCover || ''
        ? `<img loading="lazy" src="${esc(m.cardCover)}" alt="" onerror="this.style.display='none'">`
        : `<div class="cover-ph">${icon('film', 32)}</div>`
      return `<a class="card media-card" href="#/play/${it.mediaId}">
        <div class="card-cover wide">${cover}
          <div class="rc-progress"><i style="width:${pct}%"></i></div>
          ${remain ? `<span class="dur">剩 ${remain}</span>` : ''}
        </div>
        <div class="card-body">
          <div class="card-title" title="${esc(m.name)}">${esc(m.name)}</div>
          <div class="card-meta">${esc(m.folderName || it.folder || '')} · ${timeAgo(it.updatedAt)}</div>
        </div>
      </a>`
    }).filter(Boolean)
    if (!cards.length) { wrap.innerHTML = ''; return }
    wrap.innerHTML = `
      <div class="page-head" style="margin-top:6px">
        <h2>继续观看</h2>
        <span class="count">${cards.length} 条进度</span>
        <span class="spacer"></span>
        <button class="btn ghost sm" id="btn-clear-recent">清空进度</button>
      </div>
      <div class="grid recent-row">${cards.join('')}</div>
      <div class="section-title">媒体库</div>`
    document.getElementById('btn-clear-recent').addEventListener('click', async () => {
      if (!confirm('清除所有观看进度？（不影响视频文件）')) return
      await del('/api/progress')
      loadRecent()
    })
  }

  async function load() {
    const d = await api(`/api/folders?sort=${sort.key}&order=${sort.order}`)
    if (!d.folders.length) {
      grid.outerHTML = emptyHTML('还没有内容', '在电脑端 Hestia 窗口添加视频文件夹路径，或等待扫描完成')
      return
    }
    count.textContent = `${d.folders.length} 个合集`
    grid.innerHTML = d.folders.map(folderCard).join('')
  }

  document.getElementById('sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: '名称' },
    { key: 'mtime', label: '修改日期' },
  ], v => { setSort(v); Object.assign(sort, v); load() }))

  loadRecent()
  await load()
  return {}
}
