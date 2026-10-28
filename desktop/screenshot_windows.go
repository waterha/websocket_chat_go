//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows GDI constants used for the virtual-desktop capture.
const (
	smXVirtualScreen = 76
	smYVirtualScreen = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	srccopy        = 0x00CC0020
	dibRGBMode     = 0
	biCompressionRGB = 0
	dibUsageRGB    = 0

	gdiPenDC = 1 // DCX cache flag not needed; kept minimal
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
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetDC          = user32.NewProc("GetDC")
	procReleaseDC      = user32.NewProc("ReleaseDC")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC       = gdi32.NewProc("DeleteDC")
	procCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	procSelectObject   = gdi32.NewProc("SelectObject")
	procDeleteObject   = gdi32.NewProc("DeleteObject")
	procBitBlt         = gdi32.NewProc("BitBlt")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
)

func mustProc(p *windows.LazyProc, name string) error {
	if err := p.Find(); err != nil {
		return fmt.Errorf("screenshot: locate %s: %w", name, err)
	}
	return nil
}

// CaptureScreenshot captures the whole virtual desktop (all monitors,
// including negative coordinates) and returns a PNG data URL.
// It is DPI aware: the process DPI awareness is raised once so that
// GetSystemMetrics returns true physical pixels instead of scaled values.
func (a *App) CaptureScreenshot() (string, error) {
	procs := []struct {
		p    *windows.LazyProc
		name string
	}{
		{procGetSystemMetrics, "GetSystemMetrics"},
		{procGetDC, "GetDC"},
		{procReleaseDC, "ReleaseDC"},
		{procCreateCompatibleDC, "CreateCompatibleDC"},
		{procDeleteDC, "DeleteDC"},
		{procCreateDIBSection, "CreateDIBSection"},
		{procSelectObject, "SelectObject"},
		{procDeleteObject, "DeleteObject"},
		{procBitBlt, "BitBlt"},
		{procSetProcessDPIAware, "SetProcessDPIAware"},
	}
	for _, pr := range procs {
		if err := mustProc(pr.p, pr.name); err != nil {
			return "", err
		}
	}

	// Request per-monitor DPI awareness so metrics and BitBlt cover the
	// full physical resolution of every monitor. Failure is non-fatal:
	// we then simply capture at whatever virtualization Windows applies.
	_, _, _ = procSetProcessDPIAware.Call()

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

	screenDC, _, err := procGetDC.Call(0)
	if screenDC == 0 {
		return "", fmt.Errorf("screenshot: GetDC: %w", err)
	}
	releaseScreen := func() { procReleaseDC.Call(0, screenDC) }

	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		releaseScreen()
		return "", fmt.Errorf("screenshot: CreateCompatibleDC: %w", err)
	}
	defer procDeleteDC.Call(memDC)

	bmi := bitmapInfo{}
	bmi.Header.Size = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.Width = w
	bmi.Header.Height = -h // top-down DIB: first row is the top of the screen
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32 // BGRA
	bmi.Header.Compression = biCompressionRGB

	var bits unsafe.Pointer
	dib, _, err := procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		dibUsageRGB,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if dib == 0 || bits == nil {
		releaseScreen()
		return "", fmt.Errorf("screenshot: CreateDIBSection: %w", err)
	}
	// From here on, release all GDI resources on every exit path.
	success := false
	defer func() {
		procDeleteObject.Call(dib)
		releaseScreen()
		_ = success
	}()

	prevObj, _, _ := procSelectObject.Call(memDC, dib)
	if prevObj == 0 {
		return "", fmt.Errorf("screenshot: SelectObject failed")
	}
	defer procSelectObject.Call(memDC, prevObj)

	// BitBlt the whole virtual desktop, including monitors with negative
	// coordinates, into our top-down DIB.
	ret, _, err := procBitBlt.Call(
		memDC,
		0, 0,
		uintptr(w), uintptr(h),
		screenDC,
		uintptr(x), uintptr(y),
		srccopy,
	)
	if ret == 0 {
		return "", fmt.Errorf("screenshot: BitBlt: %w", err)
	}

	// Convert the BGRA buffer to RGBA in place. The alpha channel is set
	// to fully opaque because screen DCs do not populate it.
	pixels := unsafe.Slice((*uint8)(bits), uintptr(w)*uintptr(h)*4)
	for i := 0; i < len(pixels); i += 4 {
		b := pixels[i+0]
		g := pixels[i+1]
		r := pixels[i+2]
		pixels[i+0] = r
		pixels[i+1] = g
		pixels[i+2] = b
		pixels[i+3] = 0xFF
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
	success = true
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
