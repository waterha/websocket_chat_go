//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Virtual-desktop capture uses these classic Win32 constants.
const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	srccopy          = 0x00CC0020
	captureBlt       = 0x40000000
	biCompressionRGB = 0
	dibUsageRGB      = 0

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (Windows 10 1607+):
	// a pseudo-handle whose value is (HANDLE)-4.
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

	// Maximum screenshot area accepted before rejecting, ~128 megapixels,
	// guarding against overflow / absurd allocations.
	maxCapturePixels = 128_000_000
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

var (
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")

	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procSetThreadDpiAware  = user32.NewProc("SetThreadDpiAwarenessContext")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procGdiFlush           = gdi32.NewProc("GdiFlush")
)

// CaptureScreenshot captures the whole virtual desktop (all monitors,
// including negative coordinates and layered windows) and returns a PNG
// data URL.
//
// DCs and the thread DPI awareness context are thread-affine, so all Win32
// work happens on one locked OS thread and the previous DPI context is
// restored before returning. Per-monitor-aware-v2 (Windows 10 1607+) is
// requested per thread; the process-wide DPI awareness is never changed at
// runtime.
func (a *App) CaptureScreenshot() (string, error) {
	for _, p := range []*windows.LazyProc{
		procGetSystemMetrics, procGetDC, procReleaseDC,
		procSetThreadDpiAware, procCreateCompatibleDC, procDeleteDC,
		procCreateDIBSection, procSelectObject, procDeleteObject,
		procBitBlt, procGdiFlush,
	} {
		if err := p.Find(); err != nil {
			return "", fmt.Errorf("screenshot: locate %s: %w", p.Name, err)
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// On older Windows the call fails and we capture with whatever
	// virtualization applies; anything newer gets exact physical pixels.
	prevCtx, _, _ := procSetThreadDpiAware.Call(dpiAwarenessContextPerMonitorAwareV2)
	if prevCtx != 0 {
		defer procSetThreadDpiAware.Call(prevCtx)
	}

	getMetric := func(index uintptr) int32 {
		r, _, _ := procGetSystemMetrics.Call(index)
		return int32(r)
	}
	x := getMetric(smXVirtualScreen)
	y := getMetric(smYVirtualScreen)
	w := getMetric(smCXVirtualScreen)
	h := getMetric(smCYVirtualScreen)
	if w <= 0 || h <= 0 {
		return "", fmt.Errorf("screenshot: invalid virtual screen metrics %dx%d", w, h)
	}
	if int64(w)*int64(h) > maxCapturePixels {
		return "", fmt.Errorf("screenshot: virtual desktop %dx%d exceeds %d-pixel limit", w, h, maxCapturePixels)
	}

	screenDC, _, err := procGetDC.Call(0)
	if screenDC == 0 {
		return "", fmt.Errorf("screenshot: GetDC: %w", err)
	}
	releaseScreenDC := func() { procReleaseDC.Call(0, screenDC) }

	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		releaseScreenDC()
		return "", fmt.Errorf("screenshot: CreateCompatibleDC: %w", err)
	}
	// Ordered cleanup on every exit path: bitmap, then memDC, then screenDC.
	var dib uintptr
	defer func() {
		if dib != 0 {
			procDeleteObject.Call(dib)
		}
		procDeleteDC.Call(memDC)
		releaseScreenDC()
	}()

	bmi := bitmapInfo{}
	bmi.Header.Size = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.Width = w
	bmi.Header.Height = -h // top-down DIB: first row is the top of the screen
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32 // BGRA
	bmi.Header.Compression = biCompressionRGB

	var bits unsafe.Pointer
	dib, _, err = procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		dibUsageRGB,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	// A non-zero DIB with nil bits is unusable; the deferred cleanup above
	// releases it in either case.
	if dib == 0 || bits == nil {
		return "", fmt.Errorf("screenshot: CreateDIBSection (dib=%#x, bits!=nil=%v): %w", dib, bits != nil, err)
	}

	prevObj, _, _ := procSelectObject.Call(memDC, dib)
	if prevObj == 0 || prevObj == ^uintptr(0) { // 0 or HGDI_ERROR
		return "", fmt.Errorf("screenshot: SelectObject failed")
	}
	defer procSelectObject.Call(memDC, prevObj)

	// CAPTUREBLT keeps layered (alpha-blended) windows in the result.
	ret, _, err := procBitBlt.Call(
		memDC,
		0, 0,
		uintptr(w), uintptr(h),
		screenDC,
		uintptr(x), uintptr(y),
		srccopy|captureBlt,
	)
	if ret == 0 {
		return "", fmt.Errorf("screenshot: BitBlt: %w", err)
	}
	// Per the CreateDIBSection remarks, GdiFlush must be called before
	// reading the bitmap bits written by earlier GDI calls such as BitBlt.
	if r, _, _ := procGdiFlush.Call(); r == 0 {
		return "", fmt.Errorf("screenshot: GdiFlush failed")
	}

	// Convert the BGRA buffer to RGBA in place. The alpha channel is set
	// to fully opaque because screen DCs do not populate it.
	pixels := unsafe.Slice((*uint8)(bits), uintptr(w)*uintptr(h)*4)
	for i := 0; i < len(pixels); i += 4 {
		b, g, r := pixels[i], pixels[i+1], pixels[i+2]
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = r, g, b, 0xFF
	}

	img := image.RGBA{
		Pix:    pixels,
		Stride: int(w) * 4,
		Rect:   image.Rect(0, 0, int(w), int(h)),
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, &img); err != nil {
		return "", fmt.Errorf("screenshot: encode PNG: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
