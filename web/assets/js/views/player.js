// 播放器页：手势 + 倍速 + 断点续播 + 转码兜底
import { api, post } from '../api.js'
import { icon, fmtDur, toast, clamp, esc, debounce } from '../util.js'

const SPEEDS = [0.5, 1, 1.5, 2]
const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
  (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)

export async function PlayerView(app, id) {
  document.body.classList.add('player-mode')
  const meta = await api(`/api/media/${id}`).catch(e => {
    throw new Error(e.message || '视频不存在或已被移除')
  })
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
    <video playsinline webkit-playsinline preload="auto"></video>
    <div id="gesture"></div>
    <div class="hud" id="hud"><span id="hud-ico"></span><span class="hud-text" id="hud-text"></span><div class="hud-bar" id="hud-bar" style="display:none"><i id="hud-bar-i"></i></div></div>
    <div class="p-top">
      <button class="pbtn" id="btn-back" title="返回">${icon('back', 21)}</button>
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
      <div class="ctl-row">
        ${meta.prev ? `<button class="pbtn" id="btn-prev" title="上一个">${icon('back', 17)}</button>` : ''}
        <button class="pbtn" id="btn-play" title="播放/暂停">${icon('play', 22)}</button>
        ${meta.next ? `<button class="pbtn" id="btn-next" title="下一个">${icon('fwd', 17)}</button>` : ''}
        <span class="spacer"></span>
        <button class="speed-label" id="btn-speed" title="倍速">1x</button>
        <button class="pbtn ${ccTracks.length ? '' : 'hide'}" id="btn-cc" title="字幕">${icon('cc', 20)}</button>
        ${isIOS ? '' : `<span class="vol-wrap">
          <button class="pbtn" id="btn-mute" title="静音">${icon('vol', 19)}</button>
          <input type="range" id="vol" min="0" max="1" step="0.01" value="1" title="音量">
        </span>`}
        <button class="pbtn" id="btn-fs" title="全屏">${icon('expand', 19)}</button>
      </div>
    </div>
    <div class="speed-menu" id="speed-menu" hidden></div>
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
  app.appendChild(root)

  const $ = s => root.querySelector(s)
  const vid = $('video'), gesture = $('#gesture'), hud = $('#hud')
  const hudIco = $('#hud-ico'), hudText = $('#hud-text'), hudBar = $('#hud-bar'), hudBarI = $('#hud-bar-i')
  const seek = $('#seek'), playedEl = $('#played'), bufEl = $('#buf'), knob = $('#knob')
  const tCur = $('#t-cur'), tDur = $('#t-dur')
  const btnPlay = $('#btn-play'), speedMenu = $('#speed-menu'), chip = $('#tc-chip')
  const spinText = $('#spin-text'), errText = $('#err-text')
  const resumeTip = $('#resume-tip'), resumeText = $('#resume-text')

  let mode = 'direct'          // direct | hls
  let hls = null
  let session = null
  let offset = 0
  let failoverTried = false
  let rate = 1
  let lpPrevRate = 1
  let brightness = clamp(parseFloat(localStorage.getItem('hestia.brightness') || '1') || 1, 0.2, 1)
  let directStartAt = 0
  let seeking = false
  let resumeShown = false
  let lastSave = 0
  let destroyed = false

  // ---------- 基础 ----------
  const totalDur = () => mode === 'direct' ? (vid.duration || 0) : (meta.duration > 0 ? meta.duration : offset + (vid.duration || 0))
  const curTime = () => mode === 'direct' ? vid.currentTime : offset + vid.currentTime

  function applyBrightness() { vid.style.filter = `brightness(${brightness})` }
  applyBrightness()

  function tryPlay() { vid.play().catch(() => showControls()) }

  function togglePlay() { vid.paused ? tryPlay() : vid.pause() }

  function setRate(s) {
    rate = s
    vid.playbackRate = s
    $('#btn-speed').textContent = s + 'x'
    speedMenu.querySelectorAll('button').forEach(b => b.classList.toggle('active', parseFloat(b.dataset.s) === s))
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
  function showControls() {
    root.classList.remove('controls-hidden')
    clearTimeout(controlsTimer)
    controlsTimer = setTimeout(() => {
      if (!vid.paused && speedMenu.hidden && ccMenu.hidden && resumeTip.hidden) root.classList.add('controls-hidden')
    }, 3000)
  }
  function toggleControls() {
    if (root.classList.contains('controls-hidden')) showControls()
    else { root.classList.add('controls-hidden'); speedMenu.hidden = true; ccMenu.hidden = true }
  }

  // ---------- 播放源 ----------
  function stopSession() {
    if (session) {
      const sid = session
      fetch(`/api/transcode/${sid}`, { method: 'DELETE', keepalive: true }).catch(() => {})
      session = null
    }
  }
  function destroyHLS() { if (hls) { try { hls.destroy() } catch {} hls = null } }

  async function ensureHls() {
    if (window.Hls) return window.Hls
    await new Promise((res, rej) => {
      const s = document.createElement('script')
      s.src = '/assets/js/vendor/hls.min.js'
      s.onload = res
      s.onerror = () => rej(new Error('hls.js 加载失败'))
      document.head.appendChild(s)
    })
    if (!window.Hls) throw new Error('hls.js 加载失败')
    return window.Hls
  }

  async function startHLS(startAt) {
    if (!meta.playback.transcodeAvailable) throw new Error('服务端未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）')
    destroyHLS()
    stopSession()
    if (destroyed) return
    root.classList.add('loading')
    spinText.textContent = '转码启动中…'
    chip.hidden = false
    chip.textContent = '转码中…'
    const r = await post(`/api/media/${id}/transcode`, { start: Math.max(0, startAt || 0) })
    session = r.sessionId
    offset = r.start
    mode = 'hls'
    if (vid.canPlayType('application/vnd.apple.mpegurl')) {
      vid.src = r.url
    } else {
      const Hls = await ensureHls()
      if (!Hls.isSupported()) throw new Error('浏览器不支持 HLS 播放')
      hls = new Hls({ maxBufferLength: 30 })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal) return
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad()
        else showError('转码播放失败：' + (data.details || ''))
      })
      hls.loadSource(r.url)
      hls.attachMedia(vid)
    }
    tryPlay()
  }

  function startDirect(startAt) {
    mode = 'direct'
    offset = 0
    directStartAt = Math.max(0, startAt || 0)
    vid.src = meta.stream
    tryPlay()
  }

  function showError(msg) {
    root.classList.remove('loading')
    root.classList.add('error')
    errText.textContent = msg
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
      debouncedHlsSeek(t)
    }
  }
  const debouncedHlsSeek = debounce(t => {
    if (!destroyed) startHLS(t).catch(e => showError(e.message))
  }, 700)

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
  function saveProgress() {
    const t = curTime()
    if (!t || !isFinite(t) || t < 5) return
    const d = totalDur()
    try {
      localStorage.setItem(pKey, JSON.stringify({ t, d, at: Date.now() }))
    } catch {}
    // 多设备同步：服务端保存（最后写入优先）
    fetch('/api/progress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mediaId: id, position: t, duration: d, device, name: meta.name, folder: meta.folderName }),
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
  vid.addEventListener('waiting', () => { root.classList.add('loading'); spinText.textContent = '缓冲中…' })
  vid.addEventListener('playing', () => { root.classList.remove('loading', 'error'); chip.textContent = '转码'; showControls() })
  vid.addEventListener('canplay', () => root.classList.remove('loading'))
  vid.addEventListener('play', () => { btnPlay.innerHTML = icon('pause', 22); showControls() })
  vid.addEventListener('pause', () => { btnPlay.innerHTML = icon('play', 22); showControls(); saveProgress() })
  vid.addEventListener('ended', () => {
    localStorage.removeItem(pKey)
    fetch(`/api/progress/${id}`, { method: 'DELETE', keepalive: true }).catch(() => {})
    if (meta.next) toast(`已播完 · 下一个：《${meta.next.name}》`)
  })
  vid.addEventListener('error', () => {
    if (mode !== 'direct' || !vid.error) return
    if (!meta.playback.transcodeAvailable) { showError('该格式浏览器无法播放，且服务端缺少 ffmpeg'); return }
    if (failoverTried) { showError('播放失败：浏览器无法解码该文件'); return }
    failoverTried = true
    toast('直连播放失败，已切换转码模式…')
    startHLS(curTime() || 0).catch(e => showError(e.message))
  })

  // ---------- 控件 ----------
  btnPlay.addEventListener('click', togglePlay)
  $('#btn-back').addEventListener('click', () => {
    if (history.length > 1) history.back()
    else location.hash = '#/'
  })
  if (meta.prev) $('#btn-prev').addEventListener('click', () => { location.hash = `#/play/${meta.prev.id}` })
  if (meta.next) $('#btn-next').addEventListener('click', () => { location.hash = `#/play/${meta.next.id}` })
  $('#btn-fs').addEventListener('click', toggleFS)
  function toggleFS() {
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
    else {
      root.requestFullscreen?.().then(() => {
        screen.orientation?.lock?.('landscape').catch(() => {})
      }).catch(() => {})
    }
  }

  // 倍速菜单（点击按钮弹出，选中或点击其他区域后收起）
  speedMenu.innerHTML = SPEEDS.map(s => `<button data-s="${s}" class="${s === 1 ? 'active' : ''}">${s}x</button>`).join('')
  speedMenu.addEventListener('click', e => {
    const b = e.target.closest('button')
    if (!b) return
    setRate(parseFloat(b.dataset.s))
    speedMenu.hidden = true
    showControls()
  })
  function closeMenus() {
    speedMenu.hidden = true
    ccMenu.hidden = true
    showControls()
  }
  // 点击菜单/按钮以外的任意位置都收起（对齐主流播放器行为）
  root.addEventListener('click', e => {
    if (speedMenu.hidden && ccMenu.hidden) return
    if (e.target.closest('#speed-menu, #btn-speed, #cc-menu, #btn-cc')) return
    closeMenus()
  }, true)
  $('#btn-speed').addEventListener('click', e => {
    e.stopPropagation()
    ccMenu.hidden = true
    speedMenu.hidden = !speedMenu.hidden
    showControls()
  })

  // ---------- 字幕（外挂默认开启；内嵌按需提取） ----------
  const ccMenu = root.querySelector('#cc-menu')
  const btnCC = root.querySelector('#btn-cc')
  subExt.forEach((s, i) => {
    const t = document.createElement('track')
    t.kind = 'captions'
    t.label = s.name
    t.srclang = 'zh'
    t.src = `/api/subtitle/${s.id}`
    if (i === 0) t.default = true
    vid.appendChild(t)
    ccTracks[i].el = t
  })
  function ensureTrackEl(trk) {
    if (trk.el) return trk.el
    const t = document.createElement('track')
    t.kind = 'captions'
    t.label = trk.label
    t.srclang = 'zh'
    t.src = trk.kind === 'ext' ? `/api/subtitle/${trk.id}` : `/api/media/${id}/embeddedsub/${trk.index}`
    vid.appendChild(t)
    trk.el = t
    return t
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
      speedMenu.hidden = true
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
    const volInput = $('#vol'), btnMute = $('#btn-mute')
    vid.volume = clamp(parseFloat(localStorage.getItem('hestia.volume') ?? '1') || 1, 0, 1)
    const syncVol = () => {
      volInput.value = vid.volume
      volInput.style.setProperty('--v', vid.volume * 100 + '%')
      btnMute.innerHTML = icon(vid.muted || vid.volume === 0 ? 'volMute' : 'vol', 19)
    }
    volInput.addEventListener('input', () => { vid.volume = parseFloat(volInput.value); vid.muted = false; syncVol() })
    btnMute.addEventListener('click', () => { vid.muted = !vid.muted; syncVol() })
    vid.addEventListener('volumechange', () => { syncVol(); localStorage.setItem('hestia.volume', String(vid.volume)) })
    syncVol()
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
        lpPrevRate = rate
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
        g.mode = e.clientX < root.clientWidth / 2 ? 'bright' : (isIOS ? 'vseek' : 'vol')
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
      handleTap(e.clientX)
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
    if (e.pointerType === 'mouse' && e.target.closest('.p-top, .p-bottom, .speed-menu, .sub-menu') === null) showControls()
  })

  // ---------- 键盘 ----------
  const onKey = e => {
    if (e.target && /INPUT|TEXTAREA|SELECT/.test(e.target.tagName)) return
    switch (e.key) {
      case ' ': case 'k': e.preventDefault(); togglePlay(); break
      case 'ArrowLeft': seekTo(curTime() - 5); break
      case 'ArrowRight': seekTo(curTime() + 5); break
      case 'j': seekTo(curTime() - 10); break
      case 'l': seekTo(curTime() + 10); break
      case 'ArrowUp': if (!isIOS) { vid.volume = clamp(vid.volume + 0.05, 0, 1); showHUD('vol', Math.round(vid.volume * 100) + '%', vid.volume * 100); hideHUD(700) } e.preventDefault(); break
      case 'ArrowDown': if (!isIOS) { vid.volume = clamp(vid.volume - 0.05, 0, 1); showHUD('vol', Math.round(vid.volume * 100) + '%', vid.volume * 100); hideHUD(700) } e.preventDefault(); break
      case 'f': toggleFS(); break
      case 'Escape': speedMenu.hidden = true; ccMenu.hidden = true; break
    }
  }
  document.addEventListener('keydown', onKey)

  // ---------- 生命周期 ----------
  const onPageHide = () => {
    saveProgress()
    stopSession()
    // 页面关闭时尽力上报进度
    try {
      const t = curTime() || 0
      navigator.sendBeacon('/api/progress', new Blob([JSON.stringify({
        mediaId: id, position: t, duration: totalDur(), device, name: meta.name, folder: meta.folderName,
      })], { type: 'application/json' }))
    } catch {}
  }
  window.addEventListener('pagehide', onPageHide)

  // ---------- 启动 ----------
  if (meta.playback.mode === 'transcode') {
    startHLS(0).catch(e => showError(e.message))
  } else {
    startDirect(0)
  }

  return {
    destroy() {
      destroyed = true
      saveProgress()
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('pagehide', onPageHide)
      destroyHLS()
      stopSession()
      if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
      clearTimeout(controlsTimer)
      clearTimeout(hudTimer)
      root.remove()
    },
  }
}
