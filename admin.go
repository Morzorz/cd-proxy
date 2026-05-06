package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

func newAdminMux(app *App) http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/config", app.handleGetConfig)
	mux.HandleFunc("PUT /api/config", app.handlePutConfig)
	mux.HandleFunc("GET /api/status", app.handleGetStatus)
	mux.HandleFunc("GET /api/logs", app.handleLogsSSE)
	mux.HandleFunc("GET /api/logs/recent", app.handleGetRecentLogs)
	mux.HandleFunc("POST /api/config/import", app.handleImportConfig)
	mux.HandleFunc("GET /api/config/export", app.handleExportConfig)

	// Static files from embedded FS
	webSub, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(webSub))
	mux.Handle("/", fileServer)

	var handler http.Handler = mux
	handler = adminCORS(handler)
	handler = simpleLogMiddleware(app.logRing, handler)

	return handler
}

func adminCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
