// API 请求封装
import { serverMessage, t } from './i18n.js'

export async function api(path, opts = {}) {
  let res
  try {
    res = await fetch(path, opts)
  } catch (error) {
    if (error.name === 'AbortError') throw error
    throw new Error(t('networkFailed'), { cause: error })
  }
  if (!res.ok) {
    let msg = t('requestFailed', { status: res.status })
    try {
      const j = await res.json()
      if (j?.error) msg = serverMessage(j.error)
    } catch {}
    throw new Error(msg)
  }
  if (res.status === 204) return null
  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('json')) throw new Error(t('responseInvalid'))
  const j = await res.json()
  if (j === null || j === undefined) throw new Error(t('responseEmpty'))
  return j
}

export function post(path, body) {
  return api(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body === undefined ? '{}' : JSON.stringify(body) })
}
export function patch(path, body) {
  return api(path, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
}
export function del(path) {
  return api(path, { method: 'DELETE' })
}
