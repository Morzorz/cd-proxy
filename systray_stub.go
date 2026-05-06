//go:build !darwin

package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"
)

func runSystray(app *App) {
	slog.Info("cd-proxy started",
		"proxy", app.proxyAddr,
		"admin", "http://"+app.adminAddr,
	)

	openBrowser(app.adminAddr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	app.Shutdown(shutdownCtx)
	slog.Info("shutdown complete")
}

func openBrowser(addr string) {
	url := "http://" + addr
	if len(addr) > 0 && addr[0] == ':' {
		url = "http://localhost" + addr
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Start()
}
