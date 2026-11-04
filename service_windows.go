//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// updateInstalledService points an existing BRUV-Server registration at
// this binary and these arguments, in place. Uninstalling and recreating
// it would reset its Log on as account to LocalSystem (kardianos sets
// none), undoing the operator's choice on every installer update. It
// returns the account the service keeps; existed is false when there is
// no such service, and the caller installs a fresh one.
func updateInstalledService(args []string) (account string, existed bool, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	m, err := mgr.Connect()
	if err != nil {
		return "", false, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return "", true, err
	}
	account = cfg.ServiceStartName
	cfg.BinaryPathName = serviceCommandLine(exe, args)
	cfg.DisplayName = serviceDisplay
	cfg.Description = serviceDesc
	// Empty is "unchanged" to ChangeServiceConfig: the account and its
	// password stay as the operator set them.
	cfg.ServiceStartName = ""
	cfg.Password = ""
	return account, true, s.UpdateConfig(cfg)
}

// serviceCommandLine quotes the binary and its arguments the way
// mgr.CreateService does for a fresh install.
func serviceCommandLine(exe string, args []string) string {
	line := syscall.EscapeArg(exe)
	for _, a := range args {
		line += " " + syscall.EscapeArg(a)
	}
	return line
}
