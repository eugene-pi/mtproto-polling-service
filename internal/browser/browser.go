// Package browser opens URLs in the user's default browser.
//
// On Windows it can also open a URL from a service (which runs in the isolated
// session 0) into the active interactive user session, so the page actually
// appears on the logged-in user's desktop. See open_windows.go.
package browser

import "errors"

// ErrNoActiveSession is returned (on Windows, from service/session-0 context)
// when no user is logged in at the console, so there is no desktop to open the
// browser on. Callers should treat it as "try again later", not a hard failure.
var ErrNoActiveSession = errors.New("no active user session")

// Open opens url in the default browser.
//
// interactive should be true when the process runs in a normal user session
// (console mode) and false when it runs as a service. On Windows the two cases
// use different mechanisms; elsewhere the flag is ignored.
func Open(url string, interactive bool) error {
	return open(url, interactive)
}
