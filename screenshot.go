// Capture d'écran complète en pur GDI (gdi32.dll), sans dépendance externe.
// Le bitmap est lu en 32 bits, converti BGRA → RGBA et encodé en PNG.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

var (
	gdi32DLL                = syscall.NewLazyDLL("gdi32.dll")
	procCreateCompatibleDC  = gdi32DLL.NewProc("CreateCompatibleDC")
	procCreateCompatibleBmp = gdi32DLL.NewProc("CreateCompatibleBitmap")
	procSelectObject        = gdi32DLL.NewProc("SelectObject")
	procDeleteObject        = gdi32DLL.NewProc("DeleteObject")
	procDeleteDC            = gdi32DLL.NewProc("DeleteDC")
	procBitBlt              = gdi32DLL.NewProc("BitBlt")
	procGetDIBits           = gdi32DLL.NewProc("GetDIBits")
	procGetDC               = user32DLL.NewProc("GetDC")
	procReleaseDC           = user32DLL.NewProc("ReleaseDC")
	procEnumDisplayMonitors = user32DLL.NewProc("EnumDisplayMonitors")
)

const (
	srccopy      = 0x00CC0020
	dibRGBColors = 0
	biRGB        = 0
	smXVirtual   = 76
	smYVirtual   = 77
	smCXVirtual  = 78
	smCYVirtual  = 79
)

// bitmapInfoHeader fait 40 octets, aligné comme en C.
type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// bitmapInfo = en-tête + une entrée de table de couleurs (vide en 32 bits).
type bitmapInfo struct {
	hdr bitmapInfoHeader
	rgb [1]uint32
}

// captureRectPNG capture une zone (coordonnées écran virtuel) en PNG.
func captureRectPNG(x, y, w, h int32) ([]byte, error) {
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return nil, fmt.Errorf("GetDC impossible")
	}
	defer procReleaseDC.Call(0, screen)

	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("dimensions écran invalides (%dx%d)", w, h)
	}

	mem, _, _ := procCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC impossible")
	}
	defer procDeleteDC.Call(mem)

	bmp, _, _ := procCreateCompatibleBmp.Call(screen, uintptr(uint32(w)), uintptr(uint32(h)))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap impossible")
	}
	defer procDeleteObject.Call(bmp)

	old, _, _ := procSelectObject.Call(mem, bmp)
	if old == 0 || old == 0xFFFFFFFF { // GDI_ERROR
		return nil, fmt.Errorf("SelectObject impossible")
	}

	r, _, _ := procBitBlt.Call(mem, 0, 0, uintptr(uint32(w)), uintptr(uint32(h)),
		screen, uintptr(uint32(x)), uintptr(uint32(y)), srccopy)
	// On remet l'ancien objet AVANT GetDIBits : la bitmap ne doit plus être
	// sélectionnée dans un contexte pour être lue.
	procSelectObject.Call(mem, old)
	if r == 0 {
		return nil, fmt.Errorf("BitBlt impossible")
	}

	var bi bitmapInfo
	bi.hdr.biSize = uint32(unsafe.Sizeof(bi.hdr))
	bi.hdr.biWidth = w
	bi.hdr.biHeight = -h // haut → bas, l'ordre naturel d'une image
	bi.hdr.biPlanes = 1
	bi.hdr.biBitCount = 32
	bi.hdr.biCompression = biRGB
	bi.hdr.biSizeImage = uint32(w) * uint32(h) * 4

	buf := make([]byte, bi.hdr.biSizeImage)
	gr, _, _ := procGetDIBits.Call(mem, bmp, 0, uintptr(uint32(h)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors)
	if gr == 0 {
		return nil, fmt.Errorf("GetDIBits impossible")
	}

	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	copy(img.Pix, buf)
	// Le flux est B,G,R,A ; image attend R,G,B,A.
	for i := 0; i+3 < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
	}
	img.Stride = int(w) * 4

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("png: %w", err)
	}
	return out.Bytes(), nil
}

// captureScreenPNG renvoie l'image de l'ensemble des écrans (écran virtuel),
// en PNG.
func captureScreenPNG() ([]byte, error) {
	w, _, _ := procSysMetrics.Call(smCXVirtual)
	h, _, _ := procSysMetrics.Call(smCYVirtual)
	x, _, _ := procSysMetrics.Call(smXVirtual)
	y, _, _ := procSysMetrics.Call(smYVirtual)
	if w == 0 || h == 0 {
		w, _, _ = procSysMetrics.Call(0)
		h, _, _ = procSysMetrics.Call(1)
		x, y = 0, 0
	}
	return captureRectPNG(int32(int(x)), int32(int(y)), int32(int(w)), int32(int(h)))
}

// captureScreensPNG renvoie une image PNG par moniteur : sur une machine à
// plusieurs écrans, une seule image fusionnée perd les détails. Retombe sur
// la capture virtuelle si l'énumération échoue.
func captureScreensPNG() [][]byte {
	var out [][]byte
	for _, r := range enumerateMonitors() {
		data, err := captureRectPNG(r.L, r.T, r.R-r.L, r.B-r.T)
		if err != nil {
			plog("capture: " + err.Error())
			continue
		}
		if len(data) > 0 {
			out = append(out, data)
		}
	}
	if len(out) > 0 {
		return out
	}
	data, err := captureScreenPNG()
	if err != nil {
		plog("capture: " + err.Error())
		return nil
	}
	if len(data) > 0 {
		return [][]byte{data}
	}
	return nil
}

// winRect fait la taille d'un RECT Win32 (4 × int32).
type winRect struct {
	L, T, R, B int32
}

// enumerateMonitors liste les rectangles des moniteurs (coordonnées écran
// virtuel). Renvoie nil si l'énumération est refusée.
func enumerateMonitors() []winRect {
	var rects []winRect
	// Signature typée : le runtime convertit l'argument pointeur, aucun
	// passage uintptr → unsafe.Pointer dans le code Go (vet reste propre).
	cb := syscall.NewCallback(func(hmon, hdc uintptr, lprc *winRect, dwData uintptr) uintptr {
		if lprc != nil {
			rects = append(rects, *lprc)
		}
		return 1 // TRUE : continuer
	})
	r, _, _ := procEnumDisplayMonitors.Call(0, 0, cb, 0)
	if r == 0 || len(rects) == 0 {
		return nil
	}
	return rects
}
