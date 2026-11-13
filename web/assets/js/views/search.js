// 搜索页：后端分页的混合搜索结果
import { api } from '../api.js'
import { mediaCard } from '../cards.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, sortBarEl, debounce } from '../util.js'

export function SearchView(app, qs) {
  const input = document.getElementById('searchInput')
  let q = new URLSearchParams(qs).get('q') || ''
  input.value = q
  let sort = { key: 'relevance', order: 'desc' }
  let request
  let disposed = false

  app.innerHTML = `<div class="page">
    <div class="page-head">
      <h2 id="qtitle"></h2>
      <span class="count" id="count"></span>
      <span class="spacer"></span>
      <span id="sortSlot"></span>
    </div>
    <div id="grid" class="grid"></div>
  </div>`

  const root = app.firstElementChild
  const $ = sel => root.querySelector(sel)
  const grid = $('#grid')
  const pages = pager(load, '搜索结果分页')
  grid.after(pages.nav)

  async function load(page = 1) {
    request?.abort()
    const controller = new AbortController()
    request = controller
    $('#qtitle').textContent = q ? `“${q}”` : '搜索'
    $('#count').textContent = ''
    grid.classList.toggle('list-empty', !q)
    if (!q) {
      pages.update(1, 0)
      grid.innerHTML = emptyHTML('输入关键词搜索全库', '支持文件名和拼音搜索', 'search')
      return
    }
    const query = new URLSearchParams({ q, page, size: pageSize, sort: sort.key, order: sort.order })
    grid.innerHTML = skeleton(8, true)
    try {
      const data = await api(`/api/search?${query}`, { signal: controller.signal })
      if (controller.signal.aborted) return
      if (page > 1 && !data.items.length) return load(Math.max(1, Math.ceil(data.total / data.size)))
      $('#count').textContent = `${data.total} 个结果`
      grid.classList.toggle('list-empty', !data.items.length)
      grid.innerHTML = data.items.length ? data.items.map(mediaCard).join('')
        : emptyHTML('没有找到匹配的内容', '换个关键词试试', 'search')
      pages.update(data.page, data.total, data.size)
    } catch (error) {
      if (controller.signal.aborted) return
      grid.classList.add('list-empty')
      grid.innerHTML = emptyHTML('加载失败', error.message, 'search')
    }
  }

  $('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'relevance', label: '相关度' },
    { key: 'mtime', label: '修改日期' },
  ], v => { sort = v; load() }))
  const onInput = debounce(value => {
    if (disposed) return
    q = value.trim()
    history.replaceState(null, '', '#/search?q=' + encodeURIComponent(q))
    load()
  }, 350)
  const handler = e => onInput(e.target.value)
  input.addEventListener('input', handler)
  load()
  return {
    destroy() {
      disposed = true
      request?.abort()
      input.removeEventListener('input', handler)
    },
  }
}
