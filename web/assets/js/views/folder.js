// 文件夹页：子文件夹和媒体分别分页
import { api, post } from '../api.js'
import { folderCard, mediaCard } from '../cards.js'
import { pageSize, pager } from '../pagination.js'
import { skeleton, emptyHTML, esc, sortBarEl, getSort, setSort, toast } from '../util.js'

export function FolderView(app, id) {
  const sort = getSort()
  const url = `/api/folders/${id}`
  let folderPage = 1
  let mediaPage = 1
  let request
  let disposed = false
  let preview

  app.innerHTML = `<div class="page">
    <div id="crumb" class="breadcrumb"></div>
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
    <dialog class="delete-dialog" aria-labelledby="delete-title">
      <form>
        <h2 id="delete-title">删除文件夹中的媒体</h2>
        <p id="delete-summary"></p>
        <p id="delete-subfolders" class="delete-warning" hidden></p>
        <label for="delete-reason">删除原因</label>
        <input id="delete-reason" class="ipt" maxlength="80" required autocomplete="off">
        <p id="delete-preview-name" class="delete-preview-name"></p>
        <p id="delete-error" class="delete-warning" role="alert" hidden></p>
        <div class="delete-actions">
          <button class="btn ghost" type="button" id="delete-cancel">取消</button>
          <button class="btn danger-fill" type="submit" id="delete-submit">确认删除</button>
        </div>
      </form>
    </dialog>
  </div>`

  const root = app.firstElementChild
  const $ = sel => root.querySelector(sel)
  const grid = $('#grid')
  const folders = $('#fgrid')
  const dialog = $('dialog')
  const reason = $('#delete-reason')
  const deleteButton = $('#delete-media')
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
        const href = crumb.id ? `#/folder/${esc(crumb.id)}` : '#/'
        return `<a class="${current ? 'cur' : ''}" href="${href}">${esc(crumb.name)}</a>`
      }).join('<span class="sep">/</span>')
      $('#fname').textContent = breadcrumb.at(-1)?.name || ''
      $('#count').textContent = `${data.foldersTotal} 个子文件夹 · ${media.total} 个媒体`
      deleteButton.hidden = !data.canDelete
      $('#foldersWrap').hidden = !data.foldersTotal
      folders.innerHTML = data.folders.map(folderCard).join('')
      folderPager.update(data.foldersPage, data.foldersTotal, media.size)
      $('#mediaWrap').hidden = !media.total && data.foldersTotal > 0
      grid.classList.toggle('list-empty', !media.total)
      grid.innerHTML = media.items.length ? media.items.map(mediaCard).join('')
        : emptyHTML('这个文件夹是空的', '放入媒体后可在控制台重新扫描')
      mediaPager.update(media.page, media.total, media.size)
    } catch (error) {
      if (controller.signal.aborted) return
      $('#error').innerHTML = emptyHTML('加载失败', error.message)
      $('#mediaWrap').hidden = true
    }
  }

  function updateNewName() {
    const suffix = reason.value.trim()
    $('#delete-preview-name').textContent = suffix ? `改名为「${preview.name}-${suffix}」` : ''
  }
  function deleteError(message) {
    const error = $('#delete-error')
    error.textContent = message
    error.hidden = !message
  }
  deleteButton.onclick = async () => {
    deleteButton.disabled = true
    try {
      preview = await api(`${url}/delete-preview`)
      if (disposed) return
      reason.value = ''
      reason.disabled = submit.disabled = !preview.files
      updateNewName()
      deleteError('')
      $('#delete-summary').textContent = preview.files
        ? `将永久删除「${preview.name}」中的 ${preview.files} 个视频、图片或字幕文件，其他文件会保留。`
        : `「${preview.name}」中没有可删除的直属媒体文件。`
      const warning = $('#delete-subfolders')
      warning.hidden = !preview.subfolders
      warning.textContent = `发现 ${preview.subfolders} 个子文件夹或链接，里面的内容会保留。请手动检查是否需要处理。`
      dialog.showModal()
      const focus = preview.files ? reason : cancel
      focus.focus()
    } catch (error) {
      if (!disposed) toast(error.message, 'err')
    } finally {
      deleteButton.disabled = false
    }
  }

  const submit = $('#delete-submit')
  const cancel = $('#delete-cancel')
  reason.addEventListener('input', updateNewName)
  cancel.onclick = () => dialog.close()
  dialog.addEventListener('cancel', event => {
    if (cancel.disabled) event.preventDefault()
  })
  dialog.querySelector('form').onsubmit = async event => {
    event.preventDefault()
    if (!preview?.files || submit.disabled) return
    const value = reason.value.trim()
    if (!value) { deleteError('请输入删除原因'); reason.focus(); return }
    reason.disabled = submit.disabled = cancel.disabled = true
    deleteError('')
    try {
      const result = await post(`${url}/delete`, { reason: value, token: preview.token })
      if (disposed) return
      dialog.close()
      toast(result.warning || `已删除 ${result.files} 个媒体文件，文件夹已改名为「${result.name}」`,
        result.warning ? 'err' : 'info', result.warning ? 6500 : 3200)
      location.hash = `#/folder/${result.id}`
    } catch (error) {
      if (!disposed) deleteError(error.message)
    } finally {
      reason.disabled = submit.disabled = cancel.disabled = false
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
      disposed = true
      request?.abort()
      if (dialog.open) dialog.close()
    },
  }
}
