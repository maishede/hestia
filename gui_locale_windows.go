//go:build windows

package main

type guiCopy struct {
	title, trayTip, trayBalloonTitle, trayBalloonBody string
	closeTitle, closeQuestion, showWindow, exit       string
	pickFolder, startupTitle, startupHint             string
	listenerFailed, webviewFailed, loadTimeout        string
	paintTimeout                                      string
}

var guiLocales = map[string]guiCopy{
	"zh-CN": {
		title: "Hestia 控制台", trayTip: "Hestia 家庭影视服务器（左键打开，右键退出）",
		trayBalloonTitle: "Hestia 仍在运行", trayBalloonBody: "服务在托盘中继续运行。左键恢复窗口，右键退出。",
		closeTitle: "关闭 Hestia", closeQuestion: "缩到托盘继续运行？\n选择「否」将退出并停止服务。",
		showWindow: "显示主窗口", exit: "退出", pickFolder: "选择媒体库文件夹",
		startupTitle: "Hestia 启动失败", startupHint: "请查看 data\\logs\\hestia.log 后重试。",
		listenerFailed: "无法启动本机控制台服务", webviewFailed: "无法初始化 WebView2。请检查 Microsoft Edge WebView2 Runtime 是否正常安装。",
		loadTimeout: "控制台页面加载超时。", paintTimeout: "控制台页面已加载，但 WebView2 未完成绘制。",
	},
	"en": {
		title: "Hestia Console", trayTip: "Hestia media server (left-click to open, right-click to exit)",
		trayBalloonTitle: "Hestia is still running", trayBalloonBody: "Hestia is running in the tray. Left-click to restore the window or right-click to exit.",
		closeTitle: "Close Hestia", closeQuestion: "Minimize to the tray and keep running?\nChoose No to exit and stop the server.",
		showWindow: "Show window", exit: "Exit", pickFolder: "Select a media library folder",
		startupTitle: "Hestia couldn't start", startupHint: "Check data\\logs\\hestia.log and try again.",
		listenerFailed: "Couldn't start the local console server", webviewFailed: "Couldn't initialize WebView2. Check that the Microsoft Edge WebView2 Runtime is installed.",
		loadTimeout: "The console took too long to load.", paintTimeout: "The console loaded, but WebView2 couldn't display it.",
	},
	"ja": {
		title: "Hestia コンソール", trayTip: "Hestia メディアサーバー（左クリックで表示、右クリックで終了）",
		trayBalloonTitle: "Hestia は動作中です", trayBalloonBody: "サービスはタスクトレイで動作中です。左クリックで表示、右クリックで終了できます。",
		closeTitle: "Hestia を閉じる", closeQuestion: "タスクトレイに最小化して実行を続けますか？\n「いいえ」を選ぶとサービスを停止して終了します。",
		showWindow: "ウィンドウを表示", exit: "終了", pickFolder: "メディアライブラリのフォルダーを選択",
		startupTitle: "Hestia を起動できませんでした", startupHint: "data\\logs\\hestia.log を確認してから再試行してください。",
		listenerFailed: "ローカルコンソールサーバーを起動できませんでした", webviewFailed: "WebView2 を初期化できませんでした。Microsoft Edge WebView2 Runtime がインストールされているか確認してください。",
		loadTimeout: "コンソールの読み込みがタイムアウトしました。", paintTimeout: "コンソールは読み込まれましたが、WebView2 で表示できませんでした。",
	},
}

func guiLanguage(code string) string {
	if _, ok := guiLocales[code]; ok {
		return code
	}
	return "zh-CN"
}
