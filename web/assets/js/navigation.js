export function routeQuery(hash = location.hash) {
  const separator = hash.indexOf('?')
  return new URLSearchParams(separator === -1 ? '' : hash.slice(separator + 1))
}

export function pageNumber(value) {
  const page = Number(value)
  return Number.isSafeInteger(page) && page > 0 ? page : 1
}

export function homeHref(actor = '', page = 1) {
  const query = new URLSearchParams()
  if (actor) query.set('actor', actor)
  if (page > 1) query.set('page', page)
  const params = query.toString()
  return params ? `#/?${params}` : '#/'
}

export function returnHomeHref() {
  const query = routeQuery()
  const home = location.hash.split('?', 1)[0] === '#/' ? query : routeQuery(query.get('home') || '')
  return homeHref(home.get('actor'), pageNumber(home.get('page')))
}

export function withHome(href, home = returnHomeHref()) {
  if (home === '#/') return href
  const query = routeQuery(href)
  query.set('home', home)
  return `${href.split('?', 1)[0]}?${query}`
}
