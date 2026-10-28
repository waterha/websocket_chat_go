//go:build !windows

package main

import "errors"

// CaptureScreenshot is only implemented on Windows. On other platforms it
// reports that screen capture is unsupported.
func (a *App) CaptureScreenshot() (string, error) {
	return "", errors.New("screenshot: not supported on this platform")
}
