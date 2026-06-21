//go:build windows

package browser

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// rundll32 with FileProtocolHandler hands the URL to the user's default browser.
func consoleCommand(url string) (string, []string) {
	return "rundll32.exe", []string{"url.dll,FileProtocolHandler", url}
}

// open dispatches to the console or service strategy.
func open(url string, interactive bool) error {
	if interactive {
		return openConsole(url)
	}
	return openInActiveSession(url)
}

// openConsole opens url in the current (interactive) session.
func openConsole(url string) error {
	name, args := consoleCommand(url)
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

var (
	wtsapi32              = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQueryUserToken = wtsapi32.NewProc("WTSQueryUserToken")

	userenv                     = windows.NewLazySystemDLL("userenv.dll")
	procCreateEnvironmentBlock  = userenv.NewProc("CreateEnvironmentBlock")
	procDestroyEnvironmentBlock = userenv.NewProc("DestroyEnvironmentBlock")

	advapi32                 = windows.NewLazySystemDLL("advapi32.dll")
	procCreateProcessAsUserW = advapi32.NewProc("CreateProcessAsUserW")
)

const (
	createUnicodeEnvironment = 0x00000400
	normalPriorityClass      = 0x00000020
	invalidSessionID         = 0xFFFFFFFF
)

// openInActiveSession launches the URL handler in the session of the user
// currently logged in at the console. It is meant to be called from a service
// running as LocalSystem, which holds the privileges (SeTcbPrivilege) required
// to obtain another session's token.
func openInActiveSession(url string) error {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == invalidSessionID {
		return fmt.Errorf("no active console session (no user is logged in)")
	}

	// Token of the interactive user.
	var userToken windows.Token
	if r1, _, err := procWTSQueryUserToken.Call(
		uintptr(sessionID),
		uintptr(unsafe.Pointer(&userToken)),
	); r1 == 0 {
		return fmt.Errorf("WTSQueryUserToken: %w", err)
	}
	defer userToken.Close()

	// A primary token is required to start a process with it.
	var primary windows.Token
	if err := windows.DuplicateTokenEx(
		userToken,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityIdentification,
		windows.TokenPrimary,
		&primary,
	); err != nil {
		return fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primary.Close()

	// Build the user's environment so the browser launches with their profile.
	var env *uint16
	if r1, _, err := procCreateEnvironmentBlock.Call(
		uintptr(unsafe.Pointer(&env)),
		uintptr(primary),
		0,
	); r1 == 0 {
		return fmt.Errorf("CreateEnvironmentBlock: %w", err)
	}
	defer procDestroyEnvironmentBlock.Call(uintptr(unsafe.Pointer(env)))

	cmdLine, err := windows.UTF16PtrFromString(
		fmt.Sprintf("rundll32.exe url.dll,FileProtocolHandler %s", url),
	)
	if err != nil {
		return err
	}
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return err
	}

	si := windows.StartupInfo{Desktop: desktop}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation

	if r1, _, err := procCreateProcessAsUserW.Call(
		uintptr(primary),
		0, // lpApplicationName (taken from the command line)
		uintptr(unsafe.Pointer(cmdLine)),
		0, // process security attributes
		0, // thread security attributes
		0, // bInheritHandles = FALSE
		uintptr(createUnicodeEnvironment|normalPriorityClass),
		uintptr(unsafe.Pointer(env)),
		0, // current directory
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	); r1 == 0 {
		return fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}
