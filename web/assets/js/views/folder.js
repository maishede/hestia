// 文件夹页：子文件夹和媒体分别分页
import { api } from '../api.js'
import { mountFolderDelete } from '../delete-dialog.js'
import { folderCard, mediaCard } from '../cards.js'
import { returnHomeHref, withHome } from '../navigation.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, esc, sortBarEl, getSort, setSort } from '../util.js'

export function FolderView(app, id) {
  const sort = getSort()
  const url = `/api/folders/${id}`
  let folderPage = 1
  let mediaPage = 1
  let request

  app.innerHTML = `<div class="page">
    <div class="breadcrumb">
      <a class="btn ghost sm" href="${esc(returnHomeHref())}">返回主页</a>
      <span id="crumb" class="breadcrumb-trail"></span>
    </div>
    <div class="page-head">
      <h2 id="fname"></h2>
      <span class="count" id="count"></span>
      <span class="spacer"></span>
      <span id="sortSlot"></span>
      <button type="button" class="btn ghost sm danger" id="delete-media" hidden>删除媒体</button>
    </div>
    <div id="error"></div>
    <section id="foldersWrap" hidden>
      <h3 class="section-title">子文件夹</h3>
      <div id="fgrid" class="grid"></div>
    </section>
    <section id="mediaWrap">
      <h3 class="section-title">媒体</h3>
      <div id="grid" class="grid">${skeleton(8, true)}</div>
    </section>
  </div>`

  const root = app.firstElementChild
  const $ = sel => root.querySelector(sel)
  const grid = $('#grid')
  const folders = $('#fgrid')
  const deleteButton = $('#delete-media')
  const disposeDelete = mountFolderDelete(root, deleteButton, id)
  const folderPager = pager(page => { folderPage = page; load() }, '子文件夹分页')
  const mediaPager = pager(page => { mediaPage = page; load() }, '媒体分页')
  folders.after(folderPager.nav)
  grid.after(mediaPager.nav)

  async function load() {
    request?.abort()
    const controller = new AbortController()
    request = controller
    const query = new URLSearchParams({ page: mediaPage, folderPage, size: pageSize, sort: sort.key, order: sort.order })
    try {
      const data = await api(`${url}/children?${query}`, { signal: controller.signal })
      if (controller.signal.aborted) return
      const { breadcrumb, media } = data
      const lastFolder = Math.max(1, Math.ceil(data.foldersTotal / media.size))
      const lastMedia = Math.max(1, Math.ceil(media.total / media.size))
      if (folderPage > lastFolder || mediaPage > lastMedia) {
        folderPage = Math.min(folderPage, lastFolder)
        mediaPage = Math.min(mediaPage, lastMedia)
        return load()
      }
      $('#error').replaceChildren()
      $('#crumb').innerHTML = breadcrumb.map((crumb, index) => {
        const current = index === breadcrumb.length - 1
        const href = crumb.id ? withHome(`#/folder/${crumb.id}`) : returnHomeHref()
        return `<a class="${current ? 'cur' : ''}" href="${esc(href)}">${esc(crumb.name)}</a>`
      }).join('<span class="sep">/</span>')
      $('#fname').textContent = breadcrumb.at(-1)?.name || ''
      $('#count').textContent = `${data.foldersTotal} 个子文件夹 · ${media.total} 个媒体`
      deleteButton.hidden = !data.canDelete
      $('#foldersWrap').hidden = !data.foldersTotal
      folders.innerHTML = data.folders.map(folder => folderCard(folder)).join('')
      folderPager.update(data.foldersPage, data.foldersTotal, media.size)
      $('#mediaWrap').hidden = !media.total && data.foldersTotal > 0
      grid.classList.toggle('list-empty', !media.total)
      grid.innerHTML = media.items.length ? media.items.map(item => mediaCard(item)).join('')
        : emptyHTML('这个文件夹是空的', '放入媒体后可在控制台重新扫描')
      mediaPager.update(media.page, media.total, media.size)
    } catch (error) {
      if (controller.signal.aborted) return
      $('#error').innerHTML = emptyHTML('加载失败', error.message)
      $('#mediaWrap').hidden = true
    }
  }

  $('#sortSlot').appendChild(sortBarEl(sort, [
    { key: 'name', label: '名称' },
    { key: 'mtime', label: '修改日期' },
    { key: 'size', label: '大小' },
  ], v => {
    setSort(v)
    Object.assign(sort, v)
    folderPage = mediaPage = 1
    load()
  }))
  load()
  return {
    destroy() {
      request?.abort()
      disposeDelete()
    },
  }
}
