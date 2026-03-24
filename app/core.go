package screenviewer

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/kbinani/screenshot"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

var (
	modUser32Input       = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = modUser32Input.NewProc("GetAsyncKeyState")
)

func isVirtualKeyDown(vk int32) bool {
	state, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return (state & 0x8000) != 0
}

const (
	refreshInterval             = time.Second / 30
	presentInterval             = time.Second / 15
	cursorInvalidateInterval    = time.Second / 24
	cursorSignatureIgnoreRadius = 24
	windowTitle                 = "Monitor Viewer"
	maxInitialWidth             = 1280
	maxInitialHeight            = 720
	statusAreaHeight            = 32
	defaultMinWidth             = 480
	defaultMinHeight            = 320
	windowResizeFlags           = win.SWP_NOMOVE | win.SWP_NOSIZE | win.SWP_NOACTIVATE
)

type displayOption struct {
	index  int
	bounds image.Rectangle
	label  string
}

type viewerApp struct {
	mainWindow             *walk.MainWindow
	preview                *walk.CustomWidget
	statusLabel            *walk.Label
	background             *walk.SolidColorBrush
	displays               []displayOption
	displayActions         []*walk.Action
	displayIndex           int
	alwaysOnTop            bool
	currentBitmap          *walk.Bitmap
	previousBitmap         *walk.Bitmap
	currentFrame           walk.Size
	statusText             string
	lastClientSize         walk.Size
	adjustingSize          bool
	lastPresentAt          time.Time
	lastFrameSig           uint64
	haveFrameSig           bool
	captureTrigger         chan struct{}
	stopCapture            chan struct{}
	stopOnce               sync.Once
	stateMu                sync.RWMutex
	moveSizeHook           win.HWINEVENTHOOK
	windowDropEnabled      bool
	origWndProc            uintptr
	imageWndProc           uintptr
	imageViewerHwnd        win.HWND
	imageDisplayIndex      int
	imageBitmap            *walk.Bitmap
	imageSize              walk.Size
	imageFileName          string
	closeBtnRect           walk.Rectangle
	cursorX                int
	cursorY                int
	cursorVisible          bool
	lastLButtonDown        bool
	lastRButtonDown        bool
	lastMButtonDown        bool
	lastCursorInvalidateAt time.Time
	splitter               *walk.Splitter
	browserPanel           *walk.Composite
	imageList              *walk.ListBox
	browserFilter          *walk.LineEdit
	browserDir             string
	browserAllFiles        []string
	browserAllIsDirs       []bool
	browserFiles           []string
	browserItemIsDirs      []bool
}

func Run() {
	displayCount := screenshot.NumActiveDisplays()
	if displayCount == 0 {
		log.Fatal("no displays were detected")
	}

	displays := makeDisplayOptions(displayCount)
	defaultIndex := 0
	if displayCount > 1 {
		defaultIndex = 1
	}

	app, err := newViewerApp(displays, defaultIndex)
	if err != nil {
		log.Fatal(err)
	}

	app.startCaptureLoop()
	app.startCursorTracking()
	app.captureSoon()
	app.mainWindow.SetVisible(true)
	app.mainWindow.Run()
}

func newViewerApp(displays []displayOption, selectedIndex int) (*viewerApp, error) {
	mainWindow, err := walk.NewMainWindow()
	if err != nil {
		return nil, err
	}

	app := &viewerApp{
		mainWindow:        mainWindow,
		displays:          displays,
		displayIndex:      selectedIndex,
		imageDisplayIndex: -1,
		captureTrigger:    make(chan struct{}, 1),
		stopCapture:       make(chan struct{}),
	}

	if err := mainWindow.SetTitle(windowTitle); err != nil {
		mainWindow.Dispose()
		return nil, err
	}

	background, err := walk.NewSolidColorBrush(walk.RGB(24, 26, 31))
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	mainWindow.AddDisposable(background)
	mainWindow.SetBackground(background)
	app.background = background

	// Main layout: VBox with [HSplitter, StatusLabel].
	mainLayout := walk.NewVBoxLayout()
	mainLayout.SetMargins(walk.Margins{})
	mainLayout.SetSpacing(0)
	if err := mainWindow.SetLayout(mainLayout); err != nil {
		mainWindow.Dispose()
		return nil, err
	}

	// Horizontal splitter for optional browser panel + preview area.
	splitter, err := walk.NewHSplitter(mainWindow)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	app.splitter = splitter

	// Image browser panel (starts hidden).
	browserPanel, err := walk.NewComposite(splitter)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	browserLayout := walk.NewVBoxLayout()
	browserLayout.SetMargins(walk.Margins{HNear: 4, VNear: 4, HFar: 4, VFar: 4})
	browserLayout.SetSpacing(2)
	if err := browserPanel.SetLayout(browserLayout); err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	app.browserPanel = browserPanel

	browserUIFont, _ := walk.NewFont("Segoe UI", 10, 0)

	browseBtn, err := walk.NewPushButton(browserPanel)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	if err := browseBtn.SetText("Browse Folder..."); err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	if browserUIFont != nil {
		browseBtn.SetFont(browserUIFont)
	}
	browseBtn.Clicked().Attach(func() {
		app.pickImageDirectory()
	})
	_ = browserLayout.SetStretchFactor(browseBtn, 0)

	// Filter text field.
	filterEdit, err := walk.NewLineEdit(browserPanel)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	_ = filterEdit.SetCueBanner("Filter files...")
	filterEdit.TextChanged().Attach(func() {
		app.applyBrowserFilter()
	})
	if browserUIFont != nil {
		filterEdit.SetFont(browserUIFont)
	}
	app.browserFilter = filterEdit
	_ = browserLayout.SetStretchFactor(filterEdit, 0)

	imageList, err := walk.NewListBox(browserPanel)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	listFont, _ := walk.NewFont("Segoe UI Emoji", 10, 0)
	if listFont == nil {
		listFont, _ = walk.NewFont("Segoe UI", 10, 0)
	}
	if listFont != nil {
		imageList.SetFont(listFont)
	}
	imageList.CurrentIndexChanged().Attach(func() {
		app.openSelectedImage()
	})
	app.imageList = imageList
	_ = browserLayout.SetStretchFactor(imageList, 1)

	browserPanel.SetVisible(false)
	_ = browserPanel.SetMinMaxSizePixels(walk.Size{Width: 75, Height: 0}, walk.Size{})
	_ = splitter.SetFixed(browserPanel, true)

	// Preview area (right side of splitter).
	preview, err := walk.NewCustomWidgetPixels(splitter, 0, app.paintPreview)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}

	// Give the browser panel a small stretch factor so it gets ~1/10 of space on reset.
	type stretchSetter interface {
		SetStretchFactor(widget walk.Widget, factor int) error
	}
	if ss, ok := splitter.Layout().(stretchSetter); ok {
		_ = ss.SetStretchFactor(browserPanel, 1)
		_ = ss.SetStretchFactor(preview, 4)
	}
	preview.SetBackground(background)
	preview.SetClearsBackground(false)
	preview.SetPaintMode(walk.PaintNoErase)
	preview.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		app.handlePreviewClick(x, y)
	})
	app.preview = preview

	statusLabel, err := walk.NewLabel(mainWindow)
	if err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	if err := statusLabel.SetText("Waiting for first frame..."); err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	app.statusLabel = statusLabel
	app.statusText = "Waiting for first frame..."

	if err := mainWindow.SetMinMaxSizePixels(walk.Size{Width: defaultMinWidth, Height: defaultMinHeight}, walk.Size{}); err != nil {
		mainWindow.Dispose()
		return nil, err
	}

	if err := app.resizeWindowForDisplay(displays[selectedIndex]); err != nil {
		mainWindow.Dispose()
		return nil, err
	}

	if err := app.configureMenus(); err != nil {
		mainWindow.Dispose()
		return nil, err
	}
	app.installAspectRatioLock()
	app.installWindowDropHook()
	app.enableFileDrop()

	mainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		app.shutdown()
	})

	return app, nil
}

