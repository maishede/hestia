export const pageSize = 20

export function pager(onPage, label) {
  const nav = document.createElement('nav')
  nav.className = 'pagination'
  nav.setAttribute('aria-label', label)
  nav.hidden = true
  nav.innerHTML = `<button type="button" class="btn ghost sm">上一页</button>
    <span aria-live="polite"></span>
    <button type="button" class="btn ghost sm">下一页</button>`
  const [prev, info, next] = nav.children
  let current = 1
  prev.onclick = () => onPage(current - 1)
  next.onclick = () => onPage(current + 1)
  return {
    nav,
    update(page, total, size = pageSize) {
      current = page
      const pages = Math.max(1, Math.ceil(total / size))
      nav.hidden = pages <= 1
      info.textContent = `第 ${page} / ${pages} 页`
      prev.disabled = page <= 1
      next.disabled = page >= pages
    },
  }
}
