package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// buildIcon 运行时绘制托盘图标（圆角方块 + 火焰），打包为 PNG-in-ICO。
func buildIcon() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	accent := color.RGBA{91, 140, 255, 255}
	white := color.RGBA{255, 255, 255, 255}

	inCorner := func(x, y, cx, cy int) bool {
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy > 49
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			rounded := !(inCorner(x, y, 7, 7) || inCorner(x, y, 24, 7) ||
				inCorner(x, y, 7, 24) || inCorner(x, y, 24, 24))
			if rounded {
				img.Set(x, y, accent)
			}
		}
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-16, y-14
			if dx*dx+dy*dy <= 22 && dy >= -3 { // 火焰圆头
				img.Set(x, y, white)
			}
			if y >= 13 && y <= 25 { // 火焰下摆
				half := (25 - y) / 2
				if half > 0 {
					if dx < 0 {
						dx = -dx
					}
					if dx <= half {
						img.Set(x, y, white)
					}
				}
			}
		}
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)

	// ICONDIR + ICONDIRENTRY + PNG（Vista+ 支持 PNG 压缩的 ICO 条目）
	ico := &bytes.Buffer{}
	_ = binary.Write(ico, binary.LittleEndian, uint16(0))
	_ = binary.Write(ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(ico, binary.LittleEndian, uint16(1))
	ico.WriteByte(byte(size))
	ico.WriteByte(byte(size))
	ico.WriteByte(0) // 调色板
	ico.WriteByte(0) // 保留
	_ = binary.Write(ico, binary.LittleEndian, uint16(1))  // color planes
	_ = binary.Write(ico, binary.LittleEndian, uint16(32)) // bpp
	_ = binary.Write(ico, binary.LittleEndian, uint32(pngBuf.Len()))
	_ = binary.Write(ico, binary.LittleEndian, uint32(22)) // ICONDIR+ENTRY 字节数
	_, _ = ico.Write(pngBuf.Bytes())
	return ico.Bytes()
}