func (app *viewerApp) configureMenus() error {
	fileMenu, err := walk.NewMenu()
	if err != nil {
		return err
	}
	fileMenuAction, err := app.mainWindow.Menu().Actions().AddMenu(fileMenu)
	if err != nil {
		return err
	}
	if err := fileMenuAction.SetText("&File"); err != nil {
		return err
	}

	exitAction := walk.NewAction()
	if err := exitAction.SetText("E&xit"); err != nil {
		return err
	}
	exitAction.Triggered().Attach(func() {
		_ = app.mainWindow.Close()
	})
	if err := fileMenu.Actions().Add(exitAction); err != nil {
		return err
	}

	// Edit menu with Copy / Paste.
	editMenu, err := walk.NewMenu()
	if err != nil {
		return err
	}
	editMenuAction, err := app.mainWindow.Menu().Actions().AddMenu(editMenu)
	if err != nil {
		return err
	}
	if err := editMenuAction.SetText("&Edit"); err != nil {
		return err
	}

	pasteAction := walk.NewAction()
	if err := pasteAction.SetText("&Paste\tCtrl+V"); err != nil {
		return err
	}
	if err := pasteAction.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: walk.KeyV}); err != nil {
		return err
	}
	pasteAction.Triggered().Attach(func() {
		app.pasteFromClipboard()
	})
	if err := editMenu.Actions().Add(pasteAction); err != nil {
		return err
	}

	viewMenu, err := walk.NewMenu()
	if err != nil {
		return err
	}
	viewMenuAction, err := app.mainWindow.Menu().Actions().AddMenu(viewMenu)
	if err != nil {
		return err
	}
	if err := viewMenuAction.SetText("&View"); err != nil {
		return err
	}

	alwaysOnTopAction := walk.NewAction()
	if err := alwaysOnTopAction.SetText("&Always on top"); err != nil {
		return err
	}
	if err := alwaysOnTopAction.SetCheckable(true); err != nil {
		return err
	}
	alwaysOnTopAction.Triggered().Attach(func() {
		app.toggleAlwaysOnTop(alwaysOnTopAction.Checked())
	})
	if err := viewMenu.Actions().Add(alwaysOnTopAction); err != nil {
		return err
	}

	windowDropAction := walk.NewAction()
	if err := windowDropAction.SetText("&Drag window to display"); err != nil {
		return err
	}
	if err := windowDropAction.SetCheckable(true); err != nil {
		return err
	}
	windowDropAction.Triggered().Attach(func() {
		app.toggleWindowDrop(windowDropAction.Checked())
	})
	if err := viewMenu.Actions().Add(windowDropAction); err != nil {
		return err
	}

	imageBrowserAction := walk.NewAction()
	if err := imageBrowserAction.SetText("&Image browser"); err != nil {
		return err
	}
	if err := imageBrowserAction.SetCheckable(true); err != nil {
		return err
	}
	imageBrowserAction.Triggered().Attach(func() {
		app.toggleImageBrowser(imageBrowserAction.Checked())
	})
	if err := viewMenu.Actions().Add(imageBrowserAction); err != nil {
		return err
	}

	if err := viewMenu.Actions().Add(walk.NewSeparatorAction()); err != nil {
		return err
	}

	app.displayActions = make([]*walk.Action, 0, len(app.displays))
	for displayIndex, display := range app.displays {
		action := walk.NewAction()
		if err := action.SetText(display.label); err != nil {
			return err
		}
		if err := action.SetCheckable(true); err != nil {
			return err
		}
		index := displayIndex
		action.Triggered().Attach(func() {
			app.selectDisplay(index)
		})
		if err := viewMenu.Actions().Add(action); err != nil {
			return err
		}
		app.displayActions = append(app.displayActions, action)
	}

	app.syncDisplayMenuChecks()
	return nil
}

func (app *viewerApp) startCaptureLoop() {
	ticker := time.NewTicker(refreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				app.captureFrame()
			case <-app.captureTrigger:
				app.captureFrame()
			case <-app.stopCapture:
				return
			}
		}
	}()
}

func (app *viewerApp) captureSoon() {
	select {
	case app.captureTrigger <- struct{}{}:
	default:
	}
}

