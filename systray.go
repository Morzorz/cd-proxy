//go:build darwin

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/getlantern/systray"
)

func runSystray(app *App) {
	slog.Info("starting systray")
	systray.Run(onReady(app), onExit(app))
}

func onReady(app *App) func() {
	return func() {
		systray.SetTemplateIcon(runningIcon, runningIcon)
		systray.SetTitle("cd-proxy")
		systray.SetTooltip("cd-proxy - Anthropic API Proxy")

		// Proxy status
		mStatus := systray.AddMenuItem("Proxy: Starting...", "")
		mStatus.Disable()
		mUptime := systray.AddMenuItem("Uptime: --", "")
		mUptime.Disable()
		mReqCount := systray.AddMenuItem("Requests: 0", "")
		mReqCount.Disable()
		systray.AddSeparator()

		// Actions
		mDashboard := systray.AddMenuItem("Open Dashboard", "Open web management UI")
		go func() {
			for range mDashboard.ClickedCh {
				openBrowser(app.adminAddr)
			}
		}()

		systray.AddSeparator()

		mToggle := systray.AddMenuItem("Stop Proxy", "Start/stop the proxy server")
		go func() {
			for range mToggle.ClickedCh {
				if app.IsRunning() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					if err := app.StopProxy(ctx); err != nil {
						slog.Error("stop proxy error", "error", err)
					}
					cancel()
					mToggle.SetTitle("Start Proxy")
					systray.SetTemplateIcon(stoppedIcon, stoppedIcon)
				} else {
					if err := app.StartProxy(); err != nil {
						slog.Error("start proxy error", "error", err)
					}
					mToggle.SetTitle("Stop Proxy")
					systray.SetTemplateIcon(runningIcon, runningIcon)
				}
			}
		}()

		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Quit", "Quit cd-proxy")
		go func() {
			<-mQuit.ClickedCh
			slog.Info("quit from menu")
			systray.Quit()
		}()

		// Background status refresher
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				status := app.Status()
				if status.Running {
					mStatus.SetTitle(fmt.Sprintf("Proxy: Running (port %s)", app.proxyAddr))
				} else {
					mStatus.SetTitle("Proxy: Stopped")
				}
				mUptime.SetTitle(fmt.Sprintf("Uptime: %s", formatDuration(status.UptimeSeconds)))
				mReqCount.SetTitle(fmt.Sprintf("Requests: %d", status.RequestCount))
			}
		}()
	}
}

func onExit(app *App) func() {
	return func() {
		slog.Info("systray exiting, shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		app.Shutdown(ctx)
	}
}

func openBrowser(addr string) {
	url := "http://" + addr
	if strings.HasPrefix(addr, ":") {
		url = "http://localhost" + addr
	}
	exec.Command("open", url).Start()
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	return fmt.Sprintf("%dh %dm", h, m)
}
