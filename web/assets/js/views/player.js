// 播放器页：手势 + 倍速 + 断点续播 + 转码兜底
import { api, post } from '../api.js'
import { mountFolderDelete } from '../delete-dialog.js'
import { returnHomeHref, withHome } from '../navigation.js'
import { icon, fmtDur, fmtSize, toast, clamp, esc } from '../util.js'

const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
  (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)

let xgLoader = null
function ensurePlayer() {
  if (window.Player) return Promise.resolve(window.Player)
  if (!xgLoader) xgLoader = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '/assets/js/vendor/xgplayer.min.js'
    script.onload = () => window.Player ? resolve(window.Player) : reject(new Error('播放器加载失败'))
    script.onerror = () => reject(new Error('播放器加载失败'))
    document.head.appendChild(script)
  }).catch(error => { xgLoader = null; throw error })
  return xgLoader
}

let hlsLoader = null
function ensureHls() {
  if (window.Hls) return Promise.resolve(window.Hls)
  if (!hlsLoader) hlsLoader = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '/assets/js/vendor/hls.min.js'
    script.onload = () => window.Hls ? resolve(window.Hls) : reject(new Error('hls.js 加载失败'))
    script.onerror = () => { script.remove(); reject(new Error('hls.js 加载失败')) }
    document.head.appendChild(script)
  }).catch(error => { hlsLoader = null; throw error })
  return hlsLoader
}

