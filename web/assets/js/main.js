// 入口：hash 路由 + 顶栏搜索
import { HomeView } from './views/home.js'
import { FolderView } from './views/folder.js'
import { SearchView } from './views/search.js'
import { PlayerView } from './views/player.js'
import { ImageView } from './views/imageview.js'
import { language, setLanguage, t } from './i18n.js'
import { homeHref, returnHomeHref, routeQuery, searchHref } from './navigation.js'
import { esc } from './util.js'

const routes = [
  { re: /^#\/$/, view: HomeView },
  { re: /^#\/folder\/([A-Za-z0-9_-]+)$/, view: FolderView },
  { re: /^#\/search$/, view: SearchView },
  { re: /^#\/play\/([A-Za-z0-9_-]+)$/, view: PlayerView },
  { re: /^#\/image\/([A-Za-z0-9_-]+)$/, view: ImageView },
]

let current = null
let currentPath = ''
let routeGeneration = 0
const app = document.getElementById('app')

async function route(force = false) {
  const path = (location.hash || '#/').split('?', 1)[0]
  document.querySelector('.brand').href = returnHomeHref()
  if (!force && path === currentPath && current?.updateRoute) {
    current.updateRoute()
    updateClearButton()
    return
  }
  const viewState = force ? current?.saveViewState?.() : undefined
  const generation = ++routeGeneration
  for (const r of routes) {
    const m = path.match(r.re)
    if (m) {
      if (current && current.destroy) {
        try { current.destroy() } catch {}
      }
      current = null
      app.innerHTML = ''
      document.body.classList.remove('player-mode')
      window.scrollTo(0, 0)
      document.getElementById('searchInput').blur()
      try {
        const view = r.view === PlayerView || r.view === ImageView
          ? await r.view(app, ...m.slice(1), () => generation === routeGeneration)
          : await r.view(app, ...m.slice(1), viewState)
        if (generation !== routeGeneration) {
          if (view && view.destroy) view.destroy()
          return
        }
        current = view
        currentPath = path
        updateClearButton()
      } catch (e) {
        if (generation !== routeGeneration) return
        console.error(e)
        app.innerHTML = `<div class="empty-state"><p>${esc(t('loadError', { detail: e.message }))}</p><p class="hint">${esc(t('retryRefresh'))}</p></div>`
      }
      return
    }
  }
  location.hash = '#/'
}

const form = document.getElementById('searchForm')
const input = document.getElementById('searchInput')
const clear = document.getElementById('clearSearch')
const languageSelect = document.getElementById('languageSelect')

function updateClearButton() {
  clear.hidden = input.value.length === 0
}

function localizeTopbar() {
  document.documentElement.lang = language
  document.title = t('title')
  document.querySelector('.brand').setAttribute('aria-label', t('home'))
  input.placeholder = t('searchHint')
  input.setAttribute('aria-label', t('searchHint'))
  clear.setAttribute('aria-label', t('clearSearch'))
  clear.title = t('clearSearch')
  languageSelect.value = language
  languageSelect.title = t('language')
  languageSelect.setAttribute('aria-label', t('language'))
}

languageSelect.addEventListener('change', () => {
  if (setLanguage(languageSelect.value)) {
    localizeTopbar()
    route(true)
  }
})
localizeTopbar()
window.addEventListener('hashchange', () => route())
route()

input.addEventListener('input', updateClearButton)
clear.onclick = () => {
  current?.cancelPendingSearch?.()
  input.value = ''
  updateClearButton()
  const home = homeHref()
  if (location.hash !== home) {
    window.addEventListener('hashchange', () => input.focus(), { once: true })
    location.hash = home
  } else {
    input.focus()
  }
}
form.addEventListener('submit', e => {
  e.preventDefault()
  const q = input.value.trim()
  if (q) {
    const actor = currentPath === '#/search' ? routeQuery().get('actor') || '' : ''
    location.hash = searchHref(q, actor)
  }
  else input.focus()
})
