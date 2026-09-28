// API 请求封装
export async function api(path, opts = {}) {
  const res = await fetch(path, opts)
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const j = await res.json()
      if (j && j.error) msg = j.error
    } catch {}
    throw new Error(msg)
  }
  if (res.status === 204) return null
  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('json')) throw new Error('服务响应异常，请稍后重试')
  const j = await res.json()
  if (j === null || j === undefined) throw new Error('服务返回为空，请稍后重试')
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
