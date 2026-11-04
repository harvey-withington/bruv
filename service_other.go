//go:build !windows

package main

// updateInstalledService is Windows-only (a service's Log on as account);
// elsewhere every install is a fresh one.
func updateInstalledService(args []string) (account string, existed bool, err error) {
	return "", false, nil
}