export async function PlayerView(app, id, isCurrent = () => true) {
  app.innerHTML = '<div class="empty-state"><p>正在加载视频…</p></div>'
  const [meta, XGPlayer] = await Promise.all([
    api(`/api/media/${id}`).catch(e => { throw new Error(e.message || '视频不存在或已被移除') }),
    ensurePlayer(),
  ])
  if (!isCurrent()) return null
  if (!meta || !meta.stream) throw new Error('视频信息加载失败，请返回重试')

  // 字幕轨道清单（外挂 + 内嵌文本轨）
  const subExt = (meta.subtitles && meta.subtitles.external) || []
  const subEmb = (meta.subtitles && meta.subtitles.embedded) || []
  const ccTracks = [
    ...subExt.map(s => ({ kind: 'ext', id: s.id, label: s.name, el: null })),
    ...subEmb.map(s => ({ kind: 'emb', index: s.index, label: `${s.title || s.lang || '轨道 ' + (s.index)}` , el: null })),
  ]
  let activeCC = subExt.length ? 0 : -1

  // 设备标识（多设备进度同步）
  let device = localStorage.getItem('hestia.device')
  if (!device) {
    device = (crypto.randomUUID ? crypto.randomUUID() : 'dev-' + Date.now()).slice(0, 24)
    localStorage.setItem('hestia.device', device)
  }

  const root = document.createElement('div')
  root.className = 'player controls-on loading'
  root.innerHTML = `
    <div id="xg-player"></div>
    <div id="gesture"></div>
    <div class="hud" id="hud"><span id="hud-ico"></span><span class="hud-text" id="hud-text"></span><div class="hud-bar" id="hud-bar" style="display:none"><i id="hud-bar-i"></i></div></div>
    <div class="p-top">
      <button class="pbtn" id="btn-back" title="返回" aria-label="返回">${icon('back', 21)}</button>
      <div class="p-title">${esc(meta.name)}${meta.folderName ? ` <span style="opacity:.55;font-weight:400">· ${esc(meta.folderName)}</span>` : ''}</div>
      <span class="tc-chip" id="tc-chip" hidden>转码</span>
    </div>
    <div class="p-bottom">
      <div class="progress-row">
        <span class="time" id="t-cur">0:00</span>
        <div class="track" id="track">
          <div class="rail"><div class="buf" id="buf"></div><div class="played" id="played"></div></div>
          <div class="knob" id="knob" style="left:0%"></div>
          <input type="range" id="seek" min="0" max="1000" value="0" step="1" aria-label="进度">
        </div>
        <span class="time" id="t-dur">--:--</span>
      </div>
    </div>
    <div class="sub-menu" id="cc-menu" hidden></div>
    <div class="resume-tip" id="resume-tip" hidden>
      <span id="resume-text"></span>
      <button class="btn-sm" id="resume-go">继续播放</button>
      <button class="btn-sm ghost" id="resume-no">从头看</button>
    </div>
    <div class="center-note spinner-note"><div class="spinner"></div><div id="spin-text">加载中…</div></div>
    <div class="center-note error-note">
      ${icon('x', 40)}
      <div id="err-text">播放失败</div>
      <button class="btn" id="err-retry">用转码模式重试</button>
    </div>`
  const detail = document.createElement('section')
  detail.className = 'media-detail'
  const home = returnHomeHref()
  const folder = meta.folderId ? withHome(`#/folder/${meta.folderId}`, home) : ''
  const properties = [fmtDur(meta.duration), fmtSize(meta.size)].filter(Boolean).join(' · ')
  detail.innerHTML = `
    <div class="detail-header">
      <a class="btn ghost sm" href="${esc(home)}">返回主页</a>
      ${folder ? `<a class="btn ghost sm" href="${esc(folder)}">返回文件夹</a>` : ''}
      <div class="detail-title">
        <h1>${esc(meta.name)}</h1>
        <p>${esc([meta.folderName, properties].filter(Boolean).join(' · '))}</p>
      </div>
      ${meta.canDelete ? '<button type="button" class="btn danger sm" id="detail-delete">删除所在文件夹媒体</button>' : ''}
    </div>`
  detail.appendChild(root)
  app.replaceChildren(detail)

  const $ = s => root.querySelector(s)
  const savedVolume = Number.parseFloat(localStorage.getItem('hestia.volume') ?? '1')
  const player = new XGPlayer({
    el: $('#xg-player'), url: meta.stream, width: '100%', height: '100%',
    videoInit: false, autoplay: false, playsinline: true,
    ignores: ['mobile', 'pc', 'keyboard', 'progress', 'time', 'start', 'loading', 'error', 'replay', 'miniprogress', ...(isIOS ? ['volume'] : [])],
    controls: { autoHide: false, initShow: true },
    playbackRate: { list: [2, 1.5, 1, 0.5], toggleMode: 'click' },
    // Both fullscreen modes need to include Hestia's gesture and progress overlays.
    fullscreenTarget: root,
    fullscreen: { useScreenOrientation: true, lockOrientationType: 'landscape' },
    volume: Number.isFinite(savedVolume) ? clamp(savedVolume, 0, 1) : 1,
  })
  // Hestia owns source switching so xgplayer never overwrites the HLS MediaSource.
  player.handleSource = false
  await player.start()
  if (!isCurrent()) { player.destroy(); detail.remove(); return null }
  const leftGrid = $('xg-left-grid'), rightGrid = $('xg-right-grid')
  if (meta.prev) leftGrid.insertAdjacentHTML('afterbegin', `<button class="pbtn" id="btn-prev" title="上一个">${icon('back', 17)}</button>`)
  if (meta.next) leftGrid.insertAdjacentHTML('beforeend', `<button class="pbtn" id="btn-next" title="下一个">${icon('fwd', 17)}</button>`)
  rightGrid.insertAdjacentHTML('beforeend', `<button class="pbtn ${ccTracks.length ? '' : 'hide'}" id="btn-cc" title="字幕">${icon('cc', 20)}</button>`)
  const vid = player.media, gesture = $('#gesture'), hud = $('#hud')
  const hudIco = $('#hud-ico'), hudText = $('#hud-text'), hudBar = $('#hud-bar'), hudBarI = $('#hud-bar-i')
  const seek = $('#seek'), playedEl = $('#played'), bufEl = $('#buf'), knob = $('#knob')
  const tCur = $('#t-cur'), tDur = $('#t-dur')
  const chip = $('#tc-chip')
  const spinText = $('#spin-text'), errText = $('#err-text')
  const resumeTip = $('#resume-tip'), resumeText = $('#resume-text')

  let mode = 'direct'          // direct | hls
  let hls = null
  let nativeSource = null
  let session = null
  let offset = 0
  let failoverTried = false
  let lpPrevRate = 1
  let brightness = clamp(parseFloat(localStorage.getItem('hestia.brightness') || '1') || 1, 0.2, 1)
  let directStartAt = 0
  let seeking = false
  let resumeShown = false
  let lastSave = 0
  let destroyed = false
  let playbackStopped = false
  let pageHidden = false
  let sourceGeneration = 0
  let pendingHlsSeek = null // 原视频时间与播放意图，不受切源时 video 清零/暂停影响
  let sourceChanging = false
  let playIntent = false
  let hlsSeekTimer = null

  // ---------- 基础 ----------
  const totalDur = () => mode === 'direct' ? (vid.duration || 0) : (meta.duration > 0 ? meta.duration : offset + (vid.duration || 0))
  const curTime = () => pendingHlsSeek ? pendingHlsSeek.target : (mode === 'direct' ? vid.currentTime : offset + vid.currentTime)

  function applyBrightness() { vid.style.filter = `brightness(${brightness})` }
  applyBrightness()

  function tryPlay() { const result = player.play(); if (result && result.catch) result.catch(() => showControls()) }

  function togglePlay() {
    if (pendingHlsSeek) {
      pendingHlsSeek.shouldPlay = !pendingHlsSeek.shouldPlay
      if (!sourceChanging) pendingHlsSeek.shouldPlay ? tryPlay() : vid.pause()
      return
    }
    vid.paused ? tryPlay() : vid.pause()
  }

  // ---------- HUD ----------
  let hudTimer = null
  function showHUD(ico, text, pct) {
    hudIco.innerHTML = icon(ico, 26)
    hudText.textContent = text
    if (pct != null) { hudBar.style.display = ''; hudBarI.style.width = clamp(pct, 0, 100) + '%' }
    else hudBar.style.display = 'none'
    hud.classList.add('show')
    clearTimeout(hudTimer)
  }
  function hideHUD(ms = 500) {
    clearTimeout(hudTimer)
    hudTimer = setTimeout(() => hud.classList.remove('show'), ms)
  }

  // ---------- 控制栏显隐 ----------
  let controlsTimer = null
  function closeSpeedMenu() {
    const speed = player.getPlugin('playbackRate')
    if (speed && speed.isActive) speed.toggle(false)
  }
  function showControls() {
    root.classList.remove('controls-hidden')
    clearTimeout(controlsTimer)
    controlsTimer = setTimeout(() => {
      if (!vid.paused && ccMenu.hidden && resumeTip.hidden) {
        if (root.querySelector('.xg-options-list.active')) showControls()
        else root.classList.add('controls-hidden')
      }
    }, 3000)
  }
  function toggleControls() {
    closeSpeedMenu()
    if (root.classList.contains('controls-hidden')) showControls()
    else { root.classList.add('controls-hidden'); ccMenu.hidden = true }
  }

  // ---------- 播放源 ----------
  function deleteSession(sid) {
    if (sid) fetch(`/api/transcode/${sid}`, { method: 'DELETE', keepalive: true }).catch(() => {})
  }
  function stopSession() {
    nativeSource = null
    if (session) {
      const sid = session
      session = null
      deleteSession(sid)
    }
  }
  function destroyHLS() { if (hls) { try { hls.destroy() } catch {} hls = null } }
  function cancelHlsSeek() { clearTimeout(hlsSeekTimer); hlsSeekTimer = null }

  async function startHLS(startAt, shouldPlay = true, recoverMissingSession = true) {
    if (playbackStopped || destroyed) return
    if (!meta.playback.transcodeAvailable) throw new Error('服务端未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）')
    cancelHlsSeek()
    const generation = ++sourceGeneration
    const target = Math.max(0, startAt || 0)
    const pending = pendingHlsSeek && pendingHlsSeek.target === target
      ? pendingHlsSeek : { target, shouldPlay }
    pendingHlsSeek = pending
    sourceChanging = true
    destroyHLS()
    stopSession()
    if (destroyed) return
    root.classList.remove('error')
    root.classList.add('loading')
    spinText.textContent = '转码启动中…'
    chip.hidden = false
    chip.textContent = '转码中…'
    let r
    let reservedId = null
    let Hls = null
    let useMSE = false
    try {
      // Prepare the playback path before starting the server's transcode.
      // Prefer MSE when Chromium advertises native HLS but cannot demux TS.
      const nativeHls = vid.canPlayType('application/vnd.apple.mpegurl')
      try { Hls = await ensureHls() } catch (error) {
        if (!nativeHls) throw error
      }
      if (destroyed || generation !== sourceGeneration) return
      useMSE = Hls && Hls.isSupported()
      if (!useMSE && !nativeHls) throw new Error('浏览器不支持 HLS 播放')
      const reservation = await post(`/api/media/${id}/transcode/reserve`)
      reservedId = reservation.sessionId
      if (!reservedId) throw new Error('转码会话预留失败')
      if (destroyed || generation !== sourceGeneration) { deleteSession(reservedId); return }
      // Record the ID before starting FFmpeg, so pagehide can cancel either
      // the reservation or the session even when the start response is pending.
      session = reservedId
      r = await post(`/api/media/${id}/transcode`, { start: target, sessionId: reservedId })
      if (r.sessionId !== reservedId) {
        deleteSession(r.sessionId)
        throw new Error('转码会话标识不匹配')
      }
    } catch (error) {
      if (session === reservedId) stopSession()
      else deleteSession(reservedId)
      if (destroyed || generation !== sourceGeneration) return
      sourceChanging = false
      throw error
    }
    if (destroyed || generation !== sourceGeneration) {
      if (session === reservedId) session = null
      deleteSession(reservedId)
      return
    }
    offset = r.start
    mode = 'hls'
    try {
      if (useMSE) {
        const instance = new Hls({ maxBufferLength: 30 })
        hls = instance
        instance.on(Hls.Events.ERROR, (_, data) => {
          if (!data.fatal || hls !== instance || generation !== sourceGeneration) return
          const missing = data.type === Hls.ErrorTypes.NETWORK_ERROR && data.response?.code === 404
          if (missing && recoverMissingSession) {
            startHLS(curTime() || 0, playIntent, false).catch(error => showError(error.message))
            return
          }
          instance.stopLoad()
          failTranscode(missing ? '转码会话已结束，请重试' : undefined)
        })
        instance.loadSource(r.url)
        instance.attachMedia(vid)
      } else {
        nativeSource = { url: r.url, generation, recoverMissingSession }
        vid.src = r.url
      }
      // hls.js clears existing text-track cues when attaching media. Recreate
      // the selected track after attachment, using this session's time origin.
      refreshCC(true)
      playIntent = pending.shouldPlay
      pendingHlsSeek = null
      sourceChanging = false
      if (playIntent) tryPlay()
      else { vid.pause(); root.classList.remove('loading') }
    } catch (error) {
      if (destroyed || generation !== sourceGeneration) return
      sourceChanging = false
      stopSession()
      throw error
    }
  }

  function startDirect(startAt) {
    cancelHlsSeek()
    ++sourceGeneration
    pendingHlsSeek = null
    sourceChanging = false
    destroyHLS()
    stopSession()
    mode = 'direct'
    offset = 0
    playIntent = true
    directStartAt = Math.max(0, startAt || 0)
    refreshCC()
    vid.src = meta.stream
    tryPlay()
  }

  function showError(msg) {
    root.classList.remove('loading')
    root.classList.add('error')
    errText.textContent = msg
  }

  function failTranscode(message = '转码播放失败，请重试') {
    stopSession()
    vid.pause()
    showError(message)
  }

  async function handleNativeHlsError() {
    const source = nativeSource
    nativeSource = null
    const target = curTime() || 0
    const shouldPlay = playIntent
    let missing = false
    if (source.recoverMissingSession) {
      // Native media errors do not expose the HTTP status of the playlist.
      const controller = new AbortController()
      const timeout = setTimeout(() => controller.abort(), 3000)
      try {
        const response = await fetch(source.url, { method: 'HEAD', cache: 'no-store', signal: controller.signal })
        missing = response.status === 404
      } catch {} finally { clearTimeout(timeout) }
    }
    if (source.generation !== sourceGeneration || !session) return
    if (missing) {
      startHLS(target, shouldPlay, false).catch(error => showError(error.message))
      return
    }
    failTranscode()
  }

  // ---------- 进度 ----------
  function updateProgressUI(cur, frac) {
    const f = clamp(frac, 0, 1)
    playedEl.style.width = f * 100 + '%'
    knob.style.left = f * 100 + '%'
    tCur.textContent = fmtDur(cur) || '0:00'
  }

  function seekTo(t) {
    const total = totalDur()
    t = clamp(t, 0, Math.max(0, total - 0.5))
    if (mode === 'direct') {
      vid.currentTime = t
      updateProgressUI(t, total ? t / total : 0)
    } else {
      updateProgressUI(t, total ? t / total : 0)
      const shouldPlay = pendingHlsSeek ? pendingHlsSeek.shouldPlay : !vid.paused
      ++sourceGeneration // 立即使进行中的旧请求失效，不等防抖计时结束
      pendingHlsSeek = { target: t, shouldPlay }
      debouncedHlsSeek(t, shouldPlay)
    }
  }
  function debouncedHlsSeek(t, shouldPlay) {
    cancelHlsSeek()
    hlsSeekTimer = setTimeout(() => {
      hlsSeekTimer = null
      if (!destroyed) startHLS(t, shouldPlay).catch(e => showError(e.message))
    }, 700)
  }

  seek.addEventListener('input', () => {
    seeking = true
    const total = totalDur()
    const frac = seek.value / 1000
    updateProgressUI(frac * total, frac)
  })
  seek.addEventListener('change', () => {
    seeking = false
    seekTo(seek.value / 1000 * totalDur())
  })

  // ---------- 断点续播 ----------
  const pKey = `hestia.progress.${id}`
  const getSaved = () => { try { return JSON.parse(localStorage.getItem(pKey)) } catch { return null } }
  function saveProgress(useBeacon = false) {
    const t = curTime()
    if (!Number.isFinite(t) || t < 5 || (vid.ended && !pendingHlsSeek)) return
    const d = totalDur()
    try {
      localStorage.setItem(pKey, JSON.stringify({ t, d, at: Date.now() }))
    } catch {}
    const body = JSON.stringify({ mediaId: id, position: t, duration: d, device, name: meta.name, folder: meta.folderName })
    if (useBeacon) {
      try {
        if (navigator.sendBeacon('/api/progress', new Blob([body], { type: 'application/json' }))) return
      } catch {}
    }
    // 多设备同步：服务端保存（最后写入优先）
    fetch('/api/progress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
      keepalive: true,
    }).catch(() => {})
  }
  // 合并本地与服务端断点（较新者胜）
  async function mergedResume() {
    let sv = null
    try { sv = await api(`/api/progress/${id}`) } catch {}
    const lc = getSaved()
    const st = sv && sv.updatedAt ? sv.updatedAt : 0
    const lt = lc && lc.at ? lc.at : 0
    if (st >= lt && sv && sv.position > 30) return sv.position
    if (lt > 0 && lc && lc.t > 30) return lc.t
    return 0
  }
  async function maybeResumeTip() {
    if (resumeShown || seeking) return
    const savedT = await mergedResume()
    const total = totalDur()
    if (savedT > 30 && total && total - savedT > 30) {
      resumeShown = true
      resumeText.textContent = `上次看到 ${fmtDur(savedT)}`
      resumeTip.hidden = false
      showControls()
      setTimeout(() => { resumeTip.hidden = true }, 10000)
    }
  }
  $('#resume-go').addEventListener('click', () => { resumeTip.hidden = true; mergedResume().then(t => seekTo(t || 0)) })
  $('#resume-no').addEventListener('click', () => { resumeTip.hidden = true })

  // ---------- video 事件 ----------
  vid.addEventListener('loadedmetadata', () => {
    tDur.textContent = fmtDur(totalDur()) || '--:--'
    if (mode === 'direct' && directStartAt > 0) {
      vid.currentTime = Math.min(directStartAt, (vid.duration || 0) - 5)
      directStartAt = 0
    }
    maybeResumeTip()
  })
  vid.addEventListener('timeupdate', () => {
    if (!seeking) {
      const total = totalDur()
      updateProgressUI(curTime(), total ? curTime() / total : 0)
      seek.value = total ? Math.round(curTime() / total * 1000) : 0
    }
    const n = Date.now()
    if (n - lastSave > 5000) { lastSave = n; saveProgress() }
  })
  vid.addEventListener('progress', () => {
    try {
      if (vid.buffered.length && totalDur() > 0) {
        const end = vid.buffered.end(vid.buffered.length - 1)
        const abs = mode === 'direct' ? end : offset + end
        bufEl.style.width = clamp(abs / totalDur(), 0, 1) * 100 + '%'
      }
    } catch {}
  })
  vid.addEventListener('waiting', () => {
    if (root.classList.contains('error')) return
    root.classList.add('loading')
    spinText.textContent = '缓冲中…'
  })
  vid.addEventListener('playing', () => { root.classList.remove('loading', 'error'); chip.textContent = '转码'; showControls() })
  vid.addEventListener('canplay', () => root.classList.remove('loading'))
  vid.addEventListener('play', () => {
    if (!sourceChanging && !pageHidden && !vid.paused) {
      playIntent = true
      if (pendingHlsSeek) pendingHlsSeek.shouldPlay = true
    }
    showControls()
  })
  vid.addEventListener('pause', () => {
    // A failed media request can pause the element while playback is still wanted.
    if (!sourceChanging && !pageHidden && vid.paused && !vid.error && !vid.ended) {
      playIntent = false
      if (pendingHlsSeek) pendingHlsSeek.shouldPlay = false
    }
    showControls()
    if (!playbackStopped) saveProgress()
  })
  vid.addEventListener('ended', () => {
    localStorage.removeItem(pKey)
    fetch(`/api/progress/${id}`, { method: 'DELETE', keepalive: true }).catch(() => {})
    if (meta.next) toast(`已播完 · 下一个：《${meta.next.name}》`)
  })
  vid.addEventListener('error', () => {
    if (!vid.error || sourceChanging || destroyed || playbackStopped || pageHidden) return
    if (mode === 'hls') {
      // MSE errors are handled by hls.js. Native HLS reports errors here.
      if (nativeSource && !pendingHlsSeek) handleNativeHlsError()
      return
    }
    if (!meta.playback.transcodeAvailable) { showError('该格式浏览器无法播放，且服务端缺少 ffmpeg'); return }
    if (failoverTried) { showError('播放失败：浏览器无法解码该文件'); return }
    failoverTried = true
    toast('直连播放失败，已切换转码模式…')
    startHLS(curTime() || 0).catch(e => showError(e.message))
  })

  // ---------- 控件 ----------
  $('#btn-back').addEventListener('click', () => {
    if (history.length > 1) history.back()
    else location.hash = folder || home
  })
  if (meta.prev) $('#btn-prev').addEventListener('click', () => { location.hash = withHome(`#/play/${meta.prev.id}`, home) })
  if (meta.next) $('#btn-next').addEventListener('click', () => { location.hash = withHome(`#/play/${meta.next.id}`, home) })
  function toggleFS() {
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
    else {
      root.requestFullscreen?.().then(() => {
        screen.orientation?.lock?.('landscape').catch(() => {})
      }).catch(() => {})
    }
  }

  $('#err-retry').addEventListener('click', () => {
    if (playbackStopped) window.location.reload()
    else startHLS(curTime() || 0).catch(e => showError(e.message))
  })
  // 点击字幕菜单以外的任意位置时收起。
  root.addEventListener('click', e => {
    if (!e.target.closest('.xgplayer-playbackrate, .xg-options-list')) closeSpeedMenu()
    if (!ccMenu.hidden && !e.target.closest('#cc-menu, #btn-cc')) {
      ccMenu.hidden = true
      showControls()
    }
  }, true)

  // ---------- 字幕（外挂默认开启；内嵌按需提取） ----------
  const ccMenu = root.querySelector('#cc-menu')
  const btnCC = root.querySelector('#btn-cc')
  function subtitleURL(trk) {
    const base = trk.kind === 'ext' ? `/api/subtitle/${trk.id}` : `/api/media/${id}/embeddedsub/${trk.index}`
    return mode === 'hls' && offset > 0 ? `${base}?offset=${encodeURIComponent(offset)}` : base
  }
  function ensureTrackEl(trk) {
    if (trk.el) return trk.el
    const t = document.createElement('track')
    t.kind = 'captions'
    t.label = trk.label
    t.srclang = 'zh'
    t.addEventListener('load', () => {
      // Keep auto-positioned native captions above our custom control bar.
      for (const cue of t.track.cues || []) if (cue.line === 'auto') cue.line = -6
    })
    t.src = subtitleURL(trk)
    vid.appendChild(t)
    trk.el = t
    return t
  }
  function refreshCC(recreate = false) {
    for (const trk of ccTracks) {
      if (!trk.el) continue
      if (recreate) {
        trk.el.remove()
        trk.el = null
        continue
      }
      const url = subtitleURL(trk)
      if (trk.el.getAttribute('src') !== url) {
        trk.el.track.mode = 'disabled'
        trk.el.src = url
      }
    }
    setCC(activeCC)
  }
  function setCC(idx) {
    activeCC = idx
    for (let i = 0; i < vid.textTracks.length; i++) vid.textTracks[i].mode = 'disabled'
    if (idx >= 0) {
      const el = ensureTrackEl(ccTracks[idx])
      if (el.track) el.track.mode = 'showing'
      btnCC.classList.remove('dim')
    } else {
      btnCC.classList.add('dim')
    }
    renderCCMenu()
  }
  function renderCCMenu() {
    if (!ccTracks.length) return
    const items = [`<button data-i="-1" class="${activeCC < 0 ? 'active' : ''}">关闭字幕</button>`]
      .concat(ccTracks.map((t, i) => `<button data-i="${i}" class="${i === activeCC ? 'active' : ''}">${esc(t.label)}</button>`))
      .join('')
    ccMenu.innerHTML = `<div style="font-size:11px;color:var(--muted);padding:2px 10px 8px">字幕</div>${items}`
  }
  renderCCMenu()
  if (ccTracks.length) {
    if (activeCC < 0) btnCC.classList.add('dim')
    btnCC.addEventListener('click', e => {
      e.stopPropagation()
      ccMenu.hidden = !ccMenu.hidden
      showControls()
    })
    ccMenu.addEventListener('click', e => {
      const b = e.target.closest('button[data-i]')
      if (!b) return
      setCC(parseInt(b.dataset.i, 10))
      ccMenu.hidden = true
      showControls()
    })
  }

  // 音量（桌面 / 非iOS）
  if (!isIOS) {
    vid.addEventListener('volumechange', () => localStorage.setItem('hestia.volume', String(vid.volume)))
  } else if (!localStorage.getItem('hestia.ioshint')) {
    localStorage.setItem('hestia.ioshint', '1')
    setTimeout(() => toast('iOS 上请用音量键调音量；右侧上下滑动可快进/倒退', 4200), 800)
  }

  // ---------- 手势 ----------
  let ptr = null
  let tapInfo = null

  gesture.addEventListener('pointerdown', e => {
    if (e.pointerType === 'mouse' && e.button !== 0) return
    try { gesture.setPointerCapture(e.pointerId) } catch {}
    ptr = {
      id: e.pointerId, x0: e.clientX, y0: e.clientY,
      mode: null, vh: null, moved: false, lp: false,
      startCur: curTime() || 0,
      startVol: vid.volume, startBr: brightness, seekTarget: 0,
      lpTimer: setTimeout(() => {
        if (!ptr || ptr.moved) return
        ptr.lp = true
        lpPrevRate = vid.playbackRate
        vid.playbackRate = 2
        showHUD('play', '2x 快进中', null)
      }, 500),
    }
  })

  gesture.addEventListener('pointermove', e => {
    const g = ptr
    if (!g || e.pointerId !== g.id) return
    const dx = e.clientX - g.x0, dy = e.clientY - g.y0
    const adx = Math.abs(dx), ady = Math.abs(dy)
    if (!g.moved && (adx > 10 || ady > 10)) {
      g.moved = true
      if (!g.lp) clearTimeout(g.lpTimer)
    }
    if (g.lp) return
    if (!g.mode) {
      if (ady > 12 && ady > adx * 1.2) {
        g.mode = e.clientX - root.getBoundingClientRect().left < root.clientWidth / 2 ? 'bright' : (isIOS ? 'vseek' : 'vol')
      } else if (adx > 18 && adx > ady * 1.2) {
        g.mode = 'seek'
        g.vh = 'h'
      }
    }
    if (!g.mode) return
    const H = root.clientHeight
    if (g.mode === 'bright') {
      brightness = clamp(g.startBr - dy / (H * 0.6), 0.2, 1)
      applyBrightness()
      showHUD('sun', Math.round(brightness * 100) + '%', (brightness - 0.2) / 0.8 * 100)
    } else if (g.mode === 'vol') {
      vid.volume = clamp(g.startVol - dy / (H * 0.6), 0, 1)
      vid.muted = false
      showHUD(vid.volume === 0 ? 'volMute' : 'vol', Math.round(vid.volume * 100) + '%', vid.volume * 100)
    } else if (g.mode === 'seek' || g.mode === 'vseek') {
      const d = g.vh === 'h' ? dx * 0.25 : -dy * 0.12
      g.seekTarget = clamp(g.startCur + d, 0, Math.max(0, totalDur() - 0.5))
      const delta = g.seekTarget - g.startCur
      const sign = delta >= 0 ? '+' : ''
      showHUD(delta >= 0 ? 'fwd' : 'back', `${sign}${Math.round(delta)}s → ${fmtDur(g.seekTarget) || '0:00'}`, null)
    }
  })

  function endPointer(e, canceled) {
    const g = ptr
    if (!g || e.pointerId !== g.id) return
    ptr = null
    clearTimeout(g.lpTimer)
    if (g.lp) {
      vid.playbackRate = lpPrevRate
      hideHUD(400)
      return
    }
    if (g.mode === 'seek' || g.mode === 'vseek') {
      if (g.seekTarget) seekTo(g.seekTarget)
      hideHUD(700)
    } else if (g.mode === 'bright') {
      localStorage.setItem('hestia.brightness', String(brightness))
      hideHUD(700)
    } else if (g.mode === 'vol') {
      hideHUD(700)
    } else if (!g.moved && !canceled) {
      if (e.pointerType === 'mouse') {
        togglePlay()
        showControls()
      }
      else handleTap(e.clientX - root.getBoundingClientRect().left)
    }
  }
  gesture.addEventListener('pointerup', e => endPointer(e, false))
  gesture.addEventListener('pointercancel', e => endPointer(e, true))
  gesture.addEventListener('contextmenu', e => e.preventDefault())

  function handleTap(x) {
    const now = performance.now()
    if (tapInfo && now - tapInfo.t < 300 && Math.abs(x - tapInfo.x) < 70) {
      clearTimeout(tapInfo.timer)
      tapInfo = null
      const w = root.clientWidth
      if (x < w / 3) { seekTo(curTime() - 10); showHUD('back', '-10s', null); hideHUD(700) }
      else if (x > w * 2 / 3) { seekTo(curTime() + 10); showHUD('fwd', '+10s', null); hideHUD(700) }
      else togglePlay()
      return
    }
    if (tapInfo) clearTimeout(tapInfo.timer)
    tapInfo = {
      t: now, x,
      timer: setTimeout(() => { toggleControls(); tapInfo = null }, 260),
    }
  }

  // 鼠标移动唤出控制栏
  root.addEventListener('pointermove', e => {
    if (e.pointerType === 'mouse' && e.target.closest('.p-top, .p-bottom, .xgplayer-controls, .sub-menu') === null) showControls()
  })

  // ---------- 键盘 ----------
  const onKey = e => {
    if (playbackStopped || e.target?.closest?.('input, textarea, select, dialog')) return
    switch (e.key) {
      case ' ': case 'k': e.preventDefault(); togglePlay(); break
      case 'ArrowLeft': seekTo(curTime() - 5); break
      case 'ArrowRight': seekTo(curTime() + 5); break
      case 'j': seekTo(curTime() - 10); break
      case 'l': seekTo(curTime() + 10); break
      case 'ArrowUp': if (!isIOS) { vid.volume = clamp(vid.volume + 0.05, 0, 1); showHUD('vol', Math.round(vid.volume * 100) + '%', vid.volume * 100); hideHUD(700) } e.preventDefault(); break
      case 'ArrowDown': if (!isIOS) { vid.volume = clamp(vid.volume - 0.05, 0, 1); showHUD('vol', Math.round(vid.volume * 100) + '%', vid.volume * 100); hideHUD(700) } e.preventDefault(); break
      case 'f': toggleFS(); break
      case 'Escape':
        ccMenu.hidden = true
        closeSpeedMenu()
        if (player.isCssfullScreen) player.exitCssFullscreen()
        break
    }
  }
  document.addEventListener('keydown', onKey)

  // ---------- 生命周期 ----------
  const onPageHide = () => {
    pageHidden = true
    ++sourceGeneration
    cancelHlsSeek()
    stopSession()
    if (!playbackStopped) saveProgress(true)
  }
  const onPageShow = e => {
    if (!e.persisted || !pageHidden || destroyed || playbackStopped) return
    pageHidden = false
    if (mode === 'hls' || sourceChanging) {
      const shouldPlay = pendingHlsSeek ? pendingHlsSeek.shouldPlay : !vid.paused
      startHLS(curTime() || 0, shouldPlay).catch(error => showError(error.message))
    }
  }
  window.addEventListener('pagehide', onPageHide)
  window.addEventListener('pageshow', onPageShow)

  function releasePlayback() {
    if (playbackStopped) return
    playbackStopped = true
    ++sourceGeneration
    cancelHlsSeek()
    saveProgress()
    destroyHLS()
    stopSession()
    vid.pause()
    vid.removeAttribute('src')
    vid.load()
  }

  const deleteButton = detail.querySelector('#detail-delete')
  const disposeDelete = deleteButton && mountFolderDelete(detail, deleteButton, meta.folderId, () => {
    releasePlayback()
    root.classList.remove('loading')
    showError('播放已停止')
    $('#err-retry').textContent = '重新打开视频'
  })

  // ---------- 启动 ----------
  if (meta.playback.mode === 'transcode') {
    startHLS(0).catch(e => showError(e.message))
  } else {
    startDirect(0)
  }

  return {
    destroy() {
      destroyed = true
      releasePlayback()
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('pagehide', onPageHide)
      window.removeEventListener('pageshow', onPageShow)
      disposeDelete?.()
      player.destroy()
      if (document.fullscreenElement === root) document.exitFullscreen().catch(() => {})
      clearTimeout(controlsTimer)
      clearTimeout(hudTimer)
      clearTimeout(ptr?.lpTimer)
      clearTimeout(tapInfo?.timer)
      detail.remove()
    },
  }
}
