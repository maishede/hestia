// 入口：hash 路由 + 顶栏搜索
import { HomeView } from './views/home.js'
import { FolderView } from './views/folder.js'
import { SearchView } from './views/search.js'
import { PlayerView } from './views/player.js'
import { ImageView } from './views/imageview.js'
import { returnHomeHref, withHome } from './navigation.js'
import { esc } from './util.js'

const routes = [
  { re: /^#\/$/, view: HomeView },
  { re: /^#\/folder\/([A-Za-z0-9_-]+)$/, view: FolderView },
  { re: /^#\/search$/, view: SearchView },
  { re: /^#\/play\/([A-Za-z0-9_-]+)$/, view: PlayerView },
  { re: /^#\/image\/([A-Za-z0-9_-]+)$/, view: ImageView },
]

let current = null
let routeGeneration = 0
const app = document.getElementById('app')

async function route() {
  const generation = ++routeGeneration
  const path = (location.hash || '#/').split('?', 1)[0]
  document.querySelector('.brand').href = returnHomeHref()
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
          : await r.view(app, ...m.slice(1))
        if (generation !== routeGeneration) {
          if (view && view.destroy) view.destroy()
          return
        }
        current = view
      } catch (e) {
        if (generation !== routeGeneration) return
        console.error(e)
        app.innerHTML = `<div class="empty-state"><p>加载失败：${esc(e.message)}</p><p class="hint">请刷新重试</p></div>`
      }
      return
    }
  }
  location.hash = '#/'
}

window.addEventListener('hashchange', route)
route()

const form = document.getElementById('searchForm')
const input = document.getElementById('searchInput')
form.addEventListener('submit', e => {
  e.preventDefault()
  const q = input.value.trim()
  if (q) location.hash = withHome('#/search?q=' + encodeURIComponent(q))
  else input.focus()
})
