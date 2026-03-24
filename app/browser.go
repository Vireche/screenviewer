package screenviewer

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	_ "golang.org/x/image/webp"
)

const browserPanelWidth = 110

const folderEntryPrefix = "📁 "

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
}

// comCall calls a COM vtable method on an interface pointer.
func comCall(obj uintptr, methodIndex uintptr, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	method := *(*uintptr)(unsafe.Pointer(vtbl + methodIndex*unsafe.Sizeof(uintptr(0))))
	ret, _, _ := syscall.SyscallN(method, append([]uintptr{obj}, args...)...)
	return ret
}

// pickFolderDialog shows a modern IFileOpenDialog configured for folder selection.
func pickFolderDialog(owner win.HWND, title string) (string, bool) {
	// CLSID_FileOpenDialog {DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}
	clsid := win.CLSID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	// IID_IFileOpenDialog {D57C7288-D4AD-4768-BE02-9D969532D960}
	iid := win.IID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}

	var pfd unsafe.Pointer
	hr := win.CoCreateInstance(&clsid, nil, win.CLSCTX_INPROC_SERVER, &iid, &pfd)
	if hr != win.S_OK {
		return "", false
	}
	obj := uintptr(pfd)
	defer comCall(obj, 2) // Release

	// IFileDialog vtable indices (IUnknown:0-2, IModalWindow:3, IFileDialog:4+)
	const (
		idxShow       = 3
		idxSetOptions = 9
		idxGetOptions = 10
		idxSetTitle   = 17
		idxGetResult  = 20
	)

	// Get current options and add FOS_PICKFOLDERS (0x20).
	var opts uint32
	comCall(obj, idxGetOptions, uintptr(unsafe.Pointer(&opts)))
	comCall(obj, idxSetOptions, uintptr(opts|0x20))

	if title != "" {
		titlePtr, _ := syscall.UTF16PtrFromString(title)
		comCall(obj, idxSetTitle, uintptr(unsafe.Pointer(titlePtr)))
	}

	if comCall(obj, idxShow, uintptr(owner)) != 0 {
		return "", false
	}

	// GetResult -> IShellItem
	var psi uintptr
	if comCall(obj, idxGetResult, uintptr(unsafe.Pointer(&psi))) != 0 {
		return "", false
	}
	defer comCall(psi, 2) // Release IShellItem

	// IShellItem::GetDisplayName(SIGDN_FILESYSPATH)
	const sigdnFileSysPath = 0x80058000
	var pwsz uintptr
	if comCall(psi, 5, sigdnFileSysPath, uintptr(unsafe.Pointer(&pwsz))) != 0 || pwsz == 0 {
		return "", false
	}
	defer win.CoTaskMemFree(pwsz)

	// Convert wide string to Go string.
	path := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(pwsz)), 32768))
	return path, true
}

func (app *viewerApp) pickImageDirectory() {
	path, ok := pickFolderDialog(app.mainWindow.Handle(), "Select Image Directory")
	if !ok {
		return
	}

	app.browserDir = path
	app.refreshImageList()
}

func (app *viewerApp) refreshImageList() {
	entries, err := os.ReadDir(app.browserDir)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Failed to read directory: %v", err))
		return
	}

	files := make([]string, 0, len(entries)+1)
	isdirs := make([]bool, 0, len(entries)+1)

	// ".." entry to navigate up, unless already at root.
	if filepath.Dir(app.browserDir) != app.browserDir {
		files = append(files, ".. (up)")
		isdirs = append(isdirs, true)
	}

	// Directories first.
	for _, entry := range entries {
		if entry.IsDir() {
			files = append(files, folderEntryPrefix+entry.Name())
			isdirs = append(isdirs, true)
		}
	}
	// Then image files.
	for _, entry := range entries {
		if !entry.IsDir() {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if imageExtensions[ext] {
				files = append(files, entry.Name())
				isdirs = append(isdirs, false)
			}
		}
	}

	app.browserAllFiles = files
	app.browserAllIsDirs = isdirs
	if app.browserFilter != nil {
		_ = app.browserFilter.SetText("")
	}
	app.applyBrowserFilter()
	imageCount := 0
	for _, d := range isdirs {
		if !d {
			imageCount++
		}
	}
	app.setStatusText(fmt.Sprintf("%d images in %s", imageCount, filepath.Base(app.browserDir)))
}

