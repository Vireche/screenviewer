# Screen Viewer

Screen Viewer is a Windows desktop tool (Go + Walk) that mirrors one monitor inside a resizable control window and can also throw still images fullscreen onto the selected display.

## Requirements

- Windows with at least two active displays
- Go 1.24+

## Run

```powershell
go run .
```

## Build

```powershell
build.bat
```

## What It Does

- Captures the selected display continuously and shows an aspect-correct live preview.
- Starts on Display 2 when available (otherwise Display 1).
- Keeps the preview window locked to the selected display's aspect ratio while resizing.
- Supports Always-on-top mode for the control window.
- Supports dropping image files onto the control window to show them fullscreen on the selected display.
- Supports pasting an image from the clipboard (`Ctrl+V`) to show it fullscreen on the selected display.
- Includes a toggleable Image Browser panel for selecting and launching images from a folder.
- Shows a thumbnail overlay in the preview when a fullscreen image is active, with a clickable close button.
- Supports "Drag window to display" mode that moves and maximizes another app window onto the selected display when you drop it over the viewer.

## Menus And Controls

### File

- `File > Exit`: Close the app.

### Edit

- `Edit > Paste` (`Ctrl+V`): Paste an image from the clipboard and display it fullscreen on the selected monitor.

### View

- `Always on top`: Keep the control window above other windows.
- `Drag window to display`: Enable drag-and-drop window relocation to the selected display.
- `Image browser`: Show/hide the left browser panel.
- `Display N (...)`: Switch the monitored/target display.

## Ways To Show An Image Fullscreen

You can place an image on the selected display in three ways:

1. Drag an image file onto the main Screen Viewer window.
2. Open `View > Image browser`, choose a folder, then single-click an image in the list.
3. Copy an image from another app and press `Ctrl+V` (or use `Edit > Paste`).

Supported file extensions for drag/browser are:

- `.png`
- `.jpg`
- `.jpeg`
- `.gif`
- `.bmp`

## Fullscreen Image Behavior

- The fullscreen image is shown in a borderless window on the selected display and is raised above regular windows when opened.
- The image is scaled to fit while preserving aspect ratio and letterboxing as needed.
- Click on the fullscreen image monitor (left/right/middle) to close it, or press any key while the fullscreen window has focus.
- While active, the main preview shows a bottom-right thumbnail card with a red `X` close button.

## Image Browser Panel

- Hidden by default; toggle with `View > Image browser`.
- "Browse Folder..." opens a folder picker.
- Lists detected image files in that folder.
- Single-clicking an item immediately opens it fullscreen on the selected display.
- The panel is kept narrow and the preview area gets most of the horizontal space.

## Window Drag-To-Display Mode

When `View > Drag window to display` is enabled:

- Drag any other app window so the cursor ends over Screen Viewer, then release.
- Screen Viewer detects the move-end event and relocates that window to the selected display.
- The dropped window is then maximized on that display.

## Performance Notes

- Large source images are downsampled to the target display size before fullscreen presentation for faster loading and smoother interaction.
- Frame presentation is throttled and duplicate-frame detection is used to reduce unnecessary redraw work.

## Native Resource Notes

The Windows UI behavior depends on embedded resources in `resources/windows/screenviewer.manifest` and `resources/windows/rsrc.syso`.
`build.bat` stages `rsrc.syso` into the module root for linking, builds `screenviewer.exe`, then removes the staged copy.