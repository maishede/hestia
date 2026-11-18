import { api } from './api.js'
import { withHome } from './navigation.js'
import { toast } from './util.js'

// The confirmation always names the containing folder and the entire scope.
export function mountFolderDelete(parent, button, folderID, beforeDelete = () => {}, codeRequired = false) {
  const url = `/api/folders/${folderID}`
  const dialog = document.createElement('dialog')
  dialog.className = 'delete-dialog'
  dialog.setAttribute('aria-labelledby', 'delete-title')
  dialog.innerHTML = `<form>
    <h2 id="delete-title">删除文件夹中的媒体</h2>
    <div id="delete-auth" hidden>
      <p>在电脑端打开任一媒体的删除窗口，点击「查看手机授权码」。</p>
      <label for="delete-code">删除授权码</label>
      <input id="delete-code" class="ipt" type="password" maxlength="32" autocomplete="off" autocapitalize="off">
    </div>
    <p id="delete-summary"></p>
    <p id="delete-subfolders" class="delete-warning" hidden></p>
    <div id="delete-fields" hidden>
      <label for="delete-reason">删除原因</label>
      <input id="delete-reason" class="ipt" maxlength="80" autocomplete="off">
    </div>
    <p id="delete-preview-name" class="delete-preview-name"></p>
    <p id="delete-error" class="delete-warning" role="alert" hidden></p>
    ${codeRequired ? '' : '<button type="button" class="btn ghost sm" id="delete-show-code">查看手机授权码</button><p id="delete-code-display" hidden></p>'}
    <div class="delete-actions">
      <button class="btn ghost" type="button" id="delete-cancel">取消</button>
      <button class="btn danger-fill" type="submit" id="delete-submit">确认删除</button>
    </div>
  </form>`
  parent.appendChild(dialog)
  const $ = sel => dialog.querySelector(sel)
  const reason = $('#delete-reason')
  const code = $('#delete-code')
  const auth = $('#delete-auth')
  const submit = $('#delete-submit')
  const cancel = $('#delete-cancel')
  let preview
  let previewRequest
  let disposed = false
  const headers = () => codeRequired ? { 'X-Hestia-Delete-Code': code.value.trim() } : {}

  const showCode = $('#delete-show-code')
  if (showCode) showCode.onclick = async () => {
    try {
      const { code } = await api('/api/delete-access-code')
      const display = $('#delete-code-display')
      display.textContent = code
      display.hidden = false
    } catch (error) { showError(error.message) }
  }

  function updateName() {
    const suffix = reason.value.trim()
    $('#delete-preview-name').textContent = suffix ? `改名为「${preview.name}-${suffix}」` : ''
  }
  function showError(message) {
    const error = $('#delete-error')
    error.textContent = message
    error.hidden = !message
  }

  async function loadPreview() {
    previewRequest?.abort()
    const request = new AbortController()
    previewRequest = request
    submit.disabled = true
    showError('')
    try {
      const data = await api(`${url}/delete-preview`, { headers: headers(), signal: request.signal })
      if (request.signal.aborted || !dialog.open) return
      preview = data
      auth.hidden = true
      $('#delete-fields').hidden = false
      submit.textContent = '确认删除'
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
      if (!codeRequired) (preview.files ? reason : cancel).focus()
    } catch (error) {
      if (!request.signal.aborted) showError(error.message)
    } finally {
      if (!request.signal.aborted) submit.disabled = preview ? !preview.files : !codeRequired
    }
  }

  button.onclick = () => {
    previewRequest?.abort()
    preview = null
    code.value = ''
    auth.hidden = !codeRequired
    $('#delete-fields').hidden = true
    $('#delete-summary').textContent = ''
    $('#delete-subfolders').hidden = true
    $('#delete-preview-name').textContent = ''
    if (showCode) $('#delete-code-display').hidden = true
    showError('')
    submit.disabled = false
    submit.textContent = codeRequired ? '查看删除范围' : '确认删除'
    dialog.showModal()
    if (!codeRequired) loadPreview()
  }

  reason.addEventListener('input', updateName)
  cancel.onclick = () => dialog.close()
  dialog.addEventListener('cancel', event => {
    if (cancel.disabled) event.preventDefault()
  })
  $('form').onsubmit = async event => {
    event.preventDefault()
    if (submit.disabled) return
    if (!preview && codeRequired) {
      if (!/^[0-9a-f]{32}$/.test(code.value.trim())) {
        showError('请输入电脑端显示的 32 位授权码')
        return
      }
      await loadPreview()
      return
    }
    if (!preview?.files) return
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
      const result = await api(`${url}/delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...headers() },
        body: JSON.stringify({ reason: value, token: preview.token }),
      })
      if (disposed) return
      dialog.close()
      toast(result.warning || `已删除 ${result.files} 个媒体文件，文件夹已改名为「${result.name}」`,
        result.warning ? 'err' : 'info', result.warning ? 6500 : 3200)
      location.hash = withHome(`#/folder/${result.id}`)
    } catch (error) {
      if (!disposed) showError(error.message)
    } finally {
      reason.disabled = submit.disabled = cancel.disabled = false
      submit.textContent = '确认删除'
    }
  }

  return () => {
    disposed = true
    previewRequest?.abort()
    dialog.remove()
  }
}
