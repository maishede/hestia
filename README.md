# Hestia · 家庭影视服务器

把散落在多块磁盘、多个文件夹里的影视资源变成一个手机浏览器就能看的私人影院。

- **单文件**：一个 exe（约 10MB），双击即用，无需安装、无需运行时
- **纯局域网自用**：免登录、只读（不提供任何删除/修改能力）、页面永不暴露物理路径
- **手机优先**：响应式浅色 UI（播放器为深色放映厅风格）+ 触屏手势
- **跨平台**：Windows 一等公民（WebView2 GUI 窗口 + 托盘常驻 + 开机自启），macOS / Linux 同源编译（控制台模式）

## 快速开始

### 构建

```bash
# Windows（或直接双击 build.bat）
build.bat

# macOS / Linux
./build.sh
```

需要 Go 1.22+。第三方依赖已 vendor 到仓库（离线可构建）。产物在 `dist/Hestia(.exe)`。

**前端不需要单独构建**：前端是原生 HTML/CSS/ES Modules（无 npm、无打包步骤），`go:embed` 在编译时把 `web/` 目录整个打进 exe，运行时由 Go 服务直接输出。改前端只需重新执行构建命令。`web/assets/js/vendor/hls.min.js` 是预下载的第三方库，已随源码入库，同样无需下载。

交叉编译（在任意平台构建其他平台产物）：

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/Hestia.exe .
GOOS=darwin   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/Hestia .
GOOS=linux    GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/Hestia .
```

### 运行

双击 `Hestia.exe`，弹出**配置窗口**（手机访问的网页只有浏览和播放，不含任何配置）：

1. 在窗口的"媒体库路径"填路径（如 `D:\电影`）或点「浏览…」弹系统对话框选择，点"添加"——支持多个，**热生效无需重启**
2. 服务默认自启；窗口顶部显示手机访问地址（如 `http://192.168.x.x:8080`），手机浏览器输入即可观看
3. "关闭服务/启动服务"随时启停；"应用"修改端口；"开机自启"写入注册表（登录后自动运行）
4. 点窗口「X」默认**最小化到托盘**继续运行（首次会气泡提示）；左键托盘图标恢复窗口，**右键托盘图标可选「退出」**（这才会停止服务）
5. 重复启动 exe 只会把已有窗口唤到前台（单实例防抖，不闪新窗）

手机网页端 = 搜索 + 浏览 + 播放器，没有任何管理功能。

**所有数据只落在 exe 所在目录**（不写系统盘）：`config.json` 配置、`data/`（缓存缩略图/字幕/封面帧、转码临时段、滚动日志、进度、WebView2 浏览器数据），换盘/备份/删除只须处理这一个文件夹。配置可直接手改（2 秒内热加载）：

```json
{
  "port": 8080,
  "listen": "0.0.0.0",
  "openBrowser": true,
  "autoStart": false,
  "libraries": [
    { "path": "D:\\电影", "label": "电影", "enabled": true }
  ]
}
```

命令行参数：`Hestia.exe -hide` 静默启动（隐藏控制台，配合开机自启）。

### ffmpeg（转码兜底，推荐配置）

浏览器普遍无法直接播放 **HEVC(H.265) / AC3 / DTS / RMVB** 等编码。Hestia 检测到这类文件会自动用 ffmpeg 按需转码为 HLS 再播放，无需任何手动操作；播放停止后转码进程自动销毁。

自动查找顺序：exe 同目录 → `bin/` 子目录 → 系统 `PATH`。没有 ffmpeg 时 H.264/MP4 等常规格式仍可直链播放（配置窗口会显示提示）。

## 功能一览

