# Hestia 图标

当前采用「薄荷播放环」：以浅绿、浅蓝为主色，用少量浅黄和杏橙作为折面点缀。中央以通透的播放环表达家庭影视用途，背景保持浅色，避免彩虹色争抢注意力。

图标由内置 image_gen 生成。原图为 1254 × 1254 PNG，圆角外保留透明通道；项目使用的母版为 `icon-master.png`。PNG / ICO 仅从母版导出尺寸，不改变生成图案。

## 设计源文件

- `designs/icons/01-mint-loop.png`：薄荷播放环，当前项目使用。
- `designs/icons/02-soft-prism.png`：柔光棱镜，备选。
- `designs/icons/prompts.md`：两版完整生成提示词。使用内置 image_gen，`transparent_background: true`。

## 项目资源

- `icon-master.png`：1254 px 生成母版，与第一版方案一致。
- `icon-32.png`：Windows 托盘和小窗口图标。
- `icon-256.png`：Windows 大窗口图标。
- `icon.ico`：16、24、32、48、64、128、256 px 七档尺寸。
- `rsrc_windows_amd64.syso`：Windows exe 图标资源，必须随 ICO 同步更新。
- `web/assets/img/favicon.png`：64 px 浏览器图标。
- `web/assets/img/logo.png`、`web/gui/img/logo.png`：256 px 网页 / 控制台标志。

## 重新导出

选择方案后将其复制为 `icon-master.png`，在 Windows 上运行 `powershell -ExecutionPolicy Bypass -File tools/update-icons.ps1` 更新全部 PNG / ICO 和 Windows 资源。脚本只保存文件，不打开预览。

仅更新图标时需要 GNU windres（MinGW）；常规 `build.bat` 直接使用已提交的资源文件，无须新增依赖。

替换母版后运行上述脚本，再构建程序。网页图片使用版本参数刷新缓存；后续更换图标时同时更新两个 HTML 文件的版本参数。

## 配色方向

浅绿、浅蓝构成主体与环境，浅黄、杏橙仅作局部反光。以通透感、柔和明暗和简洁轮廓形成层次，不使用紫红色或深色底。
