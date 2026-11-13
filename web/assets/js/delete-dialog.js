import { api, post } from './api.js'
import { toast } from './util.js'

// The confirmation always names the containing folder and the entire scope.
export function mountFolderDelete(parent, button, folderID, beforeDelete = () => {}) {
  const url = `/api/folders/${folderID}`
  const dialog = document.createElement('dialog')
  dialog.className = 'delete-dialog'
  dialog.setAttribute('aria-labelledby', 'delete-title')
  dialog.innerHTML = `<form>
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
  </form>`
  parent.appendChild(dialog)
  const $ = sel => dialog.querySelector(sel)
  const reason = $('#delete-reason')
  const submit = $('#delete-submit')
  const cancel = $('#delete-cancel')
  let preview
  let disposed = false

  function updateName() {
    const suffix = reason.value.trim()
    $('#delete-preview-name').textContent = suffix ? `改名为「${preview.name}-${suffix}」` : ''
  }
  function showError(message) {
    const error = $('#delete-error')
    error.textContent = message
    error.hidden = !message
  }

  button.onclick = async () => {
    button.disabled = true
    try {
      preview = await api(`${url}/delete-preview`)
      if (disposed) return
      reason.value = ''
      reason.disabled = submit.disabled = !preview.files
      updateName()
      showError('')
      $('#delete-summary').textContent = preview.files
        ? `将永久删除「${preview.name}」中直属的 ${preview.files} 个视频、图片或字幕，并停止正在读取该目录的播放。其他文件会保留。`
        : `「${preview.name}」中没有可删除的直属媒体。`
      const warning = $('#delete-subfolders')
      warning.hidden = !preview.subfolders
      warning.textContent = `发现 ${preview.subfolders} 个子文件夹或链接，请手动检查。里面的内容会保留。`
      dialog.showModal()
      const focus = preview.files ? reason : cancel
      focus.focus()
    } catch (error) {
      if (!disposed) toast(error.message, 'err')
    } finally {
      button.disabled = false
    }
  }

  reason.addEventListener('input', updateName)
  cancel.onclick = () => dialog.close()
  dialog.addEventListener('cancel', event => {
    if (cancel.disabled) event.preventDefault()
  })
  $('form').onsubmit = async event => {
    event.preventDefault()
    if (!preview?.files || submit.disabled) return
    const value = reason.value.trim()
    if (!value) {
      showError('请输入删除原因')
      reason.focus()
      return
    }
    reason.disabled = submit.disabled = cancel.disabled = true
    submit.textContent = '正在删除…'
    showError('')
    try {
      beforeDelete()
      const result = await post(`${url}/delete`, { reason: value, token: preview.token })
      if (disposed) return
      dialog.close()
      toast(result.warning || `已删除 ${result.files} 个媒体文件，文件夹已改名为「${result.name}」`,
        result.warning ? 'err' : 'info', result.warning ? 6500 : 3200)
      location.hash = `#/folder/${result.id}`
    } catch (error) {
      if (!disposed) showError(error.message)
    } finally {
      reason.disabled = submit.disabled = cancel.disabled = false
      submit.textContent = '确认删除'
    }
  }

  return () => {
    disposed = true
    dialog.remove()
  }
}