func (app *viewerApp) captureFrame() {
	app.stateMu.RLock()
	display := app.displays[app.displayIndex]
	alwaysOnTop := app.alwaysOnTop
	hasImageThumbnail := app.imageBitmap != nil
	cursorVisible := app.cursorVisible
	cursorX := app.cursorX
	cursorY := app.cursorY
	app.stateMu.RUnlock()

	// Keep preview stable while fullscreen image mode is active.
	if hasImageThumbnail {
		return
	}

	frame, err := screenshot.CaptureDisplay(display.index)
	if err != nil {
		app.mainWindow.Synchronize(func() {
			app.setStatusText(fmt.Sprintf("Capture failed for %s: %v", display.label, err))
		})
		return
	}

	// Normalize to a zero-origin image to avoid partial rendering issues with non-zero bounds.
	frame = normalizeRGBA(frame)

	var ignoreRect *image.Rectangle
	if cursorVisible {
		relX := cursorX - display.bounds.Min.X
		relY := cursorY - display.bounds.Min.Y
		if relX >= 0 && relX < frame.Bounds().Dx() && relY >= 0 && relY < frame.Bounds().Dy() {
			r := cursorSignatureIgnoreRadius
			ir := image.Rect(relX-r, relY-r, relX+r+1, relY+r+1).Intersect(frame.Bounds())
			if !ir.Empty() {
				ignoreRect = &ir
			}
		}
	}

	frameSig := sampleFrameSignature(frame, ignoreRect)
	now := time.Now()
	app.stateMu.Lock()
	sameFrame := app.haveFrameSig && frameSig == app.lastFrameSig
	tooSoon := !app.lastPresentAt.IsZero() && now.Sub(app.lastPresentAt) < presentInterval
	if sameFrame || tooSoon {
		app.stateMu.Unlock()
		return
	}
	app.lastFrameSig = frameSig
	app.haveFrameSig = true
	app.lastPresentAt = now
	app.stateMu.Unlock()

	bitmap, err := walk.NewBitmapFromImage(frame)
	if err != nil {
		app.mainWindow.Synchronize(func() {
			app.setStatusText(fmt.Sprintf("Bitmap conversion failed: %v", err))
		})
		return
	}

	frameWidth := frame.Bounds().Dx()
	frameHeight := frame.Bounds().Dy()
	status := fmt.Sprintf("%s | %dx%d | Always on top: %s", display.label, frameWidth, frameHeight, onOff(alwaysOnTop))
	app.mainWindow.Synchronize(func() {
		app.stateMu.Lock()
		oldBitmap := app.currentBitmap
		staleBitmap := app.previousBitmap
		app.currentBitmap = bitmap
		app.previousBitmap = oldBitmap
		app.currentFrame = walk.Size{Width: frameWidth, Height: frameHeight}
		app.stateMu.Unlock()
		_ = app.preview.Invalidate()
		app.setStatusText(status)
		if staleBitmap != nil {
			staleBitmap.Dispose()
		}
	})
}

func (app *viewerApp) selectDisplay(index int) {
	app.stateMu.Lock()
	if index == app.displayIndex {
		app.stateMu.Unlock()
		return
	}
	closeImageOnSwitch := app.imageViewerHwnd != 0 && app.imageDisplayIndex == index
	app.displayIndex = index
	app.haveFrameSig = false
	app.lastPresentAt = time.Time{}
	display := app.displays[index]
	app.stateMu.Unlock()

	if closeImageOnSwitch {
		app.closeImageViewer()
	}

	app.syncDisplayMenuChecks()
	if err := app.resizeWindowForDisplay(display); err != nil {
		app.setStatusText(fmt.Sprintf("Window resize failed: %v", err))
		return
	}
	app.stateMu.Lock()
	oldBitmap := app.currentBitmap
	staleBitmap := app.previousBitmap
	app.currentBitmap = nil
	app.previousBitmap = nil
	app.currentFrame = walk.Size{}
	app.stateMu.Unlock()
	if oldBitmap != nil {
		oldBitmap.Dispose()
	}
	if staleBitmap != nil {
		staleBitmap.Dispose()
	}
	_ = app.preview.Invalidate()
	app.setStatusText(fmt.Sprintf("Switching to %s...", display.label))
	app.captureSoon()
}

func (app *viewerApp) syncDisplayMenuChecks() {
	for displayIndex, action := range app.displayActions {
		_ = action.SetChecked(displayIndex == app.displayIndex)
	}
}

func (app *viewerApp) toggleAlwaysOnTop(enabled bool) {
	app.stateMu.Lock()
	app.alwaysOnTop = enabled
	app.haveFrameSig = false
	app.lastPresentAt = time.Time{}
	app.stateMu.Unlock()

	insertAfter := win.HWND_NOTOPMOST
	if enabled {
		insertAfter = win.HWND_TOPMOST
	}
	win.SetWindowPos(app.mainWindow.Handle(), insertAfter, 0, 0, 0, 0, windowResizeFlags)
	app.captureSoon()
}

func (app *viewerApp) toggleWindowDrop(enabled bool) {
	app.stateMu.Lock()
	app.windowDropEnabled = enabled
	app.stateMu.Unlock()
}

func (app *viewerApp) toggleImageBrowser(visible bool) {
	app.browserPanel.SetVisible(visible)
	if visible && len(app.browserFiles) == 0 {
		app.pickImageDirectory()
	}
	app.splitter.RequestLayout()
	_ = app.preview.Invalidate()

	// Force a fresh frame so the preview redraws at the new size.
	app.stateMu.Lock()
	app.haveFrameSig = false
	app.lastPresentAt = time.Time{}
	app.stateMu.Unlock()
	app.captureSoon()
}

func (app *viewerApp) resizeWindowForDisplay(display displayOption) error {
	windowWidth, windowHeight := fitWindow(display.bounds.Dx(), display.bounds.Dy(), maxInitialWidth, maxInitialHeight)
	clientSize := walk.Size{Width: windowWidth, Height: windowHeight + statusAreaHeight}

	app.stateMu.Lock()
	app.adjustingSize = true
	app.lastClientSize = clientSize
	app.stateMu.Unlock()

	err := app.mainWindow.SetClientSizePixels(clientSize)

	app.stateMu.Lock()
	app.adjustingSize = false
	app.stateMu.Unlock()

	return err
}

