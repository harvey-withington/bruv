package http

// Git smart-HTTP for published workspaces.
//
//	GET  /repos/<repoID>/workspaces/<wsID>/git/info/refs?service=git-upload-pack
//	POST /repos/<repoID>/workspaces/<wsID>/git/git-upload-pack     (clone/fetch)
//	POST /repos/<repoID>/workspaces/<wsID>/git/git-receive-pack    (push)
//
// This is how a device that can't see the host's disk gets a real working
// copy: it clones over the BRUV connection it already has, authenticated
// with the device token it already holds. No SSH keys, no third-party
// hosting account, and no file bytes travelling through an RPC — git's own
// pack protocol does the transfer, so resume, delta compression and
// incremental fetch all come for free.
//
// The implementation shells out to the host's git rather than embedding a
// git library: `upload-pack`/`receive-pack` in --stateless-rpc mode ARE the
// server side of this protocol, and the host already needs a git binary to
// have published the workspace at all.

import (
	"compress/gzip"
	"fmt"
	"io"
	nethttp "net/http"
	"os/exec"
	"strings"
)

// gitServices are the only sub-commands reachable over the transport.
// Anything else 404s — this is an allow-list, not a filter.
var gitServices = map[string]bool{
	"git-upload-pack":  true,
	"git-receive-pack": true,
}

// gitHandler serves one published workspace. resolve maps a workspace ID to
// the repository directory on this host, returning ok=false for workspaces
// that are unknown, not published, or not yet Ready — an unpublished
// workspace is simply not on the network.
//
// sub is the path after ".../git/", e.g. "info/refs" or "git-upload-pack".
func gitHandler(dir, sub string) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch {
		case sub == "info/refs" && r.Method == nethttp.MethodGet:
			service := r.URL.Query().Get("service")
			if !gitServices[service] {
				// Dumb-protocol clients would fall back to fetching loose
				// objects over plain GETs. BRUV doesn't serve those, so say
				// so rather than letting git fail obscurely on the objects.
				nethttp.Error(w, "only the git smart protocol is served here", nethttp.StatusForbidden)
				return
			}
			advertiseRefs(w, r, dir, service)
		case gitServices[sub] && r.Method == nethttp.MethodPost:
			servicePack(w, r, dir, sub)
		default:
			nethttp.NotFound(w, r)
		}
	})
}

// advertiseRefs answers the discovery request that opens every clone/fetch/
// push: the service banner, then git's own ref advertisement.
func advertiseRefs(w nethttp.ResponseWriter, r *nethttp.Request, dir, service string) {
	cmd := gitPackCommand(r, dir, service, "--advertise-refs")
	out, err := cmd.Output()
	if err != nil {
		gitFail(w, service, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
	noCache(w)
	// The pkt-line banner is smart-HTTP's own framing, not git's — the
	// child process doesn't emit it, so we write it here.
	fmt.Fprint(w, pktLine("# service="+service+"\n"))
	fmt.Fprint(w, "0000")
	w.Write(out)
}

// servicePack pipes the request body through upload-pack/receive-pack and
// streams the result back. The response is streamed and flushed rather than
// buffered: a clone of a large workspace produces its pack incrementally,
// and git shows the user progress as it arrives.
func servicePack(w nethttp.ResponseWriter, r *nethttp.Request, dir, service string) {
	body := io.Reader(r.Body)
	// git compresses larger requests; the header is authoritative.
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			nethttp.Error(w, "invalid gzip body", nethttp.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}

	cmd := gitPackCommand(r, dir, service)
	cmd.Stdin = body
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		gitFail(w, service, err)
		return
	}
	if err := cmd.Start(); err != nil {
		gitFail(w, service, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-"+service+"-result")
	noCache(w)
	// Headers are committed the moment the first bytes go out, so any
	// failure past this point can only be reported by closing the stream —
	// git reports that to the user as a broken connection, which is the
	// best available outcome and the same thing git's own http-backend does.
	io.Copy(flushing(w), stdout)
	cmd.Wait()
}

// gitPackCommand builds the child process for one pack service, forwarding
// the negotiated protocol version so protocol v2 clients (git 2.26+, where
// v2 is the default) get v2 rather than silently falling back.
func gitPackCommand(r *nethttp.Request, dir, service string, extra ...string) *exec.Cmd {
	args := append([]string{strings.TrimPrefix(service, "git-"), "--stateless-rpc"}, extra...)
	args = append(args, dir)
	cmd := exec.CommandContext(r.Context(), "git", args...)
	cmd.Dir = dir
	if v := r.Header.Get("Git-Protocol"); v != "" {
		cmd.Env = append(cmd.Environ(), "GIT_PROTOCOL="+v)
	}
	return cmd
}

// gitFail reports a pre-stream failure. Errors here mean the host's git
// couldn't serve the repository at all (missing binary, unreadable folder),
// which is a server-side fault, not a bad request.
func gitFail(w nethttp.ResponseWriter, service string, err error) {
	nethttp.Error(w, fmt.Sprintf("%s failed: %v", service, err), nethttp.StatusInternalServerError)
}

// noCache marks git responses uncacheable — every one of them is a snapshot
// of mutable state, and a cached ref advertisement breaks fetches.
func noCache(w nethttp.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

// pktLine frames a string in git's pkt-line format: a four-digit hex length
// (covering the length prefix itself) followed by the payload.
func pktLine(s string) string {
	return fmt.Sprintf("%04x%s", len(s)+4, s)
}

// flushing pushes each chunk to the client as it's produced instead of
// waiting for net/http's buffer to fill.
func flushing(w nethttp.ResponseWriter) io.Writer {
	f, ok := w.(nethttp.Flusher)
	if !ok {
		return w
	}
	return &flushWriter{w: w, f: f}
}

type flushWriter struct {
	w io.Writer
	f nethttp.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}
