package main

import (
	"github.com/getlantern/systray"

	"hestia/internal/config"
)

// onTrayReady 构建托盘菜单：打开主页 / 控制台开关 / 开机自启 / 退出。
func (a *app) onTrayReady() {
	systray.SetIcon(buildIcon())
	systray.SetTooltip("Hestia 家庭影视服务器")
	systray.SetTitle("")

	mOpen := systray.AddMenuItem("打开主页", "在浏览器中打开")
	mConsole := systray.AddMenuItem("显示 / 隐藏控制台", "切换控制台窗口")
	mAuto := systray.AddMenuItemCheckbox("开机自启", "登录后自动启动（静默）", a.cfgM.Get().AutoStart)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "停止服务并退出")

	go func() {
		for range mOpen.ClickedCh {
			openBrowser(a.srv.URLs()[0])
		}
	}()
	go func() {
		for range mConsole.ClickedCh {
			toggleConsole()
		}
	}()
	go func() {
		for range mAuto.ClickedCh {
			want := !a.cfgM.Get().AutoStart
			if err := a.cfgM.Update(func(c *config.Config) { c.AutoStart = want }); err != nil {
				a.logger.Printf("开机自启设置失败: %v", err)
				continue
			}
			if want {
				mAuto.Check()
			} else {
				mAuto.Uncheck()
			}
		}
	}()
	go func() {
		for range mQuit.ClickedCh {
			systray.Quit()
		}
	}()
	a.logger.Print("托盘已就绪（关闭此窗口服务仍后台运行）")
}

func (a *app) onTrayExit() {
	a.shutdown()
}