func (app *viewerApp) shutdown() {
	app.stopOnce.Do(func() {
		if app.moveSizeHook != 0 {
			win.UnhookWinEvent(app.moveSizeHook)
			app.moveSizeHook = 0
		}
		app.closeImageViewer()
		close(app.stopCapture)
		app.mainWindow.Synchronize(func() {
			app.stateMu.Lock()
			oldBitmap := app.currentBitmap
			staleBitmap := app.previousBitmap
			app.currentBitmap = nil
			app.previousBitmap = nil
			app.currentFrame = walk.Size{}
			app.stateMu.Unlock()
			_ = app.preview.Invalidate()
			if oldBitmap != nil {
				oldBitmap.Dispose()
			}
			if staleBitmap != nil {
				staleBitmap.Dispose()
			}
		})
	})
}

func makeDisplayOptions(displayCount int) []displayOption {
	displays := make([]displayOption, 0, displayCount)
	for index := 0; index < displayCount; index++ {
		bounds := screenshot.GetDisplayBounds(index)
		label := fmt.Sprintf("Display %d (%dx%d @ %d,%d)", index+1, bounds.Dx(), bounds.Dy(), bounds.Min.X, bounds.Min.Y)
		displays = append(displays, displayOption{
			index:  index,
			bounds: bounds,
			label:  label,
		})
	}

	return displays
}

func fitWindow(sourceWidth int, sourceHeight int, maxWidth int, maxHeight int) (int, int) {
	if sourceWidth <= maxWidth && sourceHeight <= maxHeight {
		return sourceWidth, sourceHeight
	}

	scale := math.Min(float64(maxWidth)/float64(sourceWidth), float64(maxHeight)/float64(sourceHeight))
	return int(math.Round(float64(sourceWidth) * scale)), int(math.Round(float64(sourceHeight) * scale))
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}

	return "off"
}

func (app *viewerApp) setStatusText(text string) {
	if text == app.statusText {
		return
	}

	app.statusText = text
	_ = app.statusLabel.SetText(text)
}

func (app *viewerApp) paintPreview(canvas *walk.Canvas, updateBounds walk.Rectangle) error {
	widgetSize := app.preview.SizePixels()
	localBounds := walk.Rectangle{X: 0, Y: 0, Width: widgetSize.Width, Height: widgetSize.Height}

	app.stateMu.RLock()
	bmp := app.currentBitmap
	frameSize := app.currentFrame
	app.stateMu.RUnlock()

	if bmp == nil || frameSize.Width <= 0 || frameSize.Height <= 0 || localBounds.Width <= 0 || localBounds.Height <= 0 {
		return nil
	}

	// Draw to an aspect-fit destination so the entire monitor image is visible.
	dst := fitRect(frameSize, localBounds)
	src := walk.Rectangle{X: 0, Y: 0, Width: frameSize.Width, Height: frameSize.Height}
	if err := canvas.DrawBitmapPartWithOpacityPixels(bmp, dst, src, 255); err != nil {
		return err
	}

	// Repaint only letterbox regions to keep no-erase rendering while avoiding stale edges.
	if dst.Y > localBounds.Y {
		top := walk.Rectangle{X: localBounds.X, Y: localBounds.Y, Width: localBounds.Width, Height: dst.Y - localBounds.Y}
		if err := canvas.FillRectanglePixels(app.background, top); err != nil {
			return err
		}
	}
	bottomY := dst.Y + dst.Height
	boundsBottom := localBounds.Y + localBounds.Height
	if bottomY < boundsBottom {
		bottom := walk.Rectangle{X: localBounds.X, Y: bottomY, Width: localBounds.Width, Height: boundsBottom - bottomY}
		if err := canvas.FillRectanglePixels(app.background, bottom); err != nil {
			return err
		}
	}
	if dst.X > localBounds.X {
		left := walk.Rectangle{X: localBounds.X, Y: dst.Y, Width: dst.X - localBounds.X, Height: dst.Height}
		if err := canvas.FillRectanglePixels(app.background, left); err != nil {
			return err
		}
	}
	rightX := dst.X + dst.Width
	boundsRight := localBounds.X + localBounds.Width
	if rightX < boundsRight {
		right := walk.Rectangle{X: rightX, Y: dst.Y, Width: boundsRight - rightX, Height: dst.Height}
		if err := canvas.FillRectanglePixels(app.background, right); err != nil {
			return err
		}
	}

	// Draw image thumbnail overlay if an image is being shown on the display.
	app.stateMu.RLock()
	imgBmp := app.imageBitmap
	imgSize := app.imageSize
	imgName := app.imageFileName
	app.stateMu.RUnlock()

	if imgBmp != nil && imgSize.Width > 0 && imgSize.Height > 0 {
		if err := app.paintImageThumbnail(canvas, localBounds, imgBmp, imgSize, imgName); err != nil {
			return err
		}
	}

	// Draw cursor overlay when the mouse is on the monitored display.
	app.stateMu.RLock()
	cursorVisible := app.cursorVisible
	cursorX := app.cursorX
	cursorY := app.cursorY
	dispBounds := app.displays[app.displayIndex].bounds
	app.stateMu.RUnlock()

	if imgBmp == nil && cursorVisible && dst.Width > 0 && dst.Height > 0 {
		relX := float64(cursorX-dispBounds.Min.X) / float64(dispBounds.Dx())
		relY := float64(cursorY-dispBounds.Min.Y) / float64(dispBounds.Dy())
		px := dst.X + int(math.Round(relX*float64(dst.Width)))
		py := dst.Y + int(math.Round(relY*float64(dst.Height)))
		const crossLen = 10
		const dotHalf = 3

		// Black outline for contrast on any background.
		outlinePen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(0, 0, 0))
		if outlinePen != nil {
			defer outlinePen.Dispose()
			_ = canvas.DrawLinePixels(outlinePen,
				walk.Point{X: px - crossLen - 1, Y: py},
				walk.Point{X: px + crossLen + 1, Y: py})
			_ = canvas.DrawLinePixels(outlinePen,
				walk.Point{X: px, Y: py - crossLen - 1},
				walk.Point{X: px, Y: py + crossLen + 1})
		}

		// Red crosshair.
		cursorPen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(255, 50, 50))
		if cursorPen != nil {
			defer cursorPen.Dispose()
			_ = canvas.DrawLinePixels(cursorPen,
				walk.Point{X: px - crossLen, Y: py},
				walk.Point{X: px + crossLen, Y: py})
			_ = canvas.DrawLinePixels(cursorPen,
				walk.Point{X: px, Y: py - crossLen},
				walk.Point{X: px, Y: py + crossLen})
		}

		// Red dot at the exact cursor position.
		dotBrush, _ := walk.NewSolidColorBrush(walk.RGB(255, 50, 50))
		if dotBrush != nil {
			defer dotBrush.Dispose()
			dotRect := walk.Rectangle{X: px - dotHalf, Y: py - dotHalf, Width: dotHalf * 2, Height: dotHalf * 2}
			_ = canvas.FillRectanglePixels(dotBrush, dotRect)
		}
	}

	return nil
}

