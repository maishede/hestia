// 搜索页：后端分页的媒体文件名搜索
import { t } from '../i18n.js'
import { api } from '../api.js'
import { actorFilterMarkup, bindActorFilter } from '../actor-filter.js'
import { mediaCard } from '../cards.js'
import { returnHomeHref, routeQuery, searchHref } from '../navigation.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, sortBarEl } from '../util.js'

export function SearchView(app, viewState) {
  const input = document.getElementById('searchInput')
  let home = returnHomeHref()
  let q
  let actor = ''
  let sort = viewState?.sort ?? { key: 'relevance', order: 'desc' }
  let activePage = viewState?.page ?? 1
  let inputTimer
  let request

  app.innerHTML = `<div class="page">
    <div class="library-layout">
      ${actorFilterMarkup()}
      <div class="library-content">
        <div class="page-head">
          <h2 id="qtitle"></h2>
          <span class="count" id="count"></span>
          <span class="spacer"></span>
          <span id="sortSlot"></span>
        </div>
        <div id="grid" class="grid"></div>
      </div>
    </div>
  </div>`

  const root = app.firstElementChild
  const $ = sel => root.querySelector(sel)
  const grid = $('#grid')
  const actors = bindActorFilter(root, {
    onSelect: selectActor, allLabel: t('allSearchResults'), alwaysVisible: true, initial: viewState,
  })
  actors.render([], routeQuery().get('actor') || '')
  const pages = pager(load, t('searchPagination'))
  grid.after(pages.nav)

  async function load(page = 1) {
    activePage = page
    request?.abort()
    const controller = new AbortController()
    request = controller
    $('#qtitle').textContent = t('queryTitle', { query: q })
    $('#count').textContent = ''
    grid.classList.remove('list-empty')
    const query = new URLSearchParams({ q, page, size: pageSize, sort: sort.key, order: sort.order })
    if (actor) query.set('actor', actor)
    grid.innerHTML = skeleton(8, true)
    try {
      const data = await api(`/api/search?${query}`, { signal: controller.signal })
      if (controller.signal.aborted) return
      if (page > 1 && !data.items.length) return load(Math.max(1, Math.ceil(data.total / data.size)))
      const matchingActors = data.actors ?? []
      actors.render(matchingActors, actor, matchingActors.length ? '' : t('noSearchActors'))
      $('#count').textContent = t('resultCount', { count: data.total })
      grid.classList.toggle('list-empty', !data.items.length)
      grid.innerHTML = data.items.length ? data.items.map(item => mediaCard(item, { newTab: true })).join('')
        : emptyHTML(t('noResults'), t('tryKeyword'), 'search')
      pages.update(data.page, data.total, data.size)
    } catch (error) {
      if (controller.signal.aborted) return
      grid.classList.add('list-empty')
      grid.innerHTML = emptyHTML(t('loadFailed'), error.message, 'search')
    }
  }

  $('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'relevance', label: t('sortRelevance') },
    { key: 'mtime', label: t('sortDate') },
  ], v => { sort = v; load() }))
  function selectActor(name) {
    if (name === actor) return
    clearTimeout(inputTimer)
    location.hash = searchHref(input.value.trim() || q, name, home)
  }

  function onInput() {
    clearTimeout(inputTimer)
    inputTimer = setTimeout(() => {
      const nextQuery = input.value.trim()
      if (!nextQuery) {
        request?.abort()
        input.value = ''
        location.replace(home)
        return
      }
      history.replaceState(null, '', searchHref(nextQuery, actor, home))
      updateRoute()
    }, 350)
  }

  function updateRoute(first = false) {
    clearTimeout(inputTimer)
    home = returnHomeHref()
    const params = routeQuery()
    const nextQuery = (params.get('q') || '').trim()
    const nextActor = params.get('actor') || ''
    if (!nextQuery) {
      input.value = ''
      location.replace(home)
      return
    }
    if (q === nextQuery && actor === nextActor) return
    q = nextQuery
    actor = nextActor
    input.value = first ? viewState?.draft ?? q : q
    actors.select(actor)
    load(first ? activePage : 1)
  }

  input.addEventListener('input', onInput)
  updateRoute(true)
  if (q && input.value.trim() !== q) onInput()
  return {
    updateRoute,
    cancelPendingSearch() { clearTimeout(inputTimer) },
    saveViewState() { return { ...actors.saveState(), draft: input.value, sort, page: activePage } },
    destroy() {
      clearTimeout(inputTimer)
      request?.abort()
      input.removeEventListener('input', onInput)
    },
  }
}
