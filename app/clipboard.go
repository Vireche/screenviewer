package screenviewer

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// Clipboard Win32 API.
var (
	modUser32Clip                  = syscall.NewLazyDLL("user32.dll")
	modKernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard              = modUser32Clip.NewProc("OpenClipboard")
	procCloseClipboard             = modUser32Clip.NewProc("CloseClipboard")
	procGetClipboardData           = modUser32Clip.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = modUser32Clip.NewProc("IsClipboardFormatAvailable")
	procGlobalLock                 = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock               = modKernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = modKernel32.NewProc("GlobalSize")
)

const (
	cfDIB = 8
)

func (app *viewerApp) pasteFromClipboard() {
	img, err := imageFromClipboard(app.mainWindow.Handle())
	if err != nil {
		app.setStatusText(fmt.Sprintf("Paste failed: %v", err))
		return
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	app.stateMu.RLock()
	dispBounds := app.displays[app.displayIndex].bounds
	app.stateMu.RUnlock()
	scaled := downsampleImage(img, dispBounds.Dx(), dispBounds.Dy())
	img = nil

	bmp, err := walk.NewBitmapFromImage(scaled)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Paste bitmap failed: %v", err))
		return
	}

	app.closeImageViewer()

	app.stateMu.Lock()
	app.imageBitmap = bmp
	app.imageSize = walk.Size{Width: origW, Height: origH}
	app.imageFileName = "(pasted image)"
	app.stateMu.Unlock()

	app.showImageOnDisplay(scaled)
	_ = app.preview.Invalidate()
	app.setStatusText(fmt.Sprintf("Showing pasted image (%dx%d) on display", origW, origH))
}

func imageFromClipboard(hwnd win.HWND) (image.Image, error) {
	ret, _, _ := procOpenClipboard.Call(uintptr(hwnd))
	if ret == 0 {
		return nil, fmt.Errorf("cannot open clipboard")
	}
	defer procCloseClipboard.Call()

	// Prefer CF_DIB for full pixel data.
	ret, _, _ = procIsClipboardFormatAvailable.Call(uintptr(cfDIB))
	if ret != 0 {
		return imageFromClipboardDIB()
	}

	return nil, fmt.Errorf("no image on clipboard")
}

func imageFromClipboardDIB() (image.Image, error) {
	hMem, _, _ := procGetClipboardData.Call(uintptr(cfDIB))
	if hMem == 0 {
		return nil, fmt.Errorf("GetClipboardData failed")
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return nil, fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(hMem)

	size, _, _ := procGlobalSize.Call(hMem)
	if size == 0 {
		return nil, fmt.Errorf("GlobalSize failed")
	}

	data := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)

	// Parse BITMAPINFOHEADER.
	if size < 40 {
		return nil, fmt.Errorf("DIB data too small")
	}
	bi := (*win.BITMAPINFOHEADER)(unsafe.Pointer(ptr))
	w := int(bi.BiWidth)
	h := int(bi.BiHeight)
	bottomUp := h > 0
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 32768 || h > 32768 {
		return nil, fmt.Errorf("invalid DIB dimensions %dx%d", w, h)
	}

	bpp := int(bi.BiBitCount)
	if bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("unsupported bit depth: %d", bpp)
	}

	// Calculate offset to pixel data (header + color table).
	headerSize := int(bi.BiSize)
	colorTableSize := 0
	if bi.BiCompression == win.BI_RGB && bpp == 24 {
		colorTableSize = 0
	}
	pixelOffset := headerSize + colorTableSize

	srcStride := ((w*bpp + 31) / 32) * 4 // DWORD-aligned row stride
	needed := pixelOffset + srcStride*h
	if int(size) < needed {
		return nil, fmt.Errorf("DIB data truncated: have %d, need %d", size, needed)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	pixels := data[pixelOffset:]

	for y := 0; y < h; y++ {
		// DIBs are bottom-up by default; flip if needed.
		srcY := y
		if bottomUp {
			srcY = h - 1 - y
		}
		srcRow := pixels[srcY*srcStride:]
		for x := 0; x < w; x++ {
			dstOff := img.PixOffset(x, y)
			if bpp == 32 {
				srcOff := x * 4
				img.Pix[dstOff+0] = srcRow[srcOff+2] // R
				img.Pix[dstOff+1] = srcRow[srcOff+1] // G
				img.Pix[dstOff+2] = srcRow[srcOff+0] // B
				img.Pix[dstOff+3] = 255              // A
			} else {
				srcOff := x * 3
				img.Pix[dstOff+0] = srcRow[srcOff+2] // R
				img.Pix[dstOff+1] = srcRow[srcOff+1] // G
				img.Pix[dstOff+2] = srcRow[srcOff+0] // B
				img.Pix[dstOff+3] = 255              // A
			}
		}
	}

	return img, nil
}