const (
	thumbnailMaxWidth  = 320
	thumbnailMaxHeight = 240
	thumbnailPadding   = 10
	closeBtnSize       = 24
)

func (app *viewerApp) paintImageThumbnail(canvas *walk.Canvas, bounds walk.Rectangle, bmp *walk.Bitmap, imgSize walk.Size, name string) error {
	// Calculate thumbnail size maintaining aspect ratio.
	thumbW, thumbH := fitWindow(imgSize.Width, imgSize.Height, thumbnailMaxWidth, thumbnailMaxHeight)

	// Position in bottom-right corner with padding.
	cardW := thumbW + thumbnailPadding*2
	cardH := thumbH + thumbnailPadding*2 + closeBtnSize
	cardX := bounds.X + bounds.Width - cardW - thumbnailPadding
	cardY := bounds.Y + bounds.Height - cardH - thumbnailPadding

	// Draw card background.
	cardBg, _ := walk.NewSolidColorBrush(walk.RGB(40, 42, 48))
	defer cardBg.Dispose()
	cardRect := walk.Rectangle{X: cardX, Y: cardY, Width: cardW, Height: cardH}
	if err := canvas.FillRectanglePixels(cardBg, cardRect); err != nil {
		return err
	}

	// Draw border.
	borderBrush, _ := walk.NewSolidColorBrush(walk.RGB(80, 82, 88))
	defer borderBrush.Dispose()
	topBorder := walk.Rectangle{X: cardX, Y: cardY, Width: cardW, Height: 1}
	bottomBorder := walk.Rectangle{X: cardX, Y: cardY + cardH - 1, Width: cardW, Height: 1}
	leftBorder := walk.Rectangle{X: cardX, Y: cardY, Width: 1, Height: cardH}
	rightBorder := walk.Rectangle{X: cardX + cardW - 1, Y: cardY, Width: 1, Height: cardH}
	_ = canvas.FillRectanglePixels(borderBrush, topBorder)
	_ = canvas.FillRectanglePixels(borderBrush, bottomBorder)
	_ = canvas.FillRectanglePixels(borderBrush, leftBorder)
	_ = canvas.FillRectanglePixels(borderBrush, rightBorder)

	// Draw thumbnail image.
	thumbX := cardX + (cardW-thumbW)/2
	thumbY := cardY + thumbnailPadding
	thumbDst := walk.Rectangle{X: thumbX, Y: thumbY, Width: thumbW, Height: thumbH}
	bmpSize := bmp.Size()
	thumbSrc := walk.Rectangle{X: 0, Y: 0, Width: bmpSize.Width, Height: bmpSize.Height}
	_ = canvas.DrawBitmapPartWithOpacityPixels(bmp, thumbDst, thumbSrc, 255)

	// Draw close button [X] at top-right corner of card, styled like a standard title bar close button.
	closeBtnX := cardX + cardW - closeBtnSize - 1
	closeBtnY := cardY + 1
	closeRect := walk.Rectangle{X: closeBtnX, Y: closeBtnY, Width: closeBtnSize, Height: closeBtnSize}
	closeBg, _ := walk.NewSolidColorBrush(walk.RGB(196, 43, 28))
	defer closeBg.Dispose()
	_ = canvas.FillRectanglePixels(closeBg, closeRect)

	// Draw the "X" glyph using white lines.
	xPen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(255, 255, 255))
	defer xPen.Dispose()
	m := 7 // margin inside the button for the X lines
	_ = canvas.DrawLinePixels(xPen,
		walk.Point{X: closeBtnX + m, Y: closeBtnY + m},
		walk.Point{X: closeBtnX + closeBtnSize - m, Y: closeBtnY + closeBtnSize - m})
	_ = canvas.DrawLinePixels(xPen,
		walk.Point{X: closeBtnX + closeBtnSize - m, Y: closeBtnY + m},
		walk.Point{X: closeBtnX + m, Y: closeBtnY + closeBtnSize - m})

	// Store close button rect for click detection.
	app.stateMu.Lock()
	app.closeBtnRect = closeRect
	app.stateMu.Unlock()

	return nil
}

func fitRect(source walk.Size, target walk.Rectangle) walk.Rectangle {
	sourceW := float64(source.Width)
	sourceH := float64(source.Height)
	targetW := float64(target.Width)
	targetH := float64(target.Height)

	if sourceW <= 0 || sourceH <= 0 || targetW <= 0 || targetH <= 0 {
		return walk.Rectangle{}
	}

	scale := math.Min(targetW/sourceW, targetH/sourceH)
	if scale <= 0 {
		scale = 1
	}

	drawW := int(math.Round(sourceW * scale))
	drawH := int(math.Round(sourceH * scale))
	offsetX := target.X + (target.Width-drawW)/2
	offsetY := target.Y + (target.Height-drawH)/2

	return walk.Rectangle{X: offsetX, Y: offsetY, Width: drawW, Height: drawH}
}

