// 搜索页：模糊搜索结果（视频/图片/文件夹混合）
import { api } from '../api.js'
import { mediaCard } from '../cards.js'
import { skeleton, emptyHTML, esc, sortBarEl, debounce } from '../util.js'

export async function SearchView(app, qs) {
  const initQ = new URLSearchParams(qs).get('q') || ''
  const input = document.getElementById('searchInput')
  input.value = initQ

  let q = initQ
  let sort = { key: 'relevance', order: 'desc' }
  let page = 1
  let observer = null
  let destroyed = false

  app.innerHTML = `<div class="page">
    <div class="page-head">
      <h2 id="qtitle"></h2>
      <span class="count" id="count"></span>
      <span class="spacer"></span>
      <span id="sortSlot"></span>
    </div>
    <div id="grid" class="grid">${initQ ? skeleton(8, true) : ''}</div>
    <div id="sentinel" class="load-more"></div>
  </div>`

  const $ = sel => app.querySelector(sel)
  const grid = $('#grid')

  async function loadPage(append) {
    const d = await api(`/api/search?q=${encodeURIComponent(q)}&page=${page}&size=20&sort=${sort.key === 'relevance' ? '' : sort.key}&order=${sort.order}`)
    $('#qtitle').textContent = `“${q}”`
    $('#count').textContent = d.total ? `${d.total} 个结果` : ''
    const cards = d.items.map(mediaCard).join('')
    if (append) grid.insertAdjacentHTML('beforeend', cards)
    else grid.innerHTML = cards

    if (!d.total && page === 1) {
      grid.outerHTML = emptyHTML('没有找到匹配的内容', '换个关键词试试（支持模糊匹配）', 'search')
      return
    }
    const loaded = (page - 1) * 20 + d.items.length
    $('#sentinel').textContent = loaded < d.total ? '上滑加载更多…' : ''
    if (loaded < d.total) ensureObserver()
    else if (observer) { observer.disconnect(); observer = null }
  }

  function ensureObserver() {
    if (observer) return
    observer = new IntersectionObserver(async entries => {
      if (!entries[0].isIntersecting || destroyed || observer.loading) return
      observer.loading = true
      page += 1
      try { await loadPage(true) } catch (e) { console.error(e) }
      observer.loading = false
    }, { rootMargin: '600px' })
    observer.observe($('#sentinel'))
  }

  $('#sortSlot').appendChild(sortBarEl({ key: 'relevance', order: 'desc' }, [
    { key: 'relevance', label: '相关度' },
    { key: 'mtime', label: '修改日期' },
  ], v => {
    sort = v
    page = 1
    grid.innerHTML = skeleton(8, true)
    loadPage(false)
  }))

  if (initQ) await loadPage(false)
  else app.querySelector('.page').innerHTML = emptyHTML('输入关键词搜索全库', '支持文件名模糊匹配，如“流浪”“avgr”', 'search')

  // 搜索框即时搜索（当前停留在搜索页时）
  const onInput = debounce(v => {
    const nv = v.trim()
    if (!nv) return
    q = nv
    page = 1
    history.replaceState(null, '', '#/search?q=' + encodeURIComponent(q))
    grid.innerHTML = skeleton(8, true)
    loadPage(false)
  }, 350)
  const handler = e => onInput(e.target.value)
  input.addEventListener('input', handler)

  return {
    destroy() {
      destroyed = true
      input.removeEventListener('input', handler)
      if (observer) observer.disconnect()
    },
  }
}
