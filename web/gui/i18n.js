// Each message lists Simplified Chinese, English and Japanese in that order.
const consoleLanguages = ['zh-CN', 'en', 'ja']
const consoleMessages = {
  title: ['Hestia 控制台', 'Hestia Console', 'Hestia コンソール'],
  logs: ['日志', 'Logs', 'ログ'],
  settings: ['设置', 'Settings', '設定'],
  library: ['媒体库', 'Media library', 'メディアライブラリ'],
  addFolder: ['添加文件夹', 'Add folder', 'フォルダーを追加'],
  loadingLibrary: ['正在读取媒体库…', 'Loading media library…', 'ライブラリを読み込み中…'],
  rescan: ['重新扫描', 'Rescan', '再スキャン'],
  folderTable: ['媒体库文件夹', 'Library folders', 'ライブラリのフォルダー'],
  folder: ['文件夹', 'Folder', 'フォルダー'],
  contents: ['内容', 'Contents', '内容'],
  status: ['状态', 'Status', '状態'],
  actions: ['操作', 'Actions', '操作'],
  emptyLibrary: ['还没有媒体库', 'No folders yet', 'フォルダーがありません'],
  addFirstFolder: ['添加一个文件夹，即可开始扫描媒体。', 'Add a folder to start scanning for media.', 'フォルダーを追加するとメディアのスキャンが始まります。'],
  serviceStatus: ['服务状态', 'Server status', 'サーバーの状態'],
  loading: ['加载中…', 'Loading…', '読み込み中…'],
  startServer: ['启动服务', 'Start server', 'サービスを開始'],
  stopServer: ['停止服务', 'Stop server', 'サービスを停止'],
  access: ['访问地址', 'Addresses', 'アクセス先'],
  accessHint: ['查看本机和局域网地址', 'Show local and network addresses', 'ローカルと LAN のアドレスを表示'],
  copyAddress: ['复制访问地址', 'Copy address', 'アドレスをコピー'],
  openWeb: ['打开网页', 'Open web app', 'Web 画面を開く'],
  removeLibrary: ['移除媒体库', 'Remove library', 'ライブラリを削除'],
  close: ['关闭', 'Close', '閉じる'],
  folderPath: ['文件夹路径', 'Folder path', 'フォルダーのパス'],
  pathPlaceholder: ['输入媒体文件夹的路径', 'Enter the media folder path', 'メディアフォルダーのパスを入力'],
  browse: ['浏览…', 'Browse…', '参照…'],
  cancel: ['取消', 'Cancel', 'キャンセル'],
  add: ['添加', 'Add', '追加'],
  remove: ['移除', 'Remove', '削除'],
  port: ['服务端口', 'Server port', 'サービスポート'],
  apply: ['应用', 'Apply', '適用'],
  language: ['语言', 'Language', '言語'],
  autoStart: ['开机自启', 'Start on login', 'ログイン時に起動'],
  autoHint: ['登录 Windows 后自动运行', 'Run automatically when you sign in to Windows', 'Windows にサインインすると自動で起動します'],
  generateCovers: ['生成封面', 'Generate covers', 'カバー画像を生成'],
  coverIntro: ['为媒体库生成封面，已有缓存会直接复用。', 'Generate library covers and reuse existing cached images.', 'ライブラリのカバー画像を生成し、キャッシュ済みの画像は再利用します。'],
  coverStart: ['开始', 'Start', '開始'],
  coverStop: ['停止', 'Stop', '停止'],
  coverHide: ['收起', 'Hide', '閉じて続行'],
  coverView: ['查看封面进度', 'View cover progress', 'カバー画像の進捗を表示'],
  coverInBackground: ['封面正在后台生成，可随时查看进度。', 'Covers are generating in the background. You can check progress at any time.', 'カバー画像をバックグラウンドで生成中です。進捗はいつでも確認できます。'],
  coverProgress: ['封面生成进度', 'Cover generation progress', 'カバー画像生成の進行状況'],
  coverPreparing: ['正在准备封面列表…', 'Preparing the list of covers…', 'カバー画像の一覧を準備中…'],
  coverGenerating: ['正在生成封面…', 'Generating covers…', 'カバー画像を生成中…'],
  coverDone: ['封面生成完成', 'Cover generation complete', 'カバー画像の生成が完了しました'],
  coverEmpty: ['没有可处理的封面', 'No covers to process', '処理できるカバー画像がありません'],
  coverStopped: ['已停止生成，已完成的封面仍可使用。', 'Stopped. Covers already generated are available.', '生成を停止しました。生成済みの画像は引き続き使えます。'],
  coverInterrupted: ['封面生成中断，请重试。', 'Cover generation was interrupted. Try again.', 'カバー画像の生成が中断されました。もう一度お試しください。'],
  coverAlreadyRunning: ['已有封面生成任务正在执行或停止，请稍后重试。', 'A cover task is still running or stopping. Try again shortly.', 'カバー画像の生成処理が実行中、または停止中です。少し待ってから再試行してください。'],
  coverWaitForScan: ['请等待媒体库扫描完成。', 'Wait for the library scan to finish.', 'ライブラリのスキャンが終わるまでお待ちください。'],
  coverProcessed: ['{done} / {total} 项', '{done} of {total}', '{total} 件中 {done} 件'],
  coverGenerated: ['新生成 {number} 项', '{number} generated', '{number} 件を新規生成'],
  coverCached: ['已缓存 {number} 项', '{number} cached', '{number} 件はキャッシュ済み'],
  coverSkipped: ['跳过 {number} 项', '{number} skipped', '{number} 件をスキップ'],
  coverFailed: ['未生成 {number} 项', '{number} not generated', '{number} 件が未生成'],
  coverIssueDetails: ['查看未生成原因', 'See why covers were not generated', '未生成の理由を表示'],
  coverIssueSource: ['源文件已移动或无法访问', 'The source file moved or is inaccessible', '元のファイルが移動したか、アクセスできません'],
  coverIssueImage: ['图片无法读取或解码', 'The image could not be read or decoded', '画像を読み込めないか、デコードできません'],
  coverIssueVideo: ['FFmpeg 无法提取视频画面', 'FFmpeg could not extract a video frame', 'FFmpeg で動画のフレームを抽出できませんでした'],
  coverIssueCache: ['无法写入封面缓存', 'The cover cache could not be written', 'カバー画像をキャッシュに書き込めませんでした'],
  coverIssueOther: ['该封面未能生成，详情见日志', 'Could not generate this cover. See the log for details.', 'このカバー画像を生成できませんでした。詳しくはログを確認してください。'],
  updates: ['软件更新', 'Software updates', 'ソフトウェア更新'],
  downloadProgress: ['下载进度', 'Download progress', 'ダウンロードの進行状況'],
  checkUpdates: ['检查更新', 'Check for updates', '更新を確認'],
  installUpdate: ['下载并安装（将重启）', 'Download and restart', 'ダウンロードして再起動'],
  disabled: ['已停用', 'Disabled', '無効'],
  scanningItems: ['扫描中… {number} 项', 'Scanning… {number} found', 'スキャン中… {number} 件'],
  inaccessible: ['文件夹无法访问，内容已隐藏。请检查路径或磁盘后重新扫描，也可以在更多操作中移除。', 'This folder is unavailable and its contents are hidden. Check the path or drive and rescan, or remove it from the menu.', 'フォルダーにアクセスできないため、内容は非表示です。パスやドライブを確認して再スキャンするか、メニューから削除してください。'],
  mediaCount: ['{number} 个媒体', '{number} media items', 'メディア {number} 件'],
  oneMedia: ['{number} 个媒体', '{number} media item', 'メディア {number} 件'],
  folderCount: ['{number} 个文件夹', '{number} folders', 'フォルダー {number} 件'],
  oneFolder: ['{number} 个文件夹', '{number} folder', 'フォルダー {number} 件'],
  enableLibrary: ['{name} · 启用媒体库', '{name} · Enable library', '{name} · ライブラリを有効にする'],
  moreActions: ['{name} · 更多操作', '{name} · More actions', '{name} · その他の操作'],
  localAccess: ['本机访问', 'Local access', 'ローカルアクセス'],
  lanAccess: ['局域网访问', 'LAN access', 'LAN アクセス'],
  mobileAccess: ['手机访问', 'Mobile access', 'モバイルアクセス'],
  serverNotRunning: ['服务未运行。', 'The server is not running.', 'サービスは停止しています。'],
  copy: ['复制', 'Copy', 'コピー'],
  copyFor: ['复制{label}地址 {address}', 'Copy {label} address {address}', '{label}のアドレス {address} をコピー'],
  portNote: ['配置端口 {configured}，实际使用 {active}。', 'Configured port: {configured}. Active port: {active}.', '設定ポート: {configured}、使用中のポート: {active}。'],
  running: ['运行中', 'Running', '実行中'],
  stopped: ['已停止', 'Stopped', '停止中'],
  scanning: ['扫描中', 'Scanning', 'スキャン中'],
  scanningEllipsis: ['扫描中…', 'Scanning…', 'スキャン中…'],
  ffmpegReady: ['已就绪 · {path}', 'Ready · {path}', '利用可能 · {path}'],
  ffmpegMissing: ['未检测到 FFmpeg。播放需要转码时，请将它放到程序目录或加入 PATH。', 'FFmpeg was not found. To enable playback transcoding, put it in the program folder or add it to PATH.', 'FFmpeg が見つかりません。再生時に変換が必要な場合は、プログラムのフォルダーに配置するか PATH に追加してください。'],
  updateIdle: ['点击「检查更新」查看是否有新版本', 'Check for a newer version.', '新しいバージョンがあるか確認できます。'],
  updateChecking: ['正在从 GitHub 检查最新版本…', 'Checking GitHub for updates…', 'GitHub で更新を確認中…'],
  updateCurrent: ['已是最新版本', 'You are up to date', '最新バージョンです'],
  updateAvailable: ['发现新版本 v{version}，建议下载安装', 'Version v{version} is available', '新しいバージョン v{version} があります'],
  updateDownloading: ['正在下载并校验新版本…', 'Downloading and verifying the update…', '更新をダウンロードして検証中…'],
  updateRestarting: ['校验完成，正在重启到新版本…', 'Update verified. Restarting…', '検証が完了しました。再起動中…'],
  updateFailed: ['更新失败', 'Update failed', '更新に失敗しました'],
  updateUnknown: ['更新状态未知', 'Update status unknown', '更新状態が不明です'],
  copied: ['已复制访问地址', 'Address copied', 'アドレスをコピーしました'],
  copyFailed: ['无法自动复制，请选中地址并按 Ctrl+C', 'Select the address and press Ctrl+C to copy it.', 'アドレスを選択し、Ctrl+C でコピーしてください。'],
  enterFolder: ['请先填写文件夹路径', 'Enter a folder path first.', 'フォルダーのパスを入力してください。'],
  folderAdded: ['已添加，后台扫描中', 'Folder added. Scanning in the background.', 'フォルダーを追加しました。バックグラウンドでスキャン中です。'],
  rescanStarted: ['已开始重新扫描', 'Rescan started', '再スキャンを開始しました'],
  portApplied: ['端口已应用', 'Port updated', 'ポートを更新しました'],
  confirmRemoval: ['将「{name}」从媒体库移除？磁盘中的文件会保留。', 'Remove “{name}” from the library? Files will stay on disk.', '「{name}」をライブラリから削除しますか？ディスク上のファイルは残ります。'],
  libraryRemoved: ['已移除媒体库', 'Library removed', 'ライブラリを削除しました'],
  invalidRequest: ['请求格式错误', 'Invalid request.', 'リクエストの形式が正しくありません。'],
  missingFolder: ['路径不存在或不是文件夹：{detail}', 'Folder not found: {detail}', 'フォルダーが見つかりません: {detail}'],
  missingLibrary: ['媒体库不存在', 'Library not found.', 'ライブラリが見つかりません。'],
  invalidPort: ['端口取值 1-65535', 'Enter a port from 1 to 65535.', '1 から 65535 までのポート番号を入力してください。'],
  saveSettingFailed: ['保存设置失败：{detail}', 'Could not save settings: {detail}', '設定を保存できませんでした: {detail}'],
  invalidLanguage: ['不支持此语言', 'This language is not supported.', 'この言語には対応していません。'],
  startServerFirst: ['请先启动服务', 'Start the server first.', '先にサービスを起動してください。'],
  operationFailed: ['操作失败：{detail}', 'Operation failed: {detail}', '操作に失敗しました: {detail}'],
}

let localeIndex = -1
let count

function t(key, values = {}) {
  return consoleMessages[key][localeIndex].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ''))
}

function quantity(value, category) {
  const key = value === 1 ? (category === 'media' ? 'oneMedia' : 'oneFolder') : category + 'Count'
  return t(key, { number: count.format(value) })
}

function setConsoleLanguage(language) {
  const index = Math.max(0, consoleLanguages.indexOf(language))
  if (index === localeIndex) return false
  localeIndex = index
  document.documentElement.lang = consoleLanguages[index]
  count = new Intl.NumberFormat(consoleLanguages[index])
  document.title = t('title')
  for (const node of document.querySelectorAll('[data-label]')) node.textContent = t(node.dataset.label)
  for (const [attribute, selector] of [
    ['aria-label', 'data-label-aria'], ['title', 'data-label-title'], ['placeholder', 'data-label-placeholder'],
  ]) {
    for (const node of document.querySelectorAll(`[${selector}]`)) node.setAttribute(attribute, t(node.getAttribute(selector)))
  }
  return true
}
