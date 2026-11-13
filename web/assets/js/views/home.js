// 首页：继续观看（多设备进度）+ 分页媒体库
import { api, del } from '../api.js'
import { folderCard } from '../cards.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, sortBarEl, getSort, setSort, esc, fmtDur, icon, toast } from '../util.js'

function timeAgo(ms) {
  const d = Date.now() - ms
  if (d < 3600e3) return Math.max(1, Math.round(d / 60e3)) + ' 分钟前'
  if (d < 86400e3) return Math.round(d / 3600e3) + ' 小时前'
  return Math.round(d / 86400e3) + ' 天前'
}

export function HomeView(app) {
  const sort = getSort()
  if (sort.key === 'size') sort.key = 'name'
  let listRequest
  let disposed = false

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

  const root = app.firstElementChild
  const grid = root.querySelector('#grid')
  const count = root.querySelector('#count')
  const pages = pager(load, '媒体库分页')
  grid.after(pages.nav)

  async function loadRecent() {
    const wrap = root.querySelector('#recentWrap')
    let data
    try { data = await api('/api/progress/recent?n=12') } catch { return }
    if (disposed) return
    if (!data.items.length) { wrap.replaceChildren(); return }
    const cards = data.items.map(it => {
      const pct = it.duration > 0 ? Math.min(100, Math.round(it.position / it.duration * 100)) : 0
      const remain = it.duration > 0 ? fmtDur(Math.max(0, it.duration - it.position)) : ''
      const cover = it.cardCover
        ? `<img loading="lazy" src="${esc(it.cardCover)}" alt="" onerror="this.style.display='none'">`
        : `<div class="cover-ph">${icon('film', 32)}</div>`
      return `<a class="card media-card" href="#/play/${esc(it.mediaId)}">
        <div class="card-cover wide">${cover}
          <div class="rc-progress"><i style="width:${pct}%"></i></div>
          ${remain ? `<span class="dur">剩 ${remain}</span>` : ''}
        </div>
        <div class="card-body">
          <div class="card-title" title="${esc(it.name)}">${esc(it.name)}</div>
          <div class="card-meta">${esc(it.folder)} · ${timeAgo(it.updatedAt)}</div>
        </div>
      </a>`
    })
    wrap.innerHTML = `<div class="page-head">
        <h2>继续观看</h2>
        <span class="count">${cards.length} 条进度</span>
        <span class="spacer"></span>
        <button class="btn ghost sm" id="btn-clear-recent">清空进度</button>
      </div>
      <div class="grid recent-row">${cards.join('')}</div>`
    wrap.querySelector('#btn-clear-recent').onclick = async () => {
      if (!confirm('清除所有观看进度？')) return
      try {
        await del('/api/progress')
        loadRecent()
      } catch (error) {
        toast(error.message, 'err')
      }
    }
  }

  async function load(page = 1) {
    listRequest?.abort()
    const request = new AbortController()
    listRequest = request
    const query = new URLSearchParams({ page, size: pageSize, sort: sort.key, order: sort.order })
    grid.innerHTML = skeleton(10)
    grid.classList.remove('list-empty')
    try {
      const data = await api(`/api/folders?${query}`, { signal: request.signal })
      if (request.signal.aborted) return
      if (page > 1 && !data.folders.length) {
        return load(Math.max(1, Math.ceil(data.total / data.size)))
      }
      count.textContent = `${data.total} 个合集`
      grid.classList.toggle('list-empty', !data.folders.length)
      grid.innerHTML = data.folders.length ? data.folders.map(folderCard).join('')
        : emptyHTML('还没有内容', '在电脑端添加媒体库目录，或等待扫描完成')
      pages.update(data.page, data.total, data.size)
    } catch (error) {
      if (request.signal.aborted) return
      grid.classList.add('list-empty')
      grid.innerHTML = emptyHTML('加载失败', error.message)
    }
  }

  root.querySelector('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: '名称' },
    { key: 'mtime', label: '修改日期' },
  ], v => { setSort(v); Object.assign(sort, v); load() }))

  load()
  loadRecent()
  return {
    destroy() {
      disposed = true
      listRequest?.abort()
    },
  }
}
