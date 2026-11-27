# Hestia · Home media server

[简体中文](README.md) | English | [日本語](README.ja.md)

Browse and play media from multiple drives and folders in your phone's browser.

- **Single executable** (about 11 MB on Windows). Hestia needs no installation. Its Windows interface uses the WebView2 Runtime.
- **For your local network.** Browsing and playback require no login. The site hides filesystem paths. Media deletion requires local access or an authorization code.
- **Designed for phones.** A responsive light interface, dark playback view, and touch gestures.
- **Cross-platform.** Windows offers a WebView2 window, system tray controls, and startup at sign-in. macOS and Linux run in a terminal.

## Quick start

### Build

```bash
# Windows (or double-click build.bat)
build.bat

# macOS / Linux
./build.sh
```

Requires Go 1.27 or later. Go dependencies are included in the repository for offline builds. The outputs are `dist/Hestia.exe` on Windows, `dist/Hestia` on Linux, and `dist/Hestia.app/Contents/MacOS/Hestia` on macOS.

The frontend uses HTML, CSS, and ES modules. There is no separate frontend build or npm install. Go embeds the entire `web/` directory and serves it at runtime. Rebuild Hestia after editing the frontend. The source includes hls.js and the JavaScript and CSS for xgplayer 3.0.27, so playback needs no CDN. The xgplayer MIT license is in [xgplayer.LICENSE.txt](web/assets/js/vendor/xgplayer.LICENSE.txt).

To cross-compile, select the target operating system and architecture as shown below.

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/Hestia.exe .
GOOS=darwin   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/Hestia .
GOOS=linux    GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/Hestia .
```

### Run

Double-click `Hestia.exe` to open the Windows console. Use the site on your phone to search, browse, and play media. Configure the service in the Windows console.

1. Choose **Add folder**, enter a path such as `D:\Movies` or select one with **Browse**, then add it. You can add multiple libraries. Changes take effect without a restart.
2. The service starts with Hestia. If its configured port is busy, it tries the next 20 ports. Select the address in the footer to see the current local and LAN addresses.
3. Use the footer to stop or start the service. In **Settings**, you can change the port or start Hestia automatically when you sign in to Windows.
4. When you close the window, choose whether to keep the service running in the tray or exit. Left-click the tray icon to restore the window. You can also exit from its context menu.
5. Launching the executable again brings the existing console window forward.

The Windows console contacts GitHub for the latest stable release when you select **Check for updates**. If an update is available, the install action downloads it, verifies the SHA-256 checksum, replaces the executable, and restarts Hestia. It keeps `config.json` and `data/`. The program directory must be writable, and the release must include `Hestia.exe` and SHA-256 checksums. Upgrading from v0.3.6 or an earlier version requires replacing the executable manually the first time.

By default, Hestia keeps `config.json` and `data/` next to the executable for easy migration and backup. The data directory contains cached thumbnails, subtitles, and cover frames, temporary transcoding segments, rotating logs, playback progress, and WebView2 browser data. You can edit the configuration file directly. Changes are picked up within about two seconds.

```json
{
  "port": 8080,
  "listen": "0.0.0.0",
  "openBrowser": true,
  "autoStart": false,
  "language": "zh-CN",
  "libraries": [
    { "path": "D:\\Movies", "label": "Movies", "enabled": true }
  ]
}
```

### Language

Simplified Chinese, English, and Japanese are available in the same application. Simplified Chinese is the default. Choose **Settings → Language** in the Windows console to save your choice to `config.json`. The site's top-bar selector saves your choice in the current browser.

### Generate covers manually

Once the library scan finishes, select **Generate covers** in the Windows console, then select **Start**. You can hide the dialog and check progress while using the console. The results open automatically when generation finishes. Expand the details to see the file names and reasons for covers that could not be generated.

Covers are generated one at a time. Hestia skips covers that already have a valid cache. You can select **Stop** at any time and reuse completed covers the next time you start. Covers still load on demand while browsing. Very large images are processed on demand to limit memory use. FFmpeg is required to extract video frames.

### FFmpeg for fallback transcoding

Browsers generally cannot play files encoded as HEVC (H.265), AC3, DTS, or RMVB directly. Hestia detects such files and transcodes them to HLS on demand with FFmpeg. Transcoding processes are cleaned up when playback stops.

Hestia looks for FFmpeg next to the executable, in the `bin/` subdirectory, and then on the system `PATH`. Direct playback of standard formats such as H.264 in MP4 still works without FFmpeg. The Windows console shows whether FFmpeg is available.

## Features

| Feature | Details |
|---|---|
| Browsing | Top-level folders, subfolders, videos, and images are paginated on the server, 20 per page. Cards show local modification dates. Cards on the home and search pages open in a new tab. Returning home from a detail page restores the page number and cast filter. |
| Cast | Hestia extracts cast names from video filenames in `Cast-Title` format. Search and select cast in the desktop sidebar or use the dropdown on a phone. The filtered videos are paginated. |
| Sorting | Sort by name in natural order, modification date, or size. Choose ascending or descending order. Hestia remembers the setting. |
| Search | Fuzzy, paginated searches of video and image filenames. Supports Chinese substrings, full pinyin and initials, English subsequences, and space-separated terms. The cast filter covers all matches, and the × button returns to all collections. |
| Playback | Videos open in the current detail tab. Click the video to play or pause. The left fullscreen button fills the browser viewport and the right one enters browser fullscreen. Supports direct playback, HTTP Range, 0.5×, 1×, 1.5×, and 2× speeds, and navigation between videos in the same folder. |
| Gestures | Swipe vertically on the left for brightness or on the right for volume (seeking on iOS). Press and hold for 2× speed. Double-tap left or right to skip 10 seconds or double-tap the center to play or pause. Swipe horizontally to seek. |
| Keyboard | Space or K to play or pause, J or L to skip 10 seconds, left and right arrows to skip 5 seconds, up and down arrows to adjust volume, and F for fullscreen. |
| Subtitles | Automatically loads matching external SRT and ASS files named after the video, with an optional language suffix. Extracts embedded SRT, ASS, and mov_text tracks on demand from the CC menu. Subtitles are converted to WebVTT and rendered by the browser. |
| Playback progress | Resumes playback from the more recent local or server position. Continue Watching cards show progress. Playback positions sync between devices using the latest write. |
| Transcoding | FFmpeg converts sources such as HEVC, MKV, and AC3 to H.264/AAC HLS. Reserved sessions that receive no media requests after becoming ready are reclaimed after about 25 seconds. Sessions that have served media are reclaimed after about two minutes of inactivity. Up to three sessions run concurrently. |
| Live configuration | Add, remove, or update libraries and change ports while the service is running. Hestia scans at startup, when a library is added or enabled, and when you request a rescan. If a library becomes inaccessible, its contents are hidden and its configuration and playback progress are kept. Rescan when the path becomes available again, or remove it from the console. |
| Administration | The Windows WebView2 console is available only on `127.0.0.1`. It controls the service and includes library paths, port, startup at sign-in, rescanning, logs, and updates. Rotating logs are kept in `data/logs/hestia.log` (three 5 MB files) and can be opened from the console. Thumbnails, subtitles, and cover frames are cached on disk. |

## Directory layout

```
Configured library root
└── Top-level folder            Home cover grid
    └── Subfolder               May contain further subfolders
        ├── Video (mp4/mkv/webm/avi/mov/ts/…)
        ├── Subtitle (srt/ass)   Automatically matched to a video by name
        └── Image (jpg/png/webp/…)  Used as a cover and paired with a video card when the names match