func sampleFrameSignature(frame *image.RGBA, ignoreRect *image.Rectangle) uint64 {
	bounds := frame.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width == 0 || height == 0 {
		return 0
	}

	var signature uint64 = 1469598103934665603
	const fnvPrime uint64 = 1099511628211

	for yStep := 1; yStep <= 8; yStep++ {
		y := bounds.Min.Y + (yStep*height)/9
		if y >= bounds.Max.Y {
			y = bounds.Max.Y - 1
		}
		for xStep := 1; xStep <= 8; xStep++ {
			x := bounds.Min.X + (xStep*width)/9
			if x >= bounds.Max.X {
				x = bounds.Max.X - 1
			}
			if ignoreRect != nil && x >= ignoreRect.Min.X && x < ignoreRect.Max.X &&
				y >= ignoreRect.Min.Y && y < ignoreRect.Max.Y {
				continue
			}
			offset := frame.PixOffset(x, y)
			if offset+2 >= len(frame.Pix) {
				continue
			}
			signature ^= uint64(frame.Pix[offset])
			signature *= fnvPrime
			signature ^= uint64(frame.Pix[offset+1])
			signature *= fnvPrime
			signature ^= uint64(frame.Pix[offset+2])
			signature *= fnvPrime
		}
	}

	return signature
}

func normalizeRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	if b.Min.X == 0 && b.Min.Y == 0 {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func (app *viewerApp) installAspectRatioLock() {
	app.mainWindow.SizeChanged().Attach(func() {
		app.enforceAspectRatio()
	})
}

func (app *viewerApp) enforceAspectRatio() {
	clientRect := app.mainWindow.ClientBoundsPixels()
	clientSize := clientRect.Size()
	if clientSize.Width <= 0 || clientSize.Height <= statusAreaHeight {
		return
	}

	app.stateMu.Lock()
	if app.adjustingSize {
		app.lastClientSize = clientSize
		app.stateMu.Unlock()
		return
	}

	display := app.displays[app.displayIndex]
	lastSize := app.lastClientSize
	app.stateMu.Unlock()

	targetAspect := float64(display.bounds.Dx()) / float64(display.bounds.Dy())
	previewHeight := clientSize.Height - statusAreaHeight
	currentAspect := float64(clientSize.Width) / float64(previewHeight)

	if math.Abs(currentAspect-targetAspect) < 0.01 {
		app.stateMu.Lock()
		app.lastClientSize = clientSize
		app.stateMu.Unlock()
		return
	}

	widthDelta := absInt(clientSize.Width - lastSize.Width)
	heightDelta := absInt(clientSize.Height - lastSize.Height)

	newWidth := clientSize.Width
	newHeight := clientSize.Height
	if widthDelta >= heightDelta {
		newPreviewHeight := int(math.Round(float64(newWidth) / targetAspect))
		if newPreviewHeight < 1 {
			newPreviewHeight = 1
		}
		newHeight = newPreviewHeight + statusAreaHeight
	} else {
		newPreviewWidth := int(math.Round(float64(previewHeight) * targetAspect))
		if newPreviewWidth < 1 {
			newPreviewWidth = 1
		}
		newWidth = newPreviewWidth
	}

	if newWidth < defaultMinWidth {
		newWidth = defaultMinWidth
		newPreviewHeight := int(math.Round(float64(newWidth) / targetAspect))
		if newPreviewHeight < 1 {
			newPreviewHeight = 1
		}
		newHeight = newPreviewHeight + statusAreaHeight
	}
	if newHeight < defaultMinHeight {
		newHeight = defaultMinHeight
		newPreviewHeight := newHeight - statusAreaHeight
		if newPreviewHeight < 1 {
			newPreviewHeight = 1
		}
		newWidth = int(math.Round(float64(newPreviewHeight) * targetAspect))
	}

	if newWidth == clientSize.Width && newHeight == clientSize.Height {
		app.stateMu.Lock()
		app.lastClientSize = clientSize
		app.stateMu.Unlock()
		return
	}

	newSize := walk.Size{Width: newWidth, Height: newHeight}
	app.stateMu.Lock()
	app.adjustingSize = true
	app.lastClientSize = newSize
	app.stateMu.Unlock()

	_ = app.mainWindow.SetClientSizePixels(newSize)

	app.stateMu.Lock()
	app.adjustingSize = false
	app.stateMu.Unlock()
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

// installWindowDropHook sets up a system-wide event hook that detects when a
// window finishes being dragged over the viewer. If so, it moves that window
// to the monitored display and maximizes it.
func (app *viewerApp) installWindowDropHook() {
	var modUser32 = syscall.NewLazyDLL("user32.dll")
	var procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")

	callback := func(hWinEventHook win.HWINEVENTHOOK, event uint32, hwnd win.HWND, idObject int32, idChild int32, idEventThread uint32, dwmsEventTime uint32) uintptr {
		if event != win.EVENT_SYSTEM_MOVESIZEEND {
			return 0
		}
		app.stateMu.RLock()
		enabled := app.windowDropEnabled
		app.stateMu.RUnlock()
		if !enabled {
			return 0
		}
		if hwnd == 0 || hwnd == app.mainWindow.Handle() {
			return 0
		}

		// Check that the cursor is over the viewer window.
		var cursor win.POINT
		if !win.GetCursorPos(&cursor) {
			return 0
		}
		viewerHwnd := app.mainWindow.Handle()
		var viewerRect win.RECT
		if !win.GetWindowRect(viewerHwnd, &viewerRect) {
			return 0
		}
		if cursor.X < viewerRect.Left || cursor.X > viewerRect.Right ||
			cursor.Y < viewerRect.Top || cursor.Y > viewerRect.Bottom {
			return 0
		}

		// Get the top-level owner of the dropped window.
		topLevel := win.GetAncestor(hwnd, win.GA_ROOT)
		if topLevel == 0 {
			topLevel = hwnd
		}
		if topLevel == viewerHwnd {
			return 0
		}

		// Ensure the window belongs to another process (not us).
		var pid uint32
		procGetWindowThreadProcessId.Call(uintptr(topLevel), uintptr(unsafe.Pointer(&pid)))

		// Move to the monitored display and maximize.
		app.stateMu.RLock()
		display := app.displays[app.displayIndex]
		app.stateMu.RUnlock()

		bounds := display.bounds
		win.SetWindowPos(topLevel, 0,
			int32(bounds.Min.X), int32(bounds.Min.Y),
			int32(bounds.Dx()), int32(bounds.Dy()),
			win.SWP_NOZORDER|win.SWP_NOACTIVATE)
		win.ShowWindow(topLevel, win.SW_MAXIMIZE)

		return 0
	}

	hook, err := win.SetWinEventHook(
		win.EVENT_SYSTEM_MOVESIZEEND,
		win.EVENT_SYSTEM_MOVESIZEEND,
		0,
		callback,
		0, 0,
		win.WINEVENT_OUTOFCONTEXT,
	)
	if err == nil && hook != 0 {
		app.moveSizeHook = hook
	}
}

// enableFileDrop enables file drag-and-drop on the main window and subclasses
// its WndProc to handle WM_DROPFILES messages.
func (app *viewerApp) enableFileDrop() {
	hwnd := app.mainWindow.Handle()
	win.DragAcceptFiles(hwnd, true)

	app.origWndProc = win.SetWindowLongPtr(hwnd, win.GWLP_WNDPROC,
		syscall.NewCallback(func(h win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
			switch msg {
			case win.WM_DROPFILES:
				app.handleDropFiles(win.HDROP(wParam))
				return 0
			}
			return win.CallWindowProc(app.origWndProc, h, msg, wParam, lParam)
		}))
}

func (app *viewerApp) handleDropFiles(hDrop win.HDROP) {
	defer win.DragFinish(hDrop)

	fileCount := win.DragQueryFile(hDrop, 0xFFFFFFFF, nil, 0)
	if fileCount == 0 {
		return
	}

	// Take the first file only.
	var buf [260]uint16
	win.DragQueryFile(hDrop, 0, &buf[0], 260)
	filePath := syscall.UTF16ToString(buf[:])

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
	default:
		app.setStatusText(fmt.Sprintf("Unsupported image format: %s", ext))
		return
	}

	img, err := loadImageFile(filePath)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Failed to load image: %v", err))
		return
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	// Downsample to target display resolution.
	app.stateMu.RLock()
	dispBounds := app.displays[app.displayIndex].bounds
	app.stateMu.RUnlock()
	scaled := downsampleImage(img, dispBounds.Dx(), dispBounds.Dy())
	img = nil

	bmp, err := walk.NewBitmapFromImage(scaled)
	if err != nil {
		app.setStatusText(fmt.Sprintf("Failed to create bitmap: %v", err))
		return
	}

	app.closeImageViewer()

	app.stateMu.Lock()
	app.imageBitmap = bmp
	app.imageSize = walk.Size{Width: origW, Height: origH}
	app.imageFileName = filepath.Base(filePath)
	app.stateMu.Unlock()

	app.showImageOnDisplay(scaled)
	_ = app.preview.Invalidate()
	app.setStatusText(fmt.Sprintf("Showing %s on display", app.imageFileName))
}

// showImageOnDisplay creates a borderless fullscreen window on the target
// monitor and paints the image centered/scaled to fit.
func (app *viewerApp) showImageOnDisplay(img image.Image) {
	app.stateMu.RLock()
	targetDisplayIndex := app.displayIndex
	display := app.displays[targetDisplayIndex]
	app.stateMu.RUnlock()

	bounds := display.bounds
	className := syscall.StringToUTF16Ptr("Static")
	windowName := syscall.StringToUTF16Ptr("Image Viewer")

	hwnd := win.CreateWindowEx(
		0,
		className,
		windowName,
		win.WS_POPUP|win.WS_VISIBLE,
		int32(bounds.Min.X), int32(bounds.Min.Y),
		int32(bounds.Dx()), int32(bounds.Dy()),
		0, 0, 0, nil,
	)
	if hwnd == 0 {
		app.setStatusText("Failed to create image viewer window")
		return
	}

	app.stateMu.Lock()
	app.imageViewerHwnd = hwnd
	app.imageDisplayIndex = targetDisplayIndex
	app.stateMu.Unlock()

	// Raise above regular windows once, without making it permanently topmost.
	win.SetWindowPos(hwnd, win.HWND_TOP, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE)
	win.SetForegroundWindow(hwnd)
	win.SetFocus(hwnd)
	win.SetCapture(hwnd)

	// Create a GDI bitmap for direct painting via StretchBlt.
	hBmp := createHBitmapFromImage(img)
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()
	dispW := bounds.Dx()
	dispH := bounds.Dy()

	procFillRect := syscall.NewLazyDLL("user32.dll").NewProc("FillRect")

	var origStaticProc uintptr
	imageWndProc := syscall.NewCallback(func(h win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case win.WM_NCHITTEST:
			// Keep input on this fullscreen window (do not pass through).
			return uintptr(win.HTCLIENT)

		case win.WM_ERASEBKGND:
			// We paint the full surface in WM_PAINT.
			return 1

		case win.WM_PAINT:
			var ps win.PAINTSTRUCT
			hdc := win.BeginPaint(h, &ps)
			if hdc == 0 {
				return 0
			}

			// Fill background black.
			blackBrush := win.HBRUSH(win.GetStockObject(win.BLACK_BRUSH))
			bgRect := &win.RECT{Left: 0, Top: 0, Right: int32(dispW), Bottom: int32(dispH)}
			procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(bgRect)), uintptr(blackBrush))

			// Draw image scaled to fit using StretchBlt.
			if hBmp != 0 {
				dst := fitRect(
					walk.Size{Width: imgW, Height: imgH},
					walk.Rectangle{X: 0, Y: 0, Width: dispW, Height: dispH},
				)
				hdcMem := win.CreateCompatibleDC(hdc)
				oldObj := win.SelectObject(hdcMem, win.HGDIOBJ(hBmp))
				win.SetStretchBltMode(hdc, win.HALFTONE)
				win.StretchBlt(hdc,
					int32(dst.X), int32(dst.Y), int32(dst.Width), int32(dst.Height),
					hdcMem,
					0, 0, int32(imgW), int32(imgH),
					win.SRCCOPY)
				win.SelectObject(hdcMem, oldObj)
				win.DeleteDC(hdcMem)
			}

			win.EndPaint(h, &ps)
			return 0

		case win.WM_LBUTTONDOWN, win.WM_RBUTTONDOWN, win.WM_MBUTTONDOWN, win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
			// Any click or keypress closes the viewer.
			app.closeImageViewer()
			_ = app.preview.Invalidate()
			return 0

		case win.WM_DESTROY:
			if hBmp != 0 {
				win.DeleteObject(win.HGDIOBJ(hBmp))
			}
			return 0
		}
		return win.CallWindowProc(origStaticProc, h, msg, wParam, lParam)
	})
	app.stateMu.Lock()
	app.imageWndProc = imageWndProc
	app.stateMu.Unlock()
	origStaticProc = win.SetWindowLongPtr(hwnd, win.GWLP_WNDPROC, imageWndProc)

	win.InvalidateRect(hwnd, nil, true)
}

