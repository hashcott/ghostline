package shell

import (
	"context"
	"log/slog"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// applyPlatformOptions keeps Ghostline in the tray when its window closes,
// restores DNS before Windows logs off or shuts down, and re-checks the
// engine after sleep.
func applyPlatformOptions(opts *application.Options, orch *app.Orchestrator) {
	opts.Windows = application.WindowsOptions{
		DisableQuitOnLastWindowClosed: true,
		WndProcInterceptor: func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
			switch classify(msg, wParam) {
			case wmEndSession:
				slog.Info("shell: session ending; disconnecting", "wm", msg)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := orch.Disconnect(ctx); err != nil {
					slog.Warn("shell: disconnect at session end failed", "err", err)
				}
				cancel()
				if msg == wmQueryEndSession {
					return 1, true
				}
			case wmResume:
				go orch.OnResume(context.Background())
			}
			return 0, false
		},
	}
}