| 模块 | 说明 |
|---|---|
| 浏览 | 母文件夹 → 子文件夹 → 视频/图片；封面自动识别（`poster > cover > folder > fanart > 第一张图`，父文件夹可继承子层封面，无图自动抽视频首帧）；每页 20 个，无限滚动；两周内新入库带"新"角标 |
| 排序 | 名称（自然排序，“第2集”排在“第10集”前）/ 修改日期 / 大小，正序倒序可切，记忆选择 |
| 搜索 | 全库模糊搜索：中文子串、**拼音全拼/首字母**（`liulang`、`lldq`）、英文子序列（`irnm` 匹配 `Iron.Man`）、空格分词 AND |
| 播放 | 默认原画直链（HTTP Range 秒拖进度）；0.5x–5x 八档倍速（不变调）；同文件夹上/下一个 |
| 手势 | 左半屏上下滑调亮度 · 右半屏上下滑调音量（iOS 上改为快进）· 长按 2 倍速 · 双击左右 ±10s / 中间播放暂停 · 水平滑动快进 |
| 键盘 | 空格/K 播放暂停 · J/L ±10s · ←/→ ±5s · ↑/↓ 音量 · F 全屏 |
| 字幕 | 外挂 srt/ass（同名或"视频名.语言"）自动加载默认开启；内嵌文本字幕轨（srt/ass/mov_text）CC 菜单按需提取；统一转 WebVTT 原生渲染 |
| 进度 | 断点续播（本地 + 服务端合并，较新者胜）；首页"继续观看"带进度条；**多设备进度同步**（最后写入优先） |
| 转码 | HEVC/MKV/AC3 等自动降级 ffmpeg→HLS；空闲 2 分钟自动销毁；并发上限 3 |
| 热配置 | 媒体库增删改、端口切换，均不中断服务；每 15 分钟自动增量重扫 |
| 运维 | Windows GUI 配置窗口（WebView2 加载内嵌页面，与手机端同款浅色设计；启停/路径/端口/自启/重扫/日志；GUI 接口仅绑定 127.0.0.1，局域网不可达）；滚动日志（`data/logs/hestia.log`，5MB×3，窗口可一键打开）；缩略图/字幕/封面帧磁盘缓存 |

## 目录约定

```
媒体库根（配置项）
└── 母文件夹            ← 首页封面墙
    └── 子文件夹        ← 可继续嵌套
        ├── 视频 (mp4/mkv/webm/avi/mov/ts/...)
        ├── 字幕 (srt/ass)          ← 与视频同名自动挂载
        └── 图片 (jpg/png/webp/...) ← 自动作为封面；同名图片还会成为视频卡片封面
```

注意：直接放在**媒体库根目录**下的文件不会出现在浏览列表（只参与搜索），请按“母文件夹/子文件夹”组织。

## 从源码运行（开发）

```bash
HESTIA_HOME=./.dev go run .    # 数据/配置隔离到 .dev 目录
```

## 已知边界

- 中文排序按 Unicode 码点序（拼音匹配已覆盖搜索场景）
- ass 字幕转 VTT 不保留特效样式；内嵌 PGS 图形字幕轨不支持
- GUI 仅 Windows；macOS/Linux 为控制台运行，配置走 `config.json`
- 图片缩略图仅 jpeg/png（其他格式回原图）；HEIC 图片大部分浏览器不显示

## 第三方组件（均已 vendor）

| 组件 | 许可证 | 用途 |
|---|---|---|
| [mozillazg/go-pinyin](https://github.com/mozillazg/go-pinyin) | MIT | 拼音搜索 |
| [jchv/go-webview2](https://github.com/jchv/go-webview2) | MIT | Windows GUI 窗口（WebView2，Win11 自带运行时） |
| [hls.js](https://github.com/video-dev/hls.js) | Apache-2.0 | 桌面浏览器 HLS 播放 |
| ffmpeg / ffprobe | GPL（外部程序，按需自备） | 转码 / 探测 |

## 安全说明

服务默认监听 `0.0.0.0`（局域网可访问），全程只读且无任何写入接口；如需仅本机访问，把 `listen` 改为 `127.0.0.1`。请勿将端口暴露到公网。