```

Files directly under the library root can be found by search. Root-level videos with filenames that follow the cast convention also appear in cast-filtered results.

## Run from source

```bash
HESTIA_HOME=./.dev go run .    # Keep development data and configuration in .dev
```

## Known limits

- Chinese names are sorted by Unicode code point. Pinyin is supported in search.
- ASS styling is lost when converting to WebVTT. Embedded PGS image subtitle tracks are unsupported.
- The GUI is available on Windows. On macOS and Linux, use the terminal and edit `config.json`.
- Thumbnail generation supports JPEG and PNG. Other image formats are served as originals. Most browsers cannot display HEIC images.

## Third-party components

| Component | License | Purpose |
|---|---|---|
| [mozillazg/go-pinyin](https://github.com/mozillazg/go-pinyin) | MIT | Pinyin search |
| [jchv/go-webview2](https://github.com/jchv/go-webview2) | MIT | Windows GUI. Windows 11 includes the WebView2 Runtime. |
| [hls.js](https://github.com/video-dev/hls.js) | Apache-2.0 | HLS playback in desktop browsers |
| [xgplayer](https://github.com/bytedance/xgplayer) | MIT | Web player and playback controls |
| FFmpeg / FFprobe | GPL (external executables, supplied separately as needed) | Transcoding and media probing |

## Security

The service listens on `0.0.0.0` by default, so devices on your local network can access media. From the Windows console, open the site, go to a video or image detail page, choose the option to delete media from its folder, and enter a reason. To use the same action from a phone, display the phone authorization code in the deletion dialog on the Hestia computer, then enter it on the phone to review the files that will be deleted. The code changes when Hestia restarts. Use it only on a trusted LAN.

Confirming the operation stops related playback and transcoding, permanently deletes video, image, and subtitle files directly in that directory, and renames the directory to `original-name-reason`. Subfolders and other files are kept. If another program is still using a file, close it as prompted and try again.

To restrict access to this computer, set `listen` to `127.0.0.1`. Keep the port off the public Internet.
