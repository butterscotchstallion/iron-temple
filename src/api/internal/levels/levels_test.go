package levels

import "testing"

// The thresholds the curve was chosen for. If one of these moves, the badge means
// something different from what was agreed, and that is a decision rather than a
// refactor.
func TestPublishedThresholds(t *testing.T) {
	for _, tc := range []struct {
		sessions int
		level    int
	}{
		{0, 1},
		{1, 2},
		{10, 5},
		{66, 12},
		{190, 20},
		{435, 30},
	} {
		if got := FromSessions(tc.sessions).Level; got != tc.level {
			t.Errorf("%d sessions = level %d, want %d", tc.sessions, got, tc.level)
		}
	}
}

// BOTH SIDES OF EVERY BOUNDARY, which is the whole point of this file. A climb that
// is off by one still passes a test that only ever asks about the session that
// reaches a level; it takes the session before to catch it.
func TestBoundariesAreExact(t *testing.T) {
	for _, tc := range []struct {
		sessions  int
		level     int
		intoLevel int
	}{
		{0, 1, 0},   // nothing earned yet
		{1, 2, 0},   // the session that reaches level 2
		{2, 2, 100}, // one into it
		{9, 4, 300}, // the last session of level 4
		{10, 5, 0},  // and the one that leaves it
		{65, 11, 1000},
		{66, 12, 0},
		{67, 12, 100},
	} {
		got := FromSessions(tc.sessions)
		if got.Level != tc.level || got.XPIntoLevel != tc.intoLevel {
			t.Errorf("%d sessions = level %d +%d XP, want level %d +%d XP",
				tc.sessions, got.Level, got.XPIntoLevel, tc.level, tc.intoLevel)
		}
	}
}

// A lifter who has never trained wears level 1, not level 0. A zero beside a name
// reads as something having gone wrong rather than as a new account.
func TestUntrainedLifterIsLevelOne(t *testing.T) {
	got := FromSessions(0)
	if got.Level != 1 {
		t.Fatalf("level = %d, want 1", got.Level)
	}
	if got.XP != 0 || got.XPIntoLevel != 0 {
		t.Fatalf("XP = %d, into = %d, want 0 and 0", got.XP, got.XPIntoLevel)
	}
	if got.XPForNextLevel != XPPerSession {
		t.Fatalf("next level costs %d, want %d — level 2 is one session",
			got.XPForNextLevel, XPPerSession)
	}
}

// XPForNextLevel is the SIZE of the current level, not what is left to pay. The
// header draws XPIntoLevel over it as a bar, so getting this backwards would fill
// the bar as a lifter earned nothing.
func TestNextLevelIsTheWholeCostNotTheRemainder(t *testing.T) {
	// Level 12 costs 1,200 XP in total; 66 sessions is standing at its floor.
	got := FromSessions(66 + 3)
	if got.Level != 12 {
		t.Fatalf("level = %d, want 12", got.Level)
	}
	if got.XPIntoLevel != 300 {
		t.Fatalf("into level = %d, want 300", got.XPIntoLevel)
	}
	if got.XPForNextLevel != 1200 {
		t.Fatalf("next level costs %d, want 1200", got.XPForNextLevel)
	}
}

// The invariant that makes the bar drawable at all: you are never further into a
// level than the level is long. An off-by-one in the climb breaks this before it
// breaks anything a reader would notice.
func TestProgressNeverOverflowsItsLevel(t *testing.T) {
	for n := 0; n < 5000; n++ {
		got := FromSessions(n)
		if got.XPIntoLevel < 0 || got.XPIntoLevel >= got.XPForNextLevel {
			t.Fatalf("%d sessions: %d into a level of %d",
				n, got.XPIntoLevel, got.XPForNextLevel)
		}
		if got.XP != n*XPPerSession {
			t.Fatalf("%d sessions: XP = %d, want %d", n, got.XP, n*XPPerSession)
		}
	}
}

// Levels only ever go up, and only ever by one at a time. A session cannot skip a
// level however long the lifter has been away.
func TestLevelsClimbOneAtATime(t *testing.T) {
	prev := FromSessions(0).Level
	for n := 1; n < 5000; n++ {
		got := FromSessions(n).Level
		if got != prev && got != prev+1 {
			t.Fatalf("%d sessions jumped from level %d to %d", n, prev, got)
		}
		prev = got
	}
}

// A count below zero cannot come from a COUNT(*), but the zero value of an int can
// reach here through a struct, and clamping means no caller has to think about it.
func TestNegativeCountIsTreatedAsUntrained(t *testing.T) {
	if got := FromSessions(-5); got.Level != 1 || got.XP != 0 {
		t.Fatalf("got level %d with %d XP, want level 1 with 0", got.Level, got.XP)
	}
}