func (app *viewerApp) applyBrowserFilter() {
	filterText := ""
	if app.browserFilter != nil {
		filterText = strings.ToLower(app.browserFilter.Text())
	}
	if filterText == "" {
		app.browserFiles = app.browserAllFiles
		app.browserItemIsDirs = app.browserAllIsDirs
	} else {
		filtered := make([]string, 0, len(app.browserAllFiles))
		filteredDirs := make([]bool, 0, len(app.browserAllFiles))
		for i, name := range app.browserAllFiles {
			// Always keep directory entries (including "..") visible.
			if app.browserAllIsDirs[i] || strings.Contains(strings.ToLower(name), filterText) {
				filtered = append(filtered, name)
				filteredDirs = append(filteredDirs, app.browserAllIsDirs[i])
			}
		}
		app.browserFiles = filtered
		app.browserItemIsDirs = filteredDirs
	}
	_ = app.imageList.SetModel(app.browserFiles)
}

func (app *viewerApp) openSelectedImage() {
	idx := app.imageList.CurrentIndex()
	if idx < 0 || idx >= len(app.browserFiles) {
		return
	}

	// Navigate into a directory (or up with "..").
	if idx < len(app.browserItemIsDirs) && app.browserItemIsDirs[idx] {
		name := app.browserFiles[idx]
		var newDir string
		if name == ".. (up)" {
			newDir = filepath.Dir(app.browserDir)
		} else {
			// Strip the display prefix added for directory rows.
			folderName := strings.TrimPrefix(name, folderEntryPrefix)
			newDir = filepath.Join(app.browserDir, folderName)
		}
		app.browserDir = newDir
		app.refreshImageList()
		return
	}

	filePath := filepath.Join(app.browserDir, app.browserFiles[idx])
	img, err := loadImageFile(filePath)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Failed to load %s: %v", app.browserFiles[idx], err))
		return
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	// Downsample to target display resolution for the fullscreen window.
	app.stateMu.RLock()
	dispBounds := app.displays[app.displayIndex].bounds
	app.stateMu.RUnlock()
	scaled := downsampleImage(img, dispBounds.Dx(), dispBounds.Dy())
	img = nil // allow GC of original

	bmp, err := walk.NewBitmapFromImage(scaled)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Failed to create bitmap: %v", err))
		return
	}

	app.closeImageViewer()

	app.stateMu.Lock()
	app.imageBitmap = bmp
	app.imageSize = walk.Size{Width: origW, Height: origH}
	app.imageFileName = app.browserFiles[idx]
	app.stateMu.Unlock()

	app.showImageOnDisplay(scaled)
	_ = app.preview.Invalidate()
	app.setStatusText(fmt.Sprintf("Showing %s on display", app.browserFiles[idx]))
}

func loadImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

// downsampleImage returns a version of img scaled down to fit within maxW x maxH.
// If the image already fits, it's returned as-is. Uses nearest-neighbor for speed.
func downsampleImage(img image.Image, maxW, maxH int) image.Image {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()
	if srcW <= maxW && srcH <= maxH {
		return img
	}

	scale := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	// Convert source to RGBA for fast pixel access.
	srcRGBA, ok := img.(*image.RGBA)
	if !ok {
		srcRGBA = image.NewRGBA(img.Bounds())
		draw.Draw(srcRGBA, srcRGBA.Bounds(), img, img.Bounds().Min, draw.Src)
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy := srcRGBA.Bounds().Min.Y + int(float64(y)/scale)
		for x := 0; x < dstW; x++ {
			sx := srcRGBA.Bounds().Min.X + int(float64(x)/scale)
			off := srcRGBA.PixOffset(sx, sy)
			dOff := dst.PixOffset(x, y)
			copy(dst.Pix[dOff:dOff+4], srcRGBA.Pix[off:off+4])
		}
	}
	return dst
}
