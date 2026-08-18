//go:build windows

package termwin

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	envMarker = "LINKEDIN_INBOX_IN_TERM"

	windowWidthPx  = 980
	windowHeightPx = 640
	consoleCols    = 110
	consoleRows    = 36
	bufferRows     = 500

	smCXScreen  = 0
	smCYScreen  = 1
	swpNoZOrder = 0x0004
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procGetConsoleWindow            = kernel32.NewProc("GetConsoleWindow")
	procAllocConsole               = kernel32.NewProc("AllocConsole")
	procSetConsoleTitleW           = kernel32.NewProc("SetConsoleTitleW")
	procSetConsoleScreenBufferSize = kernel32.NewProc("SetConsoleScreenBufferSize")
	procSetConsoleWindowInfo        = kernel32.NewProc("SetConsoleWindowInfo")
	procSetConsoleOutputCP          = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP                = kernel32.NewProc("SetConsoleCP")
	procGetSystemMetrics            = user32.NewProc("GetSystemMetrics")
	procSetWindowPos                = user32.NewProc("SetWindowPos")
)

type smallRect struct {
	Left, Top, Right, Bottom int16
}

// Setup ensures a visible console when double-clicked, then sizes and centers it.
func Setup() {
	if os.Getenv(envMarker) != "1" && !hasConsole() {
		if err := relaunchWithConsole(); err == nil {
			os.Exit(0)
		}
		// Fall back to allocating a console in this process.
		if !allocConsole() {
			return
		}
		rebindStdIO()
	}

	configureConsole()
}

func hasConsole() bool {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd != 0
}

func allocConsole() bool {
	r, _, _ := procAllocConsole.Call()
	return r != 0
}

func relaunchWithConsole() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exe)
	cmd.Env = append(os.Environ(), envMarker+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func rebindStdIO() {
	in, err := openCON("CONIN$")
	if err == nil {
		os.Stdin = in
		_ = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(in.Fd()))
	}
	out, err := openCON("CONOUT$")
	if err == nil {
		os.Stdout = out
		os.Stderr = out
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(out.Fd()))
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(out.Fd()))
	}
}

func openCON(name string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}

func configureConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}

	_, _, _ = procSetConsoleOutputCP.Call(65001)
	_, _, _ = procSetConsoleCP.Call(65001)

	if title, err := windows.UTF16PtrFromString("LinkedIn Inbox Downloader"); err == nil {
		_, _, _ = procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(title)))
	}

	hOut := windows.Handle(0)
	if h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		hOut = h
	}
	if hOut != 0 && hOut != windows.InvalidHandle {
		enableVT(hOut)
		xy := uintptr(uint16(consoleCols)) | uintptr(uint16(bufferRows))<<16
		_, _, _ = procSetConsoleScreenBufferSize.Call(uintptr(hOut), xy)
		rect := smallRect{Left: 0, Top: 0, Right: consoleCols - 1, Bottom: consoleRows - 1}
		_, _, _ = procSetConsoleWindowInfo.Call(uintptr(hOut), 1, uintptr(unsafe.Pointer(&rect)))
	}

	cx, _, _ := procGetSystemMetrics.Call(smCXScreen)
	cy, _, _ := procGetSystemMetrics.Call(smCYScreen)
	x := (int32(cx) - windowWidthPx) / 2
	y := (int32(cy) - windowHeightPx) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	_, _, _ = procSetWindowPos.Call(
		hwnd,
		0,
		uintptr(x),
		uintptr(y),
		uintptr(windowWidthPx),
		uintptr(windowHeightPx),
		swpNoZOrder,
	)
}

func enableVT(hOut windows.Handle) {
	var mode uint32
	if err := windows.GetConsoleMode(hOut, &mode); err != nil {
		return
	}
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	_ = windows.SetConsoleMode(hOut, mode)
}
