// 文件夹页：面包屑 + 子文件夹 + 媒体网格（每页 20，无限滚动）
import { api } from '../api.js'
import { folderCard, mediaCard } from '../cards.js'
import { skeleton, emptyHTML, esc, sortBarEl, getSort, setSort } from '../util.js'

export async function FolderView(app, id) {
  const sort = getSort()
  let page = 1
  let sentinel = null
  let observer = null
  let destroyed = false

  app.innerHTML = `<div class="page">
    <div id="crumb" class="breadcrumb">${skeleton(1)}</div>
    <div class="page-head">
      <h2 id="fname"></h2>
      <span class="count" id="count"></span>
      <span class="spacer"></span>
      <span id="sortSlot"></span>
    </div>
    <div id="foldersWrap"></div>
    <div id="mediaWrap">
      <div id="grid" class="grid">${skeleton(8, true)}</div>
      <div id="sentinel" class="load-more"></div>
    </div>
  </div>`

  const $ = sel => app.querySelector(sel)
  const grid = $('#grid')

  function renderCrumb(crumbs, curName) {
    const parts = crumbs.map((c, i) => {
      const last = i === crumbs.length - 1
      const href = c.id ? `#/folder/${c.id}` : '#/'
      const cls = last ? 'cur' : ''
      return `<a class="${cls}" href="${href}">${esc(c.name)}</a>${last ? '' : '<span class="sep">/</span>'}`
    })
    $('#crumb').innerHTML = parts.join('')
  }

  async function loadPage(append) {
    const d = await api(`/api/folders/${id}/children?page=${page}&size=20&sort=${sort.key}&order=${sort.order}`)
    if (destroyed) return
    if (!d || !d.media) return
    d.folders = d.folders || [] // 兼容空目录（历史版本返回 null）
    d.media.items = d.media.items || []
    renderCrumb(d.breadcrumb, d.folder)
    $('#fname').textContent = d.breadcrumb.length ? d.breadcrumb[d.breadcrumb.length - 1].name : ''

    const fw = $('#foldersWrap')
    if (d.folders.length) {
      if (!fw.querySelector('.grid')) {
        fw.innerHTML = `<div class="section-title">文件夹</div><div class="grid" id="fgrid"></div>`
      }
      fw.querySelector('#fgrid').innerHTML = d.folders.map(folderCard).join('')
    } else {
      fw.innerHTML = ''
    }

    const total = d.media.total
    $('#count').textContent = d.folders.length ? `${d.folders.length} 文件夹` : ''
    if (total) {
      $('#count').textContent = ($('#count').textContent ? $('#count').textContent + ' · ' : '') + `${total} 个媒体`
    }

    if (append) {
      grid.insertAdjacentHTML('beforeend', d.media.items.map(mediaCard).join(''))
    } else {
      grid.innerHTML = d.media.items.map(mediaCard).join('')
    }
    const loaded = (page - 1) * 20 + d.media.items.length
    $('#sentinel').textContent = loaded < total ? '上滑加载更多…' : ''

    if (!d.folders.length && !total && page === 1) {
      $('#mediaWrap').innerHTML = emptyHTML('这个文件夹是空的', '放入视频或图片后在电脑端重新扫描', 'folder')
      return
    }

    // 无限滚动
    if (loaded < total) {
      ensureObserver()
    } else if (observer) {
      observer.disconnect()
      observer = null
    }
  }

  function ensureObserver() {
    if (observer) return
    sentinel = $('#sentinel')
    observer = new IntersectionObserver(async entries => {
      if (!entries[0].isIntersecting || destroyed) return
      if (observer.loading) return
      observer.loading = true
      page += 1
      try { await loadPage(true) } catch (e) { console.error(e) }
      observer.loading = false
    }, { rootMargin: '600px' })
    observer.observe(sentinel)
  }

  $('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: '名称' },
    { key: 'mtime', label: '修改日期' },
    { key: 'size', label: '大小' },
  ], v => {
    setSort(v); Object.assign(sort, v)
    page = 1
    grid.innerHTML = skeleton(8, true)
    loadPage(false)
  }))

  await loadPage(false)
  return {
    destroy() {
      destroyed = true
      if (observer) observer.disconnect()
    },
  }
}
