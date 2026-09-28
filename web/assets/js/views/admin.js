// 管理页：媒体库增删改 + 服务配置 + 扫描状态 + ffmpeg 状态（免登录，局域网自用）
import { api, post, patch, del } from '../api.js'
import { icon, esc, toast, fmtSize } from '../util.js'

export async function AdminView(app) {
  app.innerHTML = `<div class="admin-wrap">
    <div class="page-head"><h2>管理</h2><span class="spacer"></span>
      <a class="btn ghost sm" href="#/">${icon('back', 15)} 返回首页</a>
    </div>

    <div class="panel">
      <h3>服务</h3>
      <p class="desc">修改端口 / 监听方式后立即生效（不中断正在进行的播放连接）</p>
      <div id="svc-body">加载中…</div>
    </div>

    <div class="panel">
      <h3>媒体库 <button class="btn sm" id="btn-rescan" style="margin-left:auto">${icon('refresh', 14)} 全部重扫</button></h3>
      <p class="desc">添加视频根路径（母文件夹所在目录），删除/新增后自动增量生效，无需重启</p>
      <div class="row" style="margin-bottom:16px">
        <div class="field"><label>路径（如 D:\\电影）</label><input class="ipt" id="add-path" placeholder="D:\\电影" spellcheck="false"></div>
        <div class="field" style="max-width:180px"><label>标签（可选）</label><input class="ipt" id="add-label" placeholder="电影"></div>
        <button class="btn" id="btn-add">${icon('plus', 15)} 添加</button>
      </div>
      <div class="lib-list" id="lib-list"></div>
    </div>

    <div class="panel">
      <h3>运行环境</h3>
      <div id="env-body">加载中…</div>
    </div>
  </div>`

  const $ = s => app.querySelector(s)

  // ---- 服务设置 ----
  async function loadSvc() {
    const c = await api('/api/admin/config')
    $('#svc-body').innerHTML = `
      <div class="row">
        <div class="field" style="max-width:140px"><label>端口</label><input class="ipt" id="cfg-port" type="number" min="1" max="65535" value="${c.port}"></div>
        <div class="field" style="max-width:220px"><label>监听</label>
          <select class="ipt" id="cfg-listen">
            <option value="0.0.0.0" ${c.listen === '0.0.0.0' ? 'selected' : ''}>0.0.0.0（局域网可访问）</option>
            <option value="127.0.0.1" ${c.listen === '127.0.0.1' ? 'selected' : ''}>127.0.0.1（仅本机）</option>
          </select>
        </div>
        <div class="field" style="max-width:200px"><label>启动时打开浏览器</label>
          <label class="switch"><input type="checkbox" id="cfg-open" ${c.openBrowser ? 'checked' : ''}><i></i></label>
        </div>
        <div class="field" style="max-width:200px"><label>开机自启（Windows）</label>
          <label class="switch"><input type="checkbox" id="cfg-auto" ${c.autoStart ? 'checked' : ''}><i></i></label>
        </div>
        <button class="btn" id="btn-svc-save">应用</button>
      </div>
      <div class="row" style="margin-top:14px;align-items:flex-start">
        <div style="flex:1;min-width:240px">
          <div class="kv"><b>访问地址</b><span>${c.urls.map(u => `<a href="${u}" style="color:var(--accent)">${u}</a>`).join('<br>') || '-'}</span></div>
        </div>
        <div style="text-align:center">
          <img src="/api/admin/qrcode" alt="扫码访问" width="164" height="164" style="border-radius:12px;background:#fff;padding:8px">
          <div style="font-size:12px;color:var(--muted);margin-top:6px">手机扫码直达</div>
        </div>
      </div>`
    $('#btn-svc-save').addEventListener('click', async () => {
      try {
        const r = await patch('/api/admin/config', {
          port: parseInt($('#cfg-port').value, 10),
          listen: $('#cfg-listen').value,
          openBrowser: $('#cfg-open').checked,
          autoStart: $('#cfg-auto').checked,
        })
        toast('已保存' + (r.note ? '：' + r.note : ''))
        if (r.port && location.port !== String(r.port)) {
          toast(`端口已切换为 ${r.port}，本页面即将失效，请通过新地址访问`, 'info', 6000)
        }
        loadSvc()
      } catch (e) { toast(e.message, 'err') }
    })
  }

  // ---- 媒体库列表 ----
  let pollTimer = null
  async function loadLibs() {
    const d = await api('/api/admin/status')
    const list = $('#lib-list')
    if (!d.libraries.length) {
      list.innerHTML = `<div class="empty-state" style="padding:30px">${icon('folder', 36)}<p>还没有添加媒体库</p><p class="hint">在上方输入视频文件夹路径</p></div>`
    } else {
      list.innerHTML = d.libraries.map(l => {
        const dotCls = l.scanning ? 'scanning' : (!l.enabled ? 'off' : (l.lastErr ? 'err' : ''))
        const status = l.scanning ? `扫描中… ${l.files} 项` : (l.lastErr ? `错误：${esc(l.lastErr)}` : `${l.files} 文件 · ${l.dirs} 文件夹`)
        return `<div class="lib-item" data-id="${l.id}">
          <span class="dot ${dotCls}"></span>
          <div class="lib-main">
            <div class="lib-path">${esc(l.label)} <span style="color:var(--muted);font-weight:400">${esc(l.path)}</span></div>
            <div class="lib-sub">${status}${l.lastScan && !l.scanning ? ` · 上次扫描 ${new Date(l.lastScan).toLocaleString('zh-CN')}` : ''}</div>
          </div>
          <label class="switch" title="启用/停用"><input type="checkbox" class="lib-en" ${l.enabled ? 'checked' : ''}><i></i></label>
          <button class="btn danger lib-del">${icon('trash', 14)} 移除</button>
        </div>`
      }).join('')

      list.querySelectorAll('.lib-item').forEach(item => {
        const id = item.dataset.id
        item.querySelector('.lib-en').addEventListener('change', async e => {
          try { await patch(`/api/admin/libraries/${id}`, { enabled: e.target.checked }); toast('已更新') } catch (err) { toast(err.message, 'err') }
          schedulePoll()
        })
        item.querySelector('.lib-del').addEventListener('click', async () => {
          if (!confirm('从 Hestia 移除该媒体库？（只移除索引，不删除任何文件）')) return
          try { await del(`/api/admin/libraries/${id}`); toast('已移除') } catch (err) { toast(err.message, 'err') }
          loadLibs()
        })
      })
    }
    // 扫描中持续轮询
    clearTimeout(pollTimer)
    if (d.scanning) pollTimer = setTimeout(loadLibs, 1000)
    loadEnv()
  }
  function schedulePoll() { clearTimeout(pollTimer); pollTimer = setTimeout(loadLibs, 400) }

  $('#btn-add').addEventListener('click', async () => {
    const path = $('#add-path').value.trim()
    if (!path) { toast('请填写路径', 'err'); return }
    try {
      await post('/api/admin/libraries', { path, label: $('#add-label').value.trim() })
      toast('已添加，后台扫描中…')
      $('#add-path').value = ''
      $('#add-label').value = ''
      loadLibs()
    } catch (e) { toast(e.message, 'err') }
  })

  $('#btn-rescan').addEventListener('click', async () => {
    try { await post('/api/admin/rescan'); toast('已开始重新扫描') ; loadLibs() } catch (e) { toast(e.message, 'err') }
  })

  // ---- 运行环境 ----
  async function loadEnv() {
    const [info, st] = await Promise.all([api('/api/server/info'), api('/api/admin/status')])
    const ffLine = info.ffmpeg
      ? `<span class="ok-text">已就绪（${esc(info.ffmpegPath)}）</span>`
      : `<span class="bad-text">未检测到</span> — <a href="https://www.gyan.dev/ffmpeg/builds/" target="_blank" rel="noreferrer" style="color:var(--accent)">下载 ffmpeg</a>（解压后放程序同目录或加入 PATH，重启生效）`
    $('#env-body').innerHTML = `
      <div class="kv"><b>版本</b><span>Hestia v${esc(info.version)}</span></div>
      <div class="kv"><b>索引规模</b><span>${st.totals.folders} 个文件夹 · ${st.totals.media} 个媒体项</span></div>
      <div class="kv"><b>ffmpeg</b><span>${ffLine}</span></div>
      <div class="kv"><b>ffprobe</b><span class="${info.ffprobe ? 'ok-text' : 'bad-text'}">${info.ffprobe ? '已就绪（用于识别编码/时长/字幕轨）' : '未检测到'}</span></div>
      <div class="kv"><b>活跃转码</b><span>${st.transcodes} 个会话</span></div>`
  }

  await loadSvc()
  await loadLibs()

  return {
    destroy() { clearTimeout(pollTimer) },
  }
}
