package supervisor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"bruv/core/events"
	"bruv/internal/config"
)

// stubRuntime is a Runtime with no repo or workers — enough for the
// supervisor's lifecycle bookkeeping. Its ctx is cancelled by Close.
func stubRuntime() *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{ctx: ctx, cancelCtx: cancel, bus: events.NewMemBus(8)}
}

func isClosed(rt *Runtime) bool { return rt.ctx.Err() != nil }

// gatedBuild swaps buildRuntimeFn for one that blocks until release is
// closed, counting builds. Restored on cleanup.
func gatedBuild(t *testing.T) (release chan struct{}, started chan *Runtime, builds *atomic.Int32) {
	t.Helper()
	release = make(chan struct{})
	started = make(chan *Runtime, 8)
	builds = &atomic.Int32{}
	orig := buildRuntimeFn
	buildRuntimeFn = func(string, string, []byte) (*Runtime, error) {
		builds.Add(1)
		rt := stubRuntime()
		started <- rt
		<-release
		return rt, nil
	}
	t.Cleanup(func() { buildRuntimeFn = orig })
	return release, started, builds
}

func newTestSupervisor(t *testing.T, entries ...config.RepoEntry) *Supervisor {
	t.Helper()
	s, err := New(entries, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadRefusesDisabledRepo(t *testing.T) {
	_, _, builds := gatedBuild(t)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x", Disabled: true})
	if _, err := s.Load("r"); !errors.Is(err, ErrRepoDisabled) {
		t.Fatalf("Load(disabled) err = %v, want ErrRepoDisabled", err)
	}
	if builds.Load() != 0 {
		t.Fatal("disabled repo was built")
	}
}

// TestUnloadDuringBuildDiscardsRuntime: before the fix an Unload during
// an in-flight build was ignored and the build cached its runtime
// anyway — a second runtime on the repo once the next Load rebuilt.
func TestUnloadDuringBuildDiscardsRuntime(t *testing.T) {
	release, started, _ := gatedBuild(t)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x"})

	type result struct {
		rt  *Runtime
		err error
	}
	done := make(chan result, 1)
	go func() {
		rt, err := s.Load("r")
		done <- result{rt, err}
	}()
	built := <-started
	s.Unload("r")
	close(release)

	res := <-done
	if res.err == nil || res.rt != nil {
		t.Fatalf("Load = (%v, %v), want discarded build", res.rt, res.err)
	}
	if s.Resolve("r") != nil {
		t.Fatal("discarded runtime was cached")
	}
	if !isClosed(built) {
		t.Fatal("discarded runtime was not closed")
	}
}

// TestDisableDuringBuildDiscardsRuntime: a build finishing after the
// repo was disabled must not install a runtime.
func TestDisableDuringBuildDiscardsRuntime(t *testing.T) {
	release, started, _ := gatedBuild(t)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x"})

	done := make(chan error, 1)
	go func() {
		_, err := s.Load("r")
		done <- err
	}()
	built := <-started
	// SetEnabled(false) minus the repos.json write.
	s.mu.Lock()
	e := s.entries["r"]
	e.Disabled = true
	s.entries["r"] = e
	s.detachLocked("r")
	s.mu.Unlock()
	close(release)

	if err := <-done; err == nil {
		t.Fatal("Load succeeded for a repo disabled mid-build")
	}
	if s.Resolve("r") != nil || !isClosed(built) {
		t.Fatal("disabled repo's fresh runtime survived")
	}
	if _, err := s.Load("r"); !errors.Is(err, ErrRepoDisabled) {
		t.Fatalf("later Load err = %v, want ErrRepoDisabled", err)
	}
}

// TestLoadAfterCloseFails: a late Load used to write into the nil
// runtimes map while holding s.mu — the panic left the lock held and
// every later supervisor call hung.
func TestLoadAfterCloseFails(t *testing.T) {
	gatedBuild(t)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x"})
	s.Close()
	if _, err := s.Load("r"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Load after Close err = %v, want ErrClosed", err)
	}
	listed := make(chan struct{})
	go func() { s.List(); s.Close(); close(listed) }()
	select {
	case <-listed:
	case <-time.After(time.Second):
		t.Fatal("supervisor lock stuck after Close")
	}
}

// TestCloseDuringBuildClosesFreshRuntime: Close must not return while
// a build is still going to install a runtime, and that runtime must
// end up closed.
func TestCloseDuringBuildClosesFreshRuntime(t *testing.T) {
	release, started, _ := gatedBuild(t)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x"})
	go func() { _, _ = s.Load("r") }()
	built := <-started

	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a build was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the build finished")
	}
	if !isClosed(built) {
		t.Fatal("runtime built during Close was left running")
	}
}

// TestReloadWaitsForPreviousClose: a Load right after Unload must not
// build while the old runtime is still closing.
func TestReloadWaitsForPreviousClose(t *testing.T) {
	release, started, builds := gatedBuild(t)
	close(release)
	s := newTestSupervisor(t, config.RepoEntry{ID: "r", Path: "x"})
	t.Cleanup(s.Close)

	// Pretend a previous runtime for "r" is mid-Close.
	s.mu.Lock()
	s.closing["r"]++
	s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		_, err := s.Load("r")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if builds.Load() != 0 {
		t.Fatal("Load built while the previous runtime was still closing")
	}
	s.finishClose("r", stubRuntime())
	if err := <-done; err != nil {
		t.Fatalf("Load after close finished: %v", err)
	}
	<-started
	if builds.Load() != 1 {
		t.Fatalf("builds = %d, want 1", builds.Load())
	}
}

// TestRuntimeCloseWaitsForBackgroundWrites: activity-log writes used to
// be bare goroutines that could land after Close (a write into a
// closed repo; flaky TempDir cleanup in tests).
func TestRuntimeCloseWaitsForBackgroundWrites(t *testing.T) {
	rt := stubRuntime()
	release := make(chan struct{})
	var wrote atomic.Bool
	rt.goBackground(func() { <-release; wrote.Store(true) })

	closed := make(chan struct{})
	go func() { rt.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned before the background write finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-closed
	if !wrote.Load() {
		t.Fatal("background write lost")
	}

	var late atomic.Bool
	rt.goBackground(func() { late.Store(true) })
	time.Sleep(20 * time.Millisecond)
	if late.Load() {
		t.Fatal("background write started after Close")
	}
}

// TestRuntimeCloseEndsBusSubscriptions: SSE streams subscribed to a
// runtime's bus must end when the runtime closes, so clients reconnect
// to the replacement instead of heartbeating against a dead runtime.
func TestRuntimeCloseEndsBusSubscriptions(t *testing.T) {
	rt := stubRuntime()
	ch, unsub := rt.Bus().Subscribe()
	defer unsub()
	rt.Close()
	rt.Close() // idempotent
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("bus subscription still open after Runtime.Close")
	}
}
