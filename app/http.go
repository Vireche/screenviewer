package screenviewer

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net"
	"net/http"
)

const httpPort = 8765

// enableCORS sets CORS headers for cross-origin requests
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (app *viewerApp) startHTTPServer() {
	go func() {
		mux := http.NewServeMux()

		// Ping endpoint for health check
		mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			enableCORS(w)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})

		// Upload endpoint for images from Chrome extension
		mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
			enableCORS(w)

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// Parse multipart form
			err := r.ParseMultipartForm(32 << 20) // 32 MB max
			if err != nil {
				log.Printf("Failed to parse form: %v", err)
				http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
				return
			}

			// Get the image file
			file, handler, err := r.FormFile("image")
			if err != nil {
				log.Printf("No image file: %v", err)
				http.Error(w, "No image file provided: "+err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()

			// Read image data into a buffer
			imageBuffer := make([]byte, 0, 32<<20)
			buf := make([]byte, 4096)
			for {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					log.Printf("Failed to read image: %v", err)
					http.Error(w, "Failed to read image data: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if n == 0 {
					break
				}
				imageBuffer = append(imageBuffer, buf[:n]...)
			}

			// Decode image from buffer
			img, format, err := image.Decode(bytes.NewReader(imageBuffer))
			if err != nil {
				log.Printf("Failed to decode image: %v", err)
				http.Error(w, "Failed to decode image: "+err.Error(), http.StatusBadRequest)
				return
			}

			// Get original dimensions
			bounds := img.Bounds()
			origW := bounds.Dx()
			origH := bounds.Dy()

			// Construct filename with proper extension
			fileName := handler.Filename
			if fileName == "" {
				ext := ".jpg"
				if format != "" {
					ext = "." + format
				}
				fileName = "image_" + fmt.Sprintf("%d", len(app.imageSlots)+1) + ext
			}

			log.Printf("HTTP: Received image from Chrome extension: %s (%dx%d, format: %s)", fileName, origW, origH, format)

			// Marshal addImage onto the UI goroutine — Win32 window creation
			// (in showImagesOnDisplay) must happen on the thread that owns the
			// message loop, otherwise the fullscreen window paints black.
			imgCopy := img
			app.mainWindow.Synchronize(func() {
				app.addImage(imgCopy, origW, origH, fileName)
			})

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","message":"Image received"}`))

			log.Printf("HTTP: Successfully added image to ScreenViewer")
		})

		// Start server
		addr := fmt.Sprintf(":%d", httpPort)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("HTTP server failed to bind to %s: %v", addr, err)
			return
		}

		log.Printf("HTTP server listening on http://localhost:%d", httpPort)

		if err := http.Serve(listener, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}
