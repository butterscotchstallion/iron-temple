package api

import (
	"context"
	"testing"
	"time"
)

// The generated-activity loop's lifecycle, tested without a database.
//
// activityRunner is pure state and a mutex — no pool, no HTTP — so the thing worth
// asserting about it can be asserted directly, and the assertions below are
// deterministic rather than timing-dependent. That matters because the bug these
// cover was the opposite: a stop that only ASKED a loop to wind down looked correct
// on a slow machine and lost the race on a fast one.
//
// The sleeps are inside the stand-in for a loop, never in an assertion. Each one is
// a goroutine being deliberately slow to wind down; what is checked is whether stop
// waited for it, which is a fact about ordering and not about elapsed time.

// fakeLoop stands in for runActivity: it waits to be cancelled, takes its time
// noticing, and releases its slot exactly as the real one does through finish.
// Returns a channel closed once it has actually finished.
func fakeLoop(
	a *activityRunner, ctx context.Context, gen int,
	cancel context.CancelFunc, done chan struct{}, linger time.Duration,
) <-chan struct{} {
	finished := make(chan struct{})
	go func() {
		<-ctx.Done()
		// Mid-tick when the cancel arrived. The real loop is writing sessions here.
		time.Sleep(linger)
		close(finished)
		a.finish(gen, cancel, done)
	}()
	return finished
}

// STOP MEANS STOPPED. deleteActivity already says in its own comment that it stops
// first so a loop cannot write while its accounts are being deleted; before this it
// only cancelled, and whether that was true came down to which goroutine the
// scheduler picked.
func TestActivityStopWaitsForTheLoopToFinish(t *testing.T) {
	var a activityRunner
	ctx, cancel := context.WithCancel(context.Background())
	gen, done := a.begin(cancel, time.Minute, 2)

	finished := fakeLoop(&a, ctx, gen, cancel, done, 50*time.Millisecond)

	a.stop()

	// No sleep and no polling: if stop waited, this is already closed. If it merely
	// cancelled, it returned while the loop was still in its tick and this is not.
	select {
	case <-finished:
	default:
		t.Fatal("stop returned while the loop was still running")
	}
}

// A restart does not wait — it replaces, and blocking an admin's Start on the old
// loop's tick is not what that button means. So a stop afterwards is responsible for
// TWO goroutines: the incumbent and the predecessor still winding down.
//
// This is the case a single done channel on the struct would miss, and the reason
// the live set is a map rather than one field.
func TestActivityStopWaitsForAReplacedLoopToo(t *testing.T) {
	var a activityRunner

	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstGen, firstDone := a.begin(firstCancel, time.Minute, 2)
	// Slow to wind down, and cancelled by the begin below rather than by stop.
	firstFinished := fakeLoop(&a, firstCtx, firstGen, firstCancel, firstDone,
		50*time.Millisecond)

	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondGen, secondDone := a.begin(secondCancel, time.Minute, 3)
	secondFinished := fakeLoop(&a, secondCtx, secondGen, secondCancel, secondDone,
		10*time.Millisecond)

	a.stop()

	for name, ch := range map[string]<-chan struct{}{
		"replaced": firstFinished,
		"current":  secondFinished,
	} {
		select {
		case <-ch:
		default:
			t.Errorf("stop returned while the %s loop was still running", name)
		}
	}
}

// And the orphan is the one that could hang. finish refuses to clear the slot when
// it no longer owns it — a predecessor must not switch off its successor's flag —
// so leaving the live set had to be the one thing it does unconditionally, or a stop
// would wait on a channel nobody was going to close.
func TestActivityStopDoesNotHangOnAnOrphanedLoop(t *testing.T) {
	var a activityRunner

	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstGen, firstDone := a.begin(firstCancel, time.Minute, 2)
	fakeLoop(&a, firstCtx, firstGen, firstCancel, firstDone, 0)

	secondCancel := func() {}
	_, secondDone := a.begin(secondCancel, time.Minute, 3)

	// The successor is registered but its goroutine never runs, so only the orphan
	// can release anything. Released by hand here, standing in for a loop that
	// returned after being replaced.
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.finish(a.gen, secondCancel, secondDone)
	}()

	waited := make(chan struct{})
	go func() {
		a.stop()
		close(waited)
	}()

	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("stop hung waiting for a loop that had already been replaced")
	}
}

// Stopping nothing is a no-op that returns, which both callers rely on: Stop answers
// 204 whether or not a loop was running, and teardown stops unconditionally before
// it deletes.
func TestActivityStopWithNoLoopReturns(t *testing.T) {
	var a activityRunner

	done := make(chan struct{})
	go func() {
		a.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop blocked with no loop running")
	}
}

// The flag is what the admin screen reads, and it has to be off the moment stop
// returns rather than whenever the goroutine gets round to it — otherwise the screen
// offers Stop again for a loop that has already gone.
func TestActivityStopClearsTheRunningFlag(t *testing.T) {
	var a activityRunner
	ctx, cancel := context.WithCancel(context.Background())
	gen, done := a.begin(cancel, time.Minute, 2)
	fakeLoop(&a, ctx, gen, cancel, done, 0)

	if running, _, _, _, _, _ := a.snapshot(); !running {
		t.Fatal("not running after begin")
	}

	a.stop()

	if running, _, _, _, _, _ := a.snapshot(); running {
		t.Error("still reported as running after stop returned")
	}
}