// startCursorTracking polls the Windows cursor position at ~60 Hz and
// triggers a preview repaint whenever the cursor enters, leaves, or moves
// within the currently monitored display.
func (app *viewerApp) startCursorTracking() {
	ticker := time.NewTicker(time.Second / 60)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				var pt win.POINT
				if !win.GetCursorPos(&pt) {
					continue
				}

				lDown := isVirtualKeyDown(win.VK_LBUTTON)
				rDown := isVirtualKeyDown(win.VK_RBUTTON)
				mDown := isVirtualKeyDown(win.VK_MBUTTON)
				app.stateMu.RLock()
				display := app.displays[app.displayIndex]
				app.stateMu.RUnlock()

				b := display.bounds
				onDisplay := int(pt.X) >= b.Min.X && int(pt.X) < b.Max.X &&
					int(pt.Y) >= b.Min.Y && int(pt.Y) < b.Max.Y

				app.stateMu.Lock()
				visibilityChanged := app.cursorVisible != onDisplay
				positionChanged := onDisplay && (app.cursorX != int(pt.X) || app.cursorY != int(pt.Y))
				changed := visibilityChanged || positionChanged
				hasImageThumbnail := app.imageBitmap != nil
				hasImageViewer := app.imageViewerHwnd != 0 && app.imageDisplayIndex >= 0 && app.imageDisplayIndex < len(app.displays)
				clickPressed := (!app.lastLButtonDown && lDown) || (!app.lastRButtonDown && rDown) || (!app.lastMButtonDown && mDown)
				imageDisplayHit := false
				if hasImageViewer {
					ib := app.displays[app.imageDisplayIndex].bounds
					imageDisplayHit = int(pt.X) >= ib.Min.X && int(pt.X) < ib.Max.X && int(pt.Y) >= ib.Min.Y && int(pt.Y) < ib.Max.Y
				}
				app.cursorVisible = onDisplay
				if onDisplay {
					app.cursorX = int(pt.X)
					app.cursorY = int(pt.Y)
				}
				app.lastLButtonDown = lDown
				app.lastRButtonDown = rDown
				app.lastMButtonDown = mDown
				shouldInvalidate := changed && !hasImageThumbnail &&
					(visibilityChanged || now.Sub(app.lastCursorInvalidateAt) >= cursorInvalidateInterval)
				shouldCloseImageViewer := clickPressed && imageDisplayHit && hasImageViewer
				if shouldInvalidate {
					app.lastCursorInvalidateAt = now
				}
				app.stateMu.Unlock()

				if shouldCloseImageViewer {
					app.mainWindow.Synchronize(func() {
						app.closeImageViewer()
						_ = app.preview.Invalidate()
					})
					continue
				}

				if shouldInvalidate {
					app.mainWindow.Synchronize(func() {
						_ = app.preview.Invalidate()
					})
				}
			case <-app.stopCapture:
				return
			}
		}
	}()
}

func (app *viewerApp) closeImageViewer() {
	app.stateMu.Lock()
	hwnd := app.imageViewerHwnd
	app.imageViewerHwnd = 0
	app.imageWndProc = 0
	app.imageDisplayIndex = -1
	bmp := app.imageBitmap
	app.imageBitmap = nil
	app.imageSize = walk.Size{}
	app.imageFileName = ""
	app.closeBtnRect = walk.Rectangle{}
	app.stateMu.Unlock()

	if hwnd != 0 {
		win.ReleaseCapture()
		win.DestroyWindow(hwnd)
	}
	if bmp != nil {
		bmp.Dispose()
	}
	app.captureSoon()
}

func (app *viewerApp) handlePreviewClick(x, y int) {
	app.stateMu.RLock()
	r := app.closeBtnRect
	app.stateMu.RUnlock()

	if r.Width == 0 {
		return
	}
	if x >= r.X && x <= r.X+r.Width && y >= r.Y && y <= r.Y+r.Height {
		app.closeImageViewer()
		_ = app.preview.Invalidate()
		app.setStatusText("Image viewer closed")
	}
}

// createHBitmapFromImage converts an image.Image to a Win32 HBITMAP for direct
// GDI painting. The caller must call DeleteObject when done.
func createHBitmapFromImage(img image.Image) win.HBITMAP {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w == 0 || h == 0 {
		return 0
	}

	var bi win.BITMAPINFOHEADER
	bi.BiSize = uint32(unsafe.Sizeof(bi))
	bi.BiWidth = int32(w)
	bi.BiHeight = -int32(h) // top-down
	bi.BiPlanes = 1
	bi.BiBitCount = 32
	bi.BiCompression = win.BI_RGB

	hdc := win.GetDC(0)
	defer win.ReleaseDC(0, hdc)

	var bits unsafe.Pointer
	hBmp := win.CreateDIBSection(hdc, &bi, win.DIB_RGB_COLORS, &bits, 0, 0)
	if hBmp == 0 {
		return 0
	}

	// Copy pixel data in BGRA format.
	pixelCount := w * h
	slice := (*[1 << 30]byte)(bits)[: pixelCount*4 : pixelCount*4]

	// Convert source image to RGBA first if needed.
	rgba, ok := img.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcOff := rgba.PixOffset(x+rgba.Bounds().Min.X, y+rgba.Bounds().Min.Y)
			dstOff := (y*w + x) * 4
			// RGBA -> BGRA
			slice[dstOff+0] = rgba.Pix[srcOff+2] // B
			slice[dstOff+1] = rgba.Pix[srcOff+1] // G
			slice[dstOff+2] = rgba.Pix[srcOff+0] // R
			slice[dstOff+3] = rgba.Pix[srcOff+3] // A
		}
	}

	return hBmp
}
