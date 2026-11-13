# 播放器端到端验证

测试使用真实的 Hestia 接口、FFmpeg 和浏览器。产品本身不需要 Node 或 npm；它们仅用于运行这组开发测试。

1. 安装 FFmpeg/FFprobe，确保 PATH 可找到二者，且 FFmpeg 含 libx264、支持 10 位输出的 libx265、AAC 和 AC3 编码器。
2. 在临时目录安装 `playwright-core`，并通过 `NODE_PATH` 指向其 `node_modules`。准备 Chromium/Chrome；必要时设置 `HESTIA_TEST_CHROME` 为浏览器可执行文件路径。
3. 生成测试媒体：`node tools/player-tests/fixtures.cjs <临时媒体库目录>`。
4. 启动测试服务：`go run -mod=vendor ./tools/devserver -web web -lib <临时媒体库目录> -port 18100 -home <临时数据目录>`。也可通过 `HESTIA_TEST_BASE` 指定其他地址。
5. 依次运行 `node tools/player-tests/integration.cjs`、`node tools/player-tests/races.cjs`、`node tools/player-tests/play-intent.cjs`、`node tools/player-tests/compat.cjs`、`node tools/player-tests/close-session.cjs` 和 `node tools/player-tests/close-order.cjs`。
6. 运行 `node --test tools/player-tests/transcode-playback.cjs tools/player-tests/progress.cjs`，验证转码起播位置、会话失效恢复、10 位 HEVC 转码与断点进度。运行 `node tools/player-tests/close-session.cjs --lost-cancel` 可模拟取消请求丢失，验证服务端独立回收。
7. 运行 `node tools/player-tests/speed-menu.cjs`，验证桌面鼠标移出倍速按钮后仍可选中倍速。

这些测试拦截外站请求，并覆盖真实转码、字幕偏移、进度、触控和会话清理。`compat.cjs` 的 iOS 分支使用 Chromium 模拟 UA/触控；它不能替代实体 iPhone 和 Safari 验证。
