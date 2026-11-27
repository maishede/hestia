// 首页：继续观看（多设备进度）+ 分页媒体库
import { relativeTime, t } from '../i18n.js'
import { actorFilterMarkup, bindActorFilter } from '../actor-filter.js'
import { api, del } from '../api.js'
import { folderCard, mediaCard, updatedTime } from '../cards.js'
import { homeHref, pageNumber, routeQuery, withHome } from '../navigation.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, sortBarEl, getSort, setSort, esc, fmtDur, icon, toast } from '../util.js'

function timeAgo(ms) {
  if (!Number.isFinite(ms) || ms <= 0) return ''
  const d = Date.now() - ms
  if (d < 3600e3) return relativeTime(-Math.max(1, Math.round(d / 60e3)), 'minute')
  if (d < 86400e3) return relativeTime(-Math.round(d / 3600e3), 'hour')
  return relativeTime(-Math.round(d / 86400e3), 'day')
}

export function HomeView(app, viewState) {
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
      ${actorFilterMarkup()}
      <div class="library-content">
        <div id="recentWrap"></div>
        <div class="page-head">
          <h2 id="libraryTitle">${esc(t('library'))}</h2>
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
  const actors = bindActorFilter(root, { onSelect: selectActor, allLabel: t('allCollections'), initial: viewState })
  const pages = pager(next => { location.hash = homeHref(actor, next) }, t('libraryPagination'))
  grid.after(pages.nav)

  function selectActor(name) {
    if (actor !== name) location.hash = homeHref(name)
  }

  async function loadActors() {
    try {
      const data = await api('/api/actors', { signal: actorsRequest.signal })
      if (!disposed) actors.render(data.actors, actor)
    } catch (error) {
      if (!disposed) toast(error.message, 'err')
    }
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
          ${remain ? `<span class="dur">${esc(t('remaining', { time: remain }))}</span>` : ''}
        </div>
        <div class="card-body">
          <div class="card-title" title="${esc(it.name)}">${esc(it.name)}</div>
          <div class="card-meta">${esc([it.folder, timeAgo(it.updatedAt)].filter(Boolean).join(' · '))}</div>
          ${updatedTime(it.mtime)}
        </div>
      </a>`
    })
    recent.innerHTML = `<div class="page-head">
        <h2>${esc(t('keepWatching'))}</h2>
        <span class="count">${esc(t('progressCount', { count: cards.length }))}</span>
        <span class="spacer"></span>
        <button class="btn ghost sm" id="btn-clear-recent">${esc(t('clearProgress'))}</button>
      </div>
      <div class="grid recent-row">${cards.join('')}</div>`
    recent.querySelector('#btn-clear-recent').onclick = async () => {
      if (!confirm(t('confirmClearProgress'))) return
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
    root.querySelector('#libraryTitle').textContent = actor || t('library')
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
      count.textContent = t(actor ? 'videoCount' : 'collectionCount', { count: data.total })
      grid.classList.toggle('list-empty', !items.length)
      const renderCard = actor ? mediaCard : folderCard
      grid.innerHTML = items.length ? items.map(item => renderCard(item, { newTab: true })).join('')
        : actor ? emptyHTML(t('noActorVideos'), '', 'film')
          : emptyHTML(t('libraryEmpty'), t('libraryEmptyHint'))
      pages.update(data.page, data.total, data.size)
    } catch (error) {
      if (request.signal.aborted) return
      grid.classList.add('list-empty')
      grid.innerHTML = emptyHTML(t('loadFailed'), error.message)
    }
  }

  function updateRoute() {
    const query = routeQuery()
    const nextActor = query.get('actor') || ''
    const nextPage = pageNumber(query.get('page'))
    if (actor === nextActor && page === nextPage) return
    actor = nextActor
    page = nextPage
    actors.select(actor)
    recentRequest?.abort()
    recent.replaceChildren()
    if (!actor) loadRecent()
    load()
  }

  root.querySelector('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: t('sortName') },
    { key: 'mtime', label: t('sortDate') },
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
    saveViewState: actors.saveState,
    destroy() {
      disposed = true
      listRequest?.abort()
      recentRequest?.abort()
      actorsRequest.abort()
    },
  }
}
