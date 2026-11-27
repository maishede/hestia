// One catalog serves the web UI; media names and other library data are left as stored.
const languages = ['zh-CN', 'en', 'ja']
const storageKey = 'hestia.language'

const messages = {
  // Chinese, English, and Japanese. A fourth entry is the English singular form.
  title: ['Hestia · 家庭影视', 'Hestia · Home Media', 'Hestia · ホームメディア'],
  language: ['界面语言', 'Display language', '表示言語'],
  home: ['首页', 'Home', 'ホーム'],
  searchHint: ['搜索影片 / 文件夹…', 'Search videos or folders…', '動画・フォルダーを検索…'],
  clearSearch: ['清除搜索', 'Clear search', '検索をクリア'],
  loadFailed: ['加载失败', 'Could not load', '読み込めませんでした'],
  loadError: ['加载失败：{detail}', 'Could not load: {detail}', '読み込めませんでした：{detail}'],
  retryRefresh: ['请刷新重试', 'Refresh to try again', '再読み込みしてお試しください'],
  responseInvalid: ['服务响应异常，请稍后重试', 'Invalid server response. Try again later.', 'サーバーの応答が無効です。しばらくしてからお試しください。'],
  responseEmpty: ['服务返回为空，请稍后重试', 'The server returned an empty response. Try again later.', 'サーバーから応答がありません。しばらくしてからお試しください。'],
  requestFailed: ['请求失败（HTTP {status}）', 'Request failed (HTTP {status})', 'リクエストに失敗しました（HTTP {status}）'],
  networkFailed: ['无法连接到 Hestia，请稍后重试', 'Could not reach Hestia. Try again later.', 'Hestia に接続できません。しばらくしてからお試しください。'],
  newBadge: ['新', 'New', '新着'],
  updated: ['更新于 {date}', 'Updated {date}', '更新日 {date}'],
  shortVideoCount: ['{count} 部', '{count} videos', '{count} 本', '{count} video'],
  mediaBreakdown: ['{videos} 视频 · {images} 图', 'Videos: {videos} · Images: {images}', '動画 {videos} 本 · 画像 {images} 枚'],
  backHome: ['返回主页', 'Back to Home', 'ホームに戻る'],
  backFolder: ['返回文件夹', 'Back to Folder', 'フォルダーに戻る'],
  back: ['返回', 'Back', '戻る'],
  folderPagination: ['子文件夹分页', 'Subfolder pages', 'サブフォルダーのページ'],
  mediaPagination: ['媒体分页', 'Media pages', 'メディアのページ'],
  libraryPagination: ['媒体库分页', 'Library pages', 'ライブラリのページ'],
  searchPagination: ['搜索结果分页', 'Search result pages', '検索結果のページ'],
  previousPage: ['上一页', 'Previous', '前のページ'],
  nextPage: ['下一页', 'Next', '次のページ'],
  pageIndicator: ['第 {page} / {pages} 页', 'Page {page} of {pages}', '{page} / {pages} ページ'],
  subfolders: ['子文件夹', 'Subfolders', 'サブフォルダー'],
  media: ['媒体', 'Media', 'メディア'],
  folderBreakdown: ['{folders} 个子文件夹 · {media} 个媒体', 'Subfolders: {folders} · Media: {media}', 'サブフォルダー {folders} 件 · メディア {media} 件'],
  emptyFolder: ['这个文件夹是空的', 'This folder is empty', 'このフォルダーは空です'],
  rescanFolderHint: ['放入媒体后可在控制台重新扫描', 'Add media, then rescan from the desktop console', 'メディアを追加して、デスクトップのコンソールから再スキャンできます'],
  sort: ['排序方式', 'Sort by', '並べ替え'],
  ascending: ['正序（点击切换倒序）', 'Ascending (switch to descending)', '昇順（降順に切り替え）'],
  descending: ['倒序（点击切换正序）', 'Descending (switch to ascending)', '降順（昇順に切り替え）'],
  sortName: ['名称', 'Name', '名前'],
  sortDate: ['修改日期', 'Date modified', '更新日時'],
  sortSize: ['大小', 'Size', 'サイズ'],
  sortRelevance: ['相关度', 'Relevance', '関連度'],
  actorFilter: ['主演筛选', 'Filter by cast', '出演者で絞り込む'],
  actors: ['主演', 'Cast', '出演者'],
  actorSearch: ['搜索主演', 'Search cast', '出演者を検索'],
  actorSearchHint: ['搜索主演…', 'Search cast…', '出演者を検索…'],
  actorEmpty: ['没有匹配的主演', 'No matching cast members', '該当する出演者がいません'],
  library: ['媒体库', 'Media Library', 'メディアライブラリ'],
  allCollections: ['全部合集', 'All Collections', 'すべてのコレクション'],
  allSearchResults: ['全部结果', 'All Results', 'すべての検索結果'],
  noSearchActors: ['搜索结果中暂无主演', 'No cast members in these results', '検索結果に出演者がいません'],
  remaining: ['剩 {time}', '{time} left', '残り {time}'],
  keepWatching: ['继续观看', 'Continue Watching', '視聴を続ける'],
  progressCount: ['{count} 条进度', '{count} in progress', '視聴中 {count} 件'],
  clearProgress: ['清空进度', 'Clear Progress', '視聴履歴を消去'],
  confirmClearProgress: ['清除所有观看进度？', 'Clear all watch progress?', 'すべての視聴履歴を消去しますか？'],
  videoCount: ['{count} 部视频', '{count} videos', '動画 {count} 本', '{count} video'],
  collectionCount: ['{count} 个合集', '{count} collections', 'コレクション {count} 件', '{count} collection'],
  noActorVideos: ['没有找到该主演的视频', 'No videos for this cast member', 'この出演者の動画がありません'],
  libraryEmpty: ['还没有内容', 'Your library is empty', 'ライブラリは空です'],
  libraryEmptyHint: ['在电脑端添加媒体库目录，或等待扫描完成', 'Add a media folder in the desktop console or wait for the scan to finish', 'デスクトップのコンソールでフォルダーを追加するか、スキャンの完了をお待ちください'],
  resultCount: ['{count} 个结果', '{count} results', '{count} 件の結果', '{count} result'],
  noResults: ['没有找到匹配的内容', 'No matching results', '該当するコンテンツがありません'],
  tryKeyword: ['换个关键词试试', 'Try a different search term', '別のキーワードでお試しください'],
  queryTitle: ['“{query}”', '“{query}”', '「{query}」'],
  deleteMedia: ['删除媒体', 'Delete Media', 'メディアを削除'],
  deleteFolderMedia: ['删除所在文件夹媒体', 'Delete Media in Folder', 'フォルダー内のメディアを削除'],
  deleteTitle: ['删除文件夹中的媒体', 'Delete Media in This Folder', 'このフォルダー内のメディアを削除'],
  deleteGetCode: ['在电脑端打开任一媒体的删除窗口，点击「查看手机授权码」。', 'On the computer running Hestia, open a media deletion dialog and select “Show Mobile Authorization Code”.', 'Hestia を実行している PC でメディアの削除画面を開き、「モバイル用認証コードを表示」を選択してください。'],
  deleteCode: ['删除授权码', 'Deletion authorization code', '削除用認証コード'],
  deleteReason: ['删除原因', 'Reason for deletion', '削除理由'],
  deleteShowCode: ['查看手机授权码', 'Show Mobile Authorization Code', 'モバイル用認証コードを表示'],
  cancel: ['取消', 'Cancel', 'キャンセル'],
  deleteConfirm: ['确认删除', 'Confirm Deletion', '削除を確定'],
  deleteRenameTo: ['改名为「{name}」', 'Renamed to “{name}”', '「{name}」に名前を変更'],
  deleteSummary: ['将永久删除「{name}」中直属的 {count} 个视频、图片或字幕，并停止正在读取该目录的播放。其他文件会保留。', 'This permanently deletes {count} video, image, or subtitle files directly in “{name}” and stops playback from this folder. Other files are kept.', '「{name}」の直下にある動画・画像・字幕ファイル {count} 件を完全に削除し、このフォルダーからの再生を停止します。その他のファイルは保持されます。', 'This permanently deletes {count} video, image, or subtitle file directly in “{name}” and stops playback from this folder. Other files are kept.'],
  deleteNone: ['「{name}」中没有可删除的直属媒体。', 'There are no removable media files directly in “{name}”.', '「{name}」の直下に削除できるメディアがありません。'],
  deleteSubfolders: ['发现 {count} 个子文件夹或链接，请手动检查。里面的内容会保留。', 'Review the {count} subfolders or links manually. Their contents are kept.', 'サブフォルダーまたはリンクが {count} 件あります。手動で確認してください。その内容は保持されます。', 'Review the {count} subfolder or link manually. Its contents are kept.'],
  deletePreview: ['查看删除范围', 'Review Files to Delete', '削除対象を確認'],
  deleteCodeRequired: ['请输入电脑端显示的 32 位授权码', 'Enter the 32-character code shown on the computer.', 'PC に表示された 32 文字の認証コードを入力してください。'],
  deleteReasonRequired: ['请输入删除原因', 'Enter a reason for deletion.', '削除理由を入力してください。'],
  deleting: ['正在删除…', 'Deleting…', '削除しています…'],
  deleted: ['已删除 {count} 个媒体文件，文件夹已改名为「{name}」', 'Deleted {count} media files and renamed the folder to “{name}”', 'メディアファイルを {count} 件削除し、フォルダー名を「{name}」に変更しました', 'Deleted {count} media file and renamed the folder to “{name}”'],
  imageLoading: ['正在加载图片…', 'Loading image…', '画像を読み込んでいます…'],
  loadOriginal: ['加载原图', 'Load Original Image', '元の画像を表示'],
  previousImage: ['上一张', 'Previous image', '前の画像'],
  nextImage: ['下一张', 'Next image', '次の画像'],
  imageFailed: ['图片加载失败', 'Could not load image', '画像を読み込めませんでした'],
  imageClosed: ['图片已关闭', 'Image closed', '画像を閉じました'],
  playerFailed: ['播放器加载失败', 'Could not load player', 'プレーヤーを読み込めませんでした'],
  hlsFailed: ['hls.js 加载失败', 'Could not load hls.js', 'hls.js を読み込めませんでした'],
  videoLoading: ['正在加载视频…', 'Loading video…', '動画を読み込んでいます…'],
  videoUnavailable: ['视频不存在或已被移除', 'Video is missing or was removed', '動画が見つからないか、削除されています'],
  videoMetadataFailed: ['视频信息加载失败，请返回重试', 'Could not load video information. Go back and try again.', '動画情報を読み込めませんでした。戻ってからお試しください。'],
  track: ['轨道 {index}', 'Track {index}', 'トラック {index}'],
  transcode: ['转码', 'Transcode', '変換'],
  progress: ['进度', 'Playback progress', '再生位置'],
  continuePlaying: ['继续播放', 'Resume', '続きから再生'],
  startOver: ['从头看', 'Start Over', '最初から再生'],
  loading: ['加载中…', 'Loading…', '読み込んでいます…'],
  playbackFailed: ['播放失败', 'Playback failed', '再生に失敗しました'],
  retryTranscode: ['用转码模式重试', 'Retry with Transcoding', '変換して再試行'],
  previousVideo: ['上一个', 'Previous video', '前の動画'],
  nextVideo: ['下一个', 'Next video', '次の動画'],
  subtitles: ['字幕', 'Subtitles', '字幕'],
  subtitlesOff: ['关闭字幕', 'Subtitles Off', '字幕オフ'],
  ffmpegMissing: ['服务端未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）', 'Transcoding needs FFmpeg. Put it beside Hestia or add it to PATH.', '変換には FFmpeg が必要です。Hestia と同じフォルダーに置くか、PATH に追加してください。'],
  transcodeStarting: ['转码启动中…', 'Starting transcoding…', '変換を開始しています…'],
  transcoding: ['转码中…', 'Transcoding…', '変換中…'],
  hlsUnsupported: ['浏览器不支持 HLS 播放', 'This browser does not support HLS playback', 'このブラウザーでは HLS を再生できません'],
  transcodeReservationFailed: ['转码会话预留失败', 'Could not reserve a transcoding session', '変換セッションを確保できませんでした'],
  transcodeMismatch: ['转码会话标识不匹配', 'Transcoding session ID does not match', '変換セッション ID が一致しません'],
  transcodeEnded: ['转码会话已结束，请重试', 'The transcoding session ended. Try again.', '変換セッションは終了しました。もう一度お試しください。'],
  transcodeFailed: ['转码播放失败，请重试', 'Could not play the transcoded video. Try again.', '変換した動画を再生できませんでした。もう一度お試しください。'],
  resumeFrom: ['上次看到 {time}', 'Last watched at {time}', '前回の再生位置 {time}'],
  buffering: ['缓冲中…', 'Buffering…', 'バッファリング中…'],
  finishedNext: ['已播完 · 下一个：《{name}》', 'Finished · Next: “{name}”', '再生終了 · 次の動画：「{name}」'],
  cannotPlayNoFfmpeg: ['该格式浏览器无法播放，且服务端缺少 ffmpeg', 'This browser cannot play the format, and FFmpeg is missing on the server.', 'このブラウザーでは形式を再生できず、サーバーに FFmpeg がありません。'],
  cannotDecode: ['播放失败：浏览器无法解码该文件', 'Playback failed: the browser could not decode this file', '再生に失敗しました：このファイルをデコードできません'],
  transcodeFallback: ['直连播放失败，已切换转码模式…', 'Direct playback failed. Switching to transcoding…', '直接再生に失敗しました。変換に切り替えています…'],
  iosHint: ['iOS 上请用音量键调音量；右侧上下滑动可快进/倒退', 'On iOS, use the volume buttons for sound. Swipe up or down on the right to seek.', 'iOS では音量ボタンで音量を調整します。右側を上下にスワイプすると早送り・巻き戻しできます。'],
  fastForward: ['2x 快进中', 'Fast-forwarding at 2×', '2 倍速で早送り中'],
  playbackStopped: ['播放已停止', 'Playback stopped', '再生を停止しました'],
  reopenVideo: ['重新打开视频', 'Reopen Video', '動画を開き直す'],
}

