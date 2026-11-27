//go:build windows

package main

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"syscall"
	"unsafe"
)

const (
	startupWidth, startupHeight = 288, 288
	startupLogoSize             = 184
	startupLogoX                = (startupWidth - startupLogoSize) / 2
	startupLogoY                = (startupHeight - startupLogoSize) / 2
)

var (
	startupGDI      = syscall.NewLazyDLL("gdi32.dll")
	startupMakeDC   = startupGDI.NewProc("CreateCompatibleDC")
	startupDeleteDC = startupGDI.NewProc("DeleteDC")
	startupDIB      = startupGDI.NewProc("CreateDIBSection")
	startupSelect   = startupGDI.NewProc("SelectObject")
	startupDelete   = startupGDI.NewProc("DeleteObject")
	startupFlush    = startupGDI.NewProc("GdiFlush")
	startupCopy     = startupGDI.NewProc("BitBlt")
	startupLayer    = tUser32.NewProc("UpdateLayeredWindow")
)

type startupBitmapInfo struct {
	size                uint32
	width, height       int32
	planes, bits        uint16
	compression, length uint32
	xDensity, yDensity  int32
	colors, important   uint32
}

type startupBlend struct{ operation, flags, opacity, alpha byte }

type startupLogoPixel struct {
	index int
	color uint32
	x, y  float64 // Direction from the center of the play opening.
}

type startupVisual struct {
	dc, bitmap, previous uintptr
	pixels               []uint32 // Premultiplied BGRA expected by UpdateLayeredWindow.
	frost                []uint32
	logo                 []startupLogoPixel
}

func newStartupVisual() *startupVisual {
	logo, err := png.Decode(bytes.NewReader(icon256PNG))
	if err != nil {
		return nil
	}
	dc, _, _ := startupMakeDC.Call(0)
	if dc == 0 {
		return nil
	}
	info := startupBitmapInfo{size: 40, width: startupWidth, height: -startupHeight, planes: 1, bits: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := startupDIB.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		if bitmap != 0 {
			startupDelete.Call(bitmap)
		}
		startupDeleteDC.Call(dc)
		return nil
	}
	previous, _, _ := startupSelect.Call(dc, bitmap)
	if previous == 0 || previous == ^uintptr(0) {
		startupDelete.Call(bitmap)
		startupDeleteDC.Call(dc)
		return nil
	}
	v := &startupVisual{
		dc: dc, bitmap: bitmap, previous: previous,
		pixels: unsafe.Slice((*uint32)(bits), startupWidth*startupHeight),
		frost:  startupFrost(),
		logo:   startupCutout(logo),
	}
	copy(v.pixels, v.frost)
	return v
}

func (v *startupVisual) close() {
	startupSelect.Call(v.dc, v.previous)
	startupDelete.Call(v.bitmap)
	startupDeleteDC.Call(v.dc)
}

type startupPoint struct{ x, y float64 }

// Paths follow the existing 256 px icon. The outer silhouette and the play
// opening clip the original artwork, including the light that moves across it.
var startupLogoContour = [...][4]startupPoint{
	{{92, 35}, {119, 33}, {140, 49}, {166, 67}},
	{{166, 67}, {196, 86}, {221, 95}, {223, 128}},
	{{223, 128}, {224, 157}, {203, 168}, {169, 184}},
	{{169, 184}, {139, 199}, {115, 223}, {88, 219}},
	{{88, 219}, {59, 216}, {46, 198}, {48, 166}},
	{{48, 166}, {49, 145}, {51, 127}, {53, 113}},
	{{53, 113}, {49, 100}, {47, 83}, {50, 70}},
	{{50, 70}, {54, 49}, {72, 35}, {92, 35}},
}

var startupPlayContour = [...][4]startupPoint{
	{{92.7, 94}, {92, 82}, {96, 75}, {104, 80}},
	{{104, 80}, {126, 93}, {152, 110}, {174, 123}},
	{{174, 123}, {180, 127}, {180, 132}, {173, 137}},
	{{173, 137}, {154, 149}, {128, 161}, {111, 171}},
	{{111, 171}, {101, 177}, {94, 173}, {93, 162}},
	{{93, 162}, {92, 144}, {94, 118}, {92.7, 94}},
}

func startupOutline(path [][4]startupPoint) []startupPoint {
	const steps = 16
	points := make([]startupPoint, 0, len(path)*steps+1)
	points = append(points, path[0][0])
	for _, curve := range path {
		for i := 1; i <= steps; i++ {
			t := float64(i) / steps
			u := 1 - t
			weights := [4]float64{u * u * u, 3 * u * u * t, 3 * u * t * t, t * t * t}
			var p startupPoint
			for j, control := range curve {
				p.x += weights[j] * control.x
				p.y += weights[j] * control.y
			}
			points = append(points, p)
		}
	}
	return points
}

func startupSpan(points []startupPoint, y float64) (left, right float64) {
	left, right = math.Inf(1), math.Inf(-1)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if (a.y > y) != (b.y > y) {
			x := a.x + (b.x-a.x)*(y-a.y)/(b.y-a.y)
			left, right = min(left, x), max(right, x)
		}
	}
	return left, right
}

