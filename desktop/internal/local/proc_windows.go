//go:build windows

package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// PostgreSQL's Windows initdb generates SQL containing its installation/data
// paths through the active ANSI code page. ASCII short paths keep a UTF8
// cluster safe even when the Windows user/data directory contains Chinese.
func postgresPath(path string) (string, error) {
	ascii := true
	for _, ch := range path {
		if ch > 127 {
			ascii = false
			break
		}
	}
	if ascii {
		return path, nil
	}
	input, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetShortPathNameW")
	n, _, callErr := proc.Call(uintptr(unsafe.Pointer(input)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	_ = callErr
	if n == 0 || n >= uintptr(len(buffer)) {
		return postgresJunction(path)
	}
	result := syscall.UTF16ToString(buffer[:n])
	for _, ch := range result {
		if ch > 127 {
			return postgresJunction(path)
		}
	}
	return result, nil
}

// Junctions need no administrator/developer-mode privilege. The alias base has
// a protected DACL for this user and SYSTEM; existing aliases must resolve to
// the exact requested directory. User data is never moved or copied.
func postgresJunction(path string) (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sid := user.User.Sid.String()
	base := os.Getenv("LOCALAPPDATA")
	isASCII := func(text string) bool {
		for _, ch := range text {
			if ch > 127 {
				return false
			}
		}
		return text != ""
	}
	if !isASCII(base) {
		base = os.Getenv("PUBLIC")
	}
	if !isASCII(base) {
		return "", fmt.Errorf("系统未提供可用的数据库兼容路径")
	}
	sum := sha256.Sum256([]byte(sid))
	base = filepath.Join(base, "WebsocketChat-paths-"+hex.EncodeToString(sum[:8]))
	descriptor, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return "", err
	}
	basePtr, err := windows.UTF16PtrFromString(base)
	if err != nil {
		return "", err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	if err := windows.CreateDirectory(basePtr, &attributes); err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return "", err
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("数据库别名目录不安全：%s", base)
	}
	security, err := windows.GetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return "", err
	}
	owner, _, err := security.Owner()
	if err != nil || !owner.Equals(user.User.Sid) {
		return "", fmt.Errorf("数据库别名目录不属于当前用户")
	}
	actual, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	sum = sha256.Sum256([]byte(strings.ToLower(actual)))
	alias := filepath.Join(base, hex.EncodeToString(sum[:16]))
	if _, err := os.Lstat(alias); os.IsNotExist(err) {
		if strings.ContainsAny(actual, "%\r\n\"") {
			return "", fmt.Errorf("数据库路径包含不支持的控制或环境展开字符")
		}
		command := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
		hideWindow(command)
		// CMD does not use Go's CommandLineToArgvW quote/backslash escaping.
		command.SysProcAttr.CmdLine = fmt.Sprintf(`"%s" /d /v:off /c mklink /J "%s" "%s"`, command.Path, alias, actual)
		if output, err := command.CombinedOutput(); err != nil {
			return "", fmt.Errorf("无法准备数据库目录别名：%w %s", err, output)
		}
	}
	resolved, err := os.Stat(alias)
	if err != nil {
		return "", err
	}
	original, err := os.Stat(actual)
	if err != nil || !os.SameFile(resolved, original) {
		return "", fmt.Errorf("数据库目录别名目标校验失败：%s -> %s", alias, actual)
	}
	return alias, nil
}

// hideWindow keeps service helper processes from flashing a console while the
// launcher runs as a GUI application.
func hideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
