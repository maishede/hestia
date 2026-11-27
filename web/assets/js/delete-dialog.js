import { api } from './api.js'
import { withHome } from './navigation.js'
import { serverMessage, t } from './i18n.js'
import { esc, toast } from './util.js'

// The confirmation always names the containing folder and the entire scope.
export function mountFolderDelete(parent, button, folderID, beforeDelete = () => {}, codeRequired = false) {
  const url = `/api/folders/${folderID}`
  const dialog = document.createElement('dialog')
  dialog.className = 'delete-dialog'
  dialog.setAttribute('aria-labelledby', 'delete-title')
  dialog.innerHTML = `<form>
    <h2 id="delete-title">${esc(t('deleteTitle'))}</h2>
    <div id="delete-auth" hidden>
      <p>${esc(t('deleteGetCode'))}</p>
      <label for="delete-code">${esc(t('deleteCode'))}</label>
      <input id="delete-code" class="ipt" type="password" maxlength="32" autocomplete="off" autocapitalize="off">
    </div>
    <p id="delete-summary"></p>
    <p id="delete-subfolders" class="delete-warning" hidden></p>
    <div id="delete-fields" hidden>
      <label for="delete-reason">${esc(t('deleteReason'))}</label>
      <input id="delete-reason" class="ipt" maxlength="80" autocomplete="off">
    </div>
    <p id="delete-preview-name" class="delete-preview-name"></p>
    <p id="delete-error" class="delete-warning" role="alert" hidden></p>
    ${codeRequired ? '' : `<button type="button" class="btn ghost sm" id="delete-show-code">${esc(t('deleteShowCode'))}</button><p id="delete-code-display" hidden></p>`}
    <div class="delete-actions">
      <button class="btn ghost" type="button" id="delete-cancel">${esc(t('cancel'))}</button>
      <button class="btn danger-fill" type="submit" id="delete-submit">${esc(t('deleteConfirm'))}</button>
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
    $('#delete-preview-name').textContent = suffix ? t('deleteRenameTo', { name: `${preview.name}-${suffix}` }) : ''
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
      submit.textContent = t('deleteConfirm')
      reason.value = ''
      reason.disabled = submit.disabled = !preview.files
      updateName()
      showError('')
      $('#delete-summary').textContent = preview.files
        ? t('deleteSummary', { name: preview.name, count: preview.files })
        : t('deleteNone', { name: preview.name })
      const warning = $('#delete-subfolders')
      warning.hidden = !preview.subfolders
      warning.textContent = t('deleteSubfolders', { count: preview.subfolders })
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
    submit.textContent = codeRequired ? t('deletePreview') : t('deleteConfirm')
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
        showError(t('deleteCodeRequired'))
        return
      }
      await loadPreview()
      return
    }
    if (!preview?.files) return
    const value = reason.value.trim()
    if (!value) {
      showError(t('deleteReasonRequired'))
      reason.focus()
      return
    }
    reason.disabled = submit.disabled = cancel.disabled = true
    submit.textContent = t('deleting')
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
      toast(result.warning ? serverMessage(result.warning) : t('deleted', { count: result.files, name: result.name }),
        result.warning ? 'err' : 'info', result.warning ? 6500 : 3200)
      location.hash = withHome(`#/folder/${result.id}`)
    } catch (error) {
      if (!disposed) showError(error.message)
    } finally {
      reason.disabled = submit.disabled = cancel.disabled = false
      submit.textContent = t('deleteConfirm')
    }
  }

  return () => {
    disposed = true
    previewRequest?.abort()
    dialog.remove()
  }
}
