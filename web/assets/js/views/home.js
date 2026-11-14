// 首页：继续观看（多设备进度）+ 分页媒体库
import { api, del } from '../api.js'
import { folderCard, mediaCard, updatedTime } from '../cards.js'
import { homeHref, pageNumber, routeQuery, withHome } from '../navigation.js'
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
  let actor
  let page
  let listRequest
  let recentRequest
  const actorsRequest = new AbortController()
  let disposed = false

  app.innerHTML = `<div class="page">
    <div class="library-layout">
      <aside class="actor-sidebar" aria-label="主演筛选" hidden>
        <h2>主演</h2>
        <div id="actorList" class="actor-list"></div>
      </aside>
      <div class="actor-picker" hidden>
        <label for="actorPicker">主演</label>
        <select class="sort-sel" id="actorPicker"></select>
      </div>
      <div class="library-content">
        <div id="recentWrap"></div>
        <div class="page-head">
          <h2 id="libraryTitle">媒体库</h2>
          <span class="count" id="count"></span>
          <span class="spacer"></span>
          <span id="sortSlot"></span>
        </div>
        <div id="grid" class="grid">${skeleton(10)}</div>
      </div>
    </div>
  </div>`

  const root = app.firstElementChild
  const grid = root.querySelector('#grid')
  const count = root.querySelector('#count')
  const recent = root.querySelector('#recentWrap')
  const actorList = root.querySelector('#actorList')
  const actorPicker = root.querySelector('#actorPicker')
  const pages = pager(next => { location.hash = homeHref(actor, next) }, '媒体库分页')
  grid.after(pages.nav)

  function selectActor(name) {
    if (actor !== name) location.hash = homeHref(name)
  }

  actorPicker.onchange = () => selectActor(actorPicker.value)
  actorList.onclick = event => {
    const button = event.target.closest('button[data-actor]')
    if (button) selectActor(button.dataset.actor)
  }

  async function loadActors() {
    let actors
    try {
      const data = await api('/api/actors', { signal: actorsRequest.signal })
      if (disposed) return
      actors = data.actors
    } catch (error) {
      if (!disposed) toast(error.message, 'err')
      return
    }
    if (!actors.length && !actor) return
    const choices = [{ name: '', label: '全部合集' }, ...actors.map(a => ({ ...a, label: a.name }))]
    if (actor && !actors.some(a => a.name === actor)) choices.push({ name: actor, label: actor, count: 0 })
    actorList.innerHTML = choices.map(({ name, label, count }) =>
      `<button type="button" data-actor="${esc(name)}" aria-pressed="${name === actor}">
        <span>${esc(label)}</span>${count == null ? '' : `<small>${count}</small>`}
      </button>`).join('')
    for (const { name, label } of choices) actorPicker.add(new Option(label, name))
    actorPicker.value = actor
    actorList.parentElement.hidden = false
    actorPicker.parentElement.hidden = false
    root.classList.add('with-actors')
  }

  async function loadRecent() {
    if (disposed || actor) return
    recentRequest?.abort()
    const request = new AbortController()
    recentRequest = request
    let data
    try { data = await api('/api/progress/recent?n=12', { signal: request.signal }) } catch { return }
    if (request.signal.aborted) return
    if (!data.items.length) { recent.replaceChildren(); return }
    const cards = data.items.map(it => {
      const pct = it.duration > 0 ? Math.min(100, Math.round(it.position / it.duration * 100)) : 0
      const remain = it.duration > 0 ? fmtDur(Math.max(0, it.duration - it.position)) : ''
      const cover = it.cardCover
        ? `<img loading="lazy" src="${esc(it.cardCover)}" alt="" onerror="this.style.display='none'">`
        : `<div class="cover-ph">${icon('film', 32)}</div>`
      return `<a class="card media-card" href="${esc(withHome(`#/play/${it.mediaId}`))}" target="_blank" rel="noopener">
        <div class="card-cover wide">${cover}
          <div class="rc-progress"><i style="width:${pct}%"></i></div>
          ${remain ? `<span class="dur">剩 ${remain}</span>` : ''}
        </div>
        <div class="card-body">
          <div class="card-title" title="${esc(it.name)}">${esc(it.name)}</div>
          <div class="card-meta">${esc(it.folder)} · ${timeAgo(it.updatedAt)}</div>
          ${updatedTime(it.mtime)}
        </div>
      </a>`
    })
    recent.innerHTML = `<div class="page-head">
        <h2>继续观看</h2>
        <span class="count">${cards.length} 条进度</span>
        <span class="spacer"></span>
        <button class="btn ghost sm" id="btn-clear-recent">清空进度</button>
      </div>
      <div class="grid recent-row">${cards.join('')}</div>`
    recent.querySelector('#btn-clear-recent').onclick = async () => {
      if (!confirm('清除所有观看进度？')) return
      try {
        await del('/api/progress')
        loadRecent()
      } catch (error) {
        toast(error.message, 'err')
      }
    }
  }

  async function load() {
    listRequest?.abort()
    const request = new AbortController()
    listRequest = request
    const query = new URLSearchParams({ page, size: pageSize, sort: sort.key, order: sort.order })
    if (actor) query.set('actor', actor)
    const endpoint = actor ? '/api/actors/videos' : '/api/folders'
    root.querySelector('#libraryTitle').textContent = actor || '媒体库'
    count.textContent = ''
    pages.nav.hidden = true
    grid.innerHTML = skeleton(10, Boolean(actor))
    grid.classList.remove('list-empty')
    try {
      const data = await api(`${endpoint}?${query}`, { signal: request.signal })
      if (request.signal.aborted) return
      const items = actor ? data.items : data.folders
      if (page > 1 && !items.length) {
        location.replace(homeHref(actor, Math.max(1, Math.ceil(data.total / data.size))))
        return
      }
      count.textContent = actor ? `${data.total} 部视频` : `${data.total} 个合集`
      grid.classList.toggle('list-empty', !items.length)
      const renderCard = actor ? mediaCard : folderCard
      grid.innerHTML = items.length ? items.map(item => renderCard(item, { newTab: true })).join('')
        : actor ? emptyHTML('没有找到该主演的视频', '', 'film')
          : emptyHTML('还没有内容', '在电脑端添加媒体库目录，或等待扫描完成')
      pages.update(data.page, data.total, data.size)
    } catch (error) {
      if (request.signal.aborted) return
      grid.classList.add('list-empty')
      grid.innerHTML = emptyHTML('加载失败', error.message)
    }
  }

  function updateRoute() {
    const query = routeQuery()
    const nextActor = query.get('actor') || ''
    const nextPage = pageNumber(query.get('page'))
    if (actor === nextActor && page === nextPage) return
    actor = nextActor
    page = nextPage
    actorList.querySelector('[aria-pressed="true"]')?.setAttribute('aria-pressed', 'false')
    actorList.querySelector(`[data-actor="${CSS.escape(actor)}"]`)?.setAttribute('aria-pressed', 'true')
    actorPicker.value = actor
    recentRequest?.abort()
    recent.replaceChildren()
    if (!actor) loadRecent()
    load()
  }

  root.querySelector('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: '名称' },
    { key: 'mtime', label: '修改日期' },
  ], v => {
    setSort(v)
    Object.assign(sort, v)
    if (page > 1) location.replace(homeHref(actor))
    else load()
  }))

  updateRoute()
  loadActors()
  return {
    updateRoute,
    destroy() {
      disposed = true
      listRequest?.abort()
      recentRequest?.abort()
      actorsRequest.abort()
    },
  }
}