// The API sends human-readable diagnostics; translate stable messages and retain technical details.
const serverMessages = new Map([
  ['文件夹不存在', ['Folder not found', 'フォルダーが見つかりません']],
  ['文件夹不存在或已移动', ['The folder is missing or has moved', 'フォルダーが見つからないか、移動されています']],
  ['媒体不存在', ['Media not found', 'メディアが見つかりません']],
  ['视频不存在', ['Video not found', '動画が見つかりません']],
  ['文件无法读取（磁盘未挂载？）', ['Could not read the file. Check that its drive is mounted.', 'ファイルを読み込めません。ドライブがマウントされているか確認してください。']],
  ['文件无法读取', ['Could not read the file', 'ファイルを読み込めません']],
  ['文件不可用', ['File unavailable', 'ファイルを利用できません']],
  ['请求格式错误', ['Invalid request format', 'リクエストの形式が無効です']],
  ['转码会话不存在或已结束', ['The transcoding session is missing or has ended', '変換セッションが存在しないか、終了しています']],
  ['转码会话已取消或已过期', ['The transcoding session was canceled or expired', '変換セッションがキャンセルされたか、期限が切れています']],
  ['转码会话已停止', ['The transcoding session stopped', '変換セッションが停止しました']],
  ['待启动的转码会话过多', ['Too many transcoding sessions are waiting to start', '開始待ちの変換セッションが多すぎます']],
  ['转码启动超时', ['Starting transcoding timed out', '変換の開始がタイムアウトしました']],
  ['未找到 ffmpeg', ['FFmpeg was not found', 'FFmpeg が見つかりません']],
  ['未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）', ['Transcoding needs FFmpeg. Put it beside Hestia or add it to PATH.', '変換には FFmpeg が必要です。Hestia と同じフォルダーに置くか、PATH に追加してください。']],
  ['请在 Hestia 所在电脑上查看授权码', ['View the authorization code on the computer running Hestia', 'Hestia を実行している PC で認証コードを確認してください']],
  ['请输入电脑端的删除授权码', ['Enter the deletion code shown on the computer', 'PC に表示された削除用認証コードを入力してください']],
  ['删除授权无效，请重新打开删除窗口', ['The deletion authorization is invalid. Reopen the deletion dialog.', '削除の認証が無効です。削除画面を開き直してください。']],
  ['请先确认删除范围', ['Review which files will be deleted first', '先に削除対象を確認してください']],
  ['请输入有效的删除原因', ['Enter a valid reason for deletion', '有効な削除理由を入力してください']],
  ['请输入有效的删除原因，改名后的文件夹名称太长或含有无效字符', ['Enter a valid reason. The resulting folder name is too long or contains invalid characters.', '有効な理由を入力してください。変更後のフォルダー名が長すぎるか、使用できない文字を含んでいます。']],
  ['无法按当前确认内容删除', ['Could not delete the files with the current confirmation', '現在の確認内容では削除できません']],
  ['无法按当前确认内容删除：文件夹与 Hestia 数据目录重叠', ['Cannot delete: this folder overlaps with the Hestia data directory', 'Hestia のデータディレクトリと重複しているため削除できません']],
  ['无法按当前确认内容删除：文件夹与另一个媒体库重叠', ['Cannot delete: this folder overlaps with another media library', '他のメディアライブラリと重複しているため削除できません']],
  ['无法按当前确认内容删除：同名文件夹或文件已存在', ['Cannot delete: a file or folder with that name already exists', '同名のファイルまたはフォルダーがあるため削除できません']],
  ['无法按当前确认内容删除：文件夹内容已变化，请重新确认', ['The folder contents changed. Review the deletion again.', 'フォルダーの内容が変わりました。削除対象を確認し直してください。']],
  ['无法按当前确认内容删除：当前文件夹没有可删除的媒体', ['The folder has no removable media', 'フォルダー内に削除できるメディアがありません']],
  ['文件仍在读取，请稍后重试', ['The file is still in use. Try again later.', 'ファイルは使用中です。しばらくしてからお試しください。']],
  ['文件夹正在删除或改名', ['The folder is being deleted or renamed', 'フォルダーを削除中、または名前を変更中です']],
])

const serverPrefixes = [
  ['文件仍被其他程序占用。关闭正在读取该目录的程序后重试。', ['Another program is using a file. Close programs accessing the folder and try again. ', '他のプログラムがファイルを使用しています。フォルダーを使用しているプログラムを閉じて再試行してください。']],
  ['无法处理该文件夹，请检查文件是否被占用及目录权限。', ['Could not process the folder. Check file usage and folder permissions. ', 'フォルダーを処理できません。ファイルの使用状況とフォルダーのアクセス権を確認してください。']],
  ['ffmpeg 失败: ', ['FFmpeg failed: ', 'FFmpeg が失敗しました：']],
  ['转码异常结束: ', ['Transcoding ended unexpectedly: ', '変換が異常終了しました：']],
  ['文件夹已改名，请检查临时目录 ', ['The folder was renamed. Check this temporary directory: ', 'フォルダー名を変更しました。次の一時ディレクトリを確認してください：']],
  ['索引刷新失败，请在控制台重新扫描', ['Could not refresh the index. Rescan from the desktop console.', 'インデックスを更新できませんでした。デスクトップのコンソールから再スキャンしてください。']],
]

export function serverMessage(message) {
  if (language === 'zh-CN' || typeof message !== 'string') return message
  const index = languages.indexOf(language) - 1
  const exact = serverMessages.get(message)
  if (exact) return exact[index]
  for (const [prefix, translated] of serverPrefixes) {
    if (message.startsWith(prefix)) return translated[index] + message.slice(prefix.length)
  }
  return message
}

function storedLanguage() {
  try {
    const value = localStorage.getItem(storageKey)
    if (languages.includes(value)) return value
  } catch {}
  return languages[0]
}

export let language = storedLanguage()

export function setLanguage(value) {
  if (!languages.includes(value) || value === language) return false
  language = value
  try { localStorage.setItem(storageKey, value) } catch {}
  return true
}

export function t(key, values = {}) {
  const message = messages[key]
  const index = languages.indexOf(language)
  const template = index === 1 && values.count === 1 && message[3] ? message[3] : message[index]
  return template.replace(/\{(\w+)\}/g, (_, name) =>
    typeof values[name] === 'number' ? number(values[name]) : String(values[name] ?? ''))
}

export function number(value, fractionDigits = 0) {
  return Number(value).toLocaleString(language, {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  })
}

export function relativeTime(value, unit) {
  return new Intl.RelativeTimeFormat(language).format(value, unit)
}
