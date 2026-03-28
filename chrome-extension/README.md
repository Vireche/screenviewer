# ScreenViewer Chrome Extension

This extension adds a right-click context menu option to send images directly to ScreenViewer.

## Installation

1. Make sure ScreenViewer is running and the HTTP server is enabled (default port: 8765)
2. Open Chrome and go to `chrome://extensions/`
3. Enable "Developer mode" (toggle in top right)
4. Click "Load unpacked"
5. Navigate to this directory (`chrome-extension/`) and select it

The extension will now be installed and active.

## Usage

1. Right-click any image on a web page
2. Select "Send to ScreenViewer" from the context menu
3. The image will be transmitted to ScreenViewer

If successful, the extension icon will show a green checkmark (✓) for 2 seconds.
If it fails, it will show a red X (✗) for 3 seconds. Check the error details in the extension's error log.

## Configuration

To change the port that the extension uses, edit this line in `background.js`:

```javascript
const SCREENVIEWER_PORT = 8765;
```

And make sure ScreenViewer is configured to listen on the same port.

## Troubleshooting

- **"ScreenViewer is not responding"**: Make sure ScreenViewer is running and has HTTP server enabled
- **Port conflict**: If 8765 is already in use, change it in both the extension and ScreenViewer settings
- **CORS errors**: The extension uses `mode: "no-cors"` which should handle this, but if problems persist, check ScreenViewer's HTTP server configuration

## Development

- `manifest.json` - Extension configuration
- `background.js` - Service worker that handles context menu and image upload
