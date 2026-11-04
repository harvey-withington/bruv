//go:build windows

package uncliengine

import "golang.org/x/sys/windows"

// serviceAccounts are Windows' own service accounts, by SID: the CLIs
// would run tools as one of them, and LocalSystem can do anything on the
// machine. The same list UNCLI refuses.
var serviceAccounts = map[string]string{
	"S-1-5-18": "LocalSystem",
	"S-1-5-19": "LocalService",
	"S-1-5-20": "NetworkService",
}

// serviceAccount names the service account this process runs as, or ""
// for a person's account.
func serviceAccount() string {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ""
	}
	return serviceAccounts[user.User.Sid.String()]
}