func startupCutout(src image.Image) []startupLogoPixel {
	outer := startupOutline(startupLogoContour[:])
	hole := startupOutline(startupPlayContour[:])
	const samples = 3
	const scale = 202.0 / startupLogoSize
	var logo []startupLogoPixel
	for y := range startupLogoSize {
		var exterior, interior [samples][2]float64
		for sy := range samples {
			py := 26 + (float64(y)+(float64(sy)+0.5)/samples)*scale
			exterior[sy][0], exterior[sy][1] = startupSpan(outer, py)
			interior[sy][0], interior[sy][1] = startupSpan(hole, py)
		}
		for x := range startupLogoSize {
			var red, green, blue, alpha uint32
			for sy := range samples {
				py := 26 + (float64(y)+(float64(sy)+0.5)/samples)*scale
				for sx := range samples {
					px := 35 + (float64(x)+(float64(sx)+0.5)/samples)*scale
					if px < exterior[sy][0] || px > exterior[sy][1] || (px >= interior[sy][0] && px <= interior[sy][1]) {
						continue
					}
					r, g, b, a := src.At(int(px), int(py)).RGBA()
					red, green, blue, alpha = red+r, green+g, blue+b, alpha+a
				}
			}
			if alpha == 0 {
				continue
			}
			const divisor = 257 * samples * samples
			color := alpha/divisor<<24 | red/divisor<<16 | green/divisor<<8 | blue/divisor
			dx := 35 + (float64(x)+0.5)*scale - 123
			dy := 26 + (float64(y)+0.5)*scale - 126
			length := math.Hypot(dx, dy)
			index := (startupLogoY+y)*startupWidth + startupLogoX + x
			logo = append(logo, startupLogoPixel{index: index, color: color, x: dx / length, y: dy / length})
		}
	}
	return logo
}

// The matte disc lives on the splash's alpha surface. Its textured fill lets
// the desktop show through and its feathered edge fades to full transparency.
func startupFrost() []uint32 {
	const radius, feather = 136, 55
	pixels := make([]uint32, startupWidth*startupHeight)
	for y := range startupHeight {
		for x := range startupWidth {
			dx, dy := float64(x)+0.5-startupWidth/2, float64(y)+0.5-startupHeight/2
			edge := radius - math.Hypot(dx, dy)
			if edge <= 0 {
				continue
			}
			fade := min(1, edge/feather)
			fade *= fade * (3 - 2*fade)
			grainSeed := uint32(x)*0x51ED270B ^ uint32(y)*0x9E3779B9
			grainSeed ^= grainSeed >> 16
			grain := float64(grainSeed&255)/255 - 0.5
			shade := (-0.55*dx-0.85*dy)/radius + 0.16*grain
			alpha := uint32(fade * (54 + 8*shade))
			red := uint32(198 + 11*shade)
			green := uint32(212 + 11*shade)
			blue := uint32(218 + 11*shade)
			pixels[y*startupWidth+x] = alpha<<24 | (red*alpha/255)<<16 | (green*alpha/255)<<8 | blue*alpha/255
		}
	}
	return pixels
}

func startupOver(foreground, background uint32) uint32 {
	remaining := 255 - (foreground >> 24)
	if remaining == 0 {
		return foreground
	}
	a := (background >> 24) * remaining / 255
	r := (background >> 16 & 0xFF) * remaining / 255
	g := (background >> 8 & 0xFF) * remaining / 255
	b := (background & 0xFF) * remaining / 255
	return foreground + (a<<24 | r<<16 | g<<8 | b)
}

func (v *startupVisual) draw(elapsed float64, animate bool) {
	angle := 2*math.Pi*elapsed/1.65 - 2.3
	sin, cos := math.Sincos(angle)
	for _, p := range v.logo {
		color := p.color
		if animate {
			facing := p.x*cos + p.y*sin
			// A soft mint reflection and a narrower blue leading edge travel through the ribbon.
			light := max(0, facing)
			light *= light
			lift := light * light * light
			leading := max(0, facing*0.978+(p.y*cos-p.x*sin)*0.208)
			leading *= leading
			leading *= leading
			leading *= leading
			alpha := color >> 24
			r, g, b := color>>16&0xFF, color>>8&0xFF, color&0xFF
			r += uint32(float64(alpha-r) * lift * 0.47)
			g += uint32(float64(alpha-g) * lift * 0.68)
			b += uint32(float64(alpha-b) * leading * leading * 0.83)
			color = alpha<<24 | r<<16 | g<<8 | b
		}
		v.pixels[p.index] = startupOver(color, v.frost[p.index])
	}
}

func (v *startupVisual) present(hwnd uintptr, elapsed float64, animate bool) bool {
	v.draw(elapsed, animate)
	size := pointT{X: startupWidth, Y: startupHeight}
	origin := pointT{}
	blend := startupBlend{opacity: 255, alpha: 1}
	ok, _, _ := startupLayer.Call(hwnd, 0, 0, uintptr(unsafe.Pointer(&size)), v.dc,
		uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), 2) // ULW_ALPHA
	return ok != 0
}

func (v *startupVisual) print(dc uintptr) {
	startupCopy.Call(dc, 0, 0, startupWidth, startupHeight, v.dc, 0, 0, 0x00CC0020) // SRCCOPY
	startupFlush.Call()
}
