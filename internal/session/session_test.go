package session_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/dhamidi/dmux/internal/session"
)

func TestNewSessionMonotonicID(t *testing.T) {
	r := session.NewRegistry()
	a, err := r.NewSession("a", nil)
	if err != nil {
		t.Fatalf("NewSession(a): %v", err)
	}
	b, err := r.NewSession("b", nil)
	if err != nil {
		t.Fatalf("NewSession(b): %v", err)
	}
	c, err := r.NewSession("c", nil)
	if err != nil {
		t.Fatalf("NewSession(c): %v", err)
	}
	if a.ID() != 1 || b.ID() != 2 || c.ID() != 3 {
		t.Fatalf("ids not monotonic from 1: %d %d %d", a.ID(), b.ID(), c.ID())
	}
}

func TestFindSession(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("a", nil)
	if got := r.FindSession(a.ID()); got != a {
		t.Fatalf("FindSession returned different pointer: %p vs %p", got, a)
	}
	if got := r.FindSession(9999); got != nil {
		t.Fatalf("FindSession on missing id returned non-nil: %v", got)
	}
}

func TestFindSessionByName(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("alpha", nil)
	if got := r.FindSessionByName("alpha"); got != a {
		t.Fatalf("FindSessionByName returned different pointer: %p vs %p", got, a)
	}
	if got := r.FindSessionByName("missing"); got != nil {
		t.Fatalf("FindSessionByName on missing name returned non-nil: %v", got)
	}
}

func TestDuplicateSessionName(t *testing.T) {
	r := session.NewRegistry()
	if _, err := r.NewSession("dup", nil); err != nil {
		t.Fatalf("first NewSession(dup): %v", err)
	}
	_, err := r.NewSession("dup", nil)
	if err == nil {
		t.Fatalf("second NewSession(dup) returned nil error, want ErrDuplicateSession")
	}
	if !errors.Is(err, session.ErrDuplicateSession) {
		t.Fatalf("error does not wrap ErrDuplicateSession: %v", err)
	}
	// Ensure the registry did not grow on the rejected second call.
	if r.Len() != 1 {
		t.Fatalf("Len after duplicate rejected: got %d, want 1", r.Len())
	}
}

func TestSessionsOrderedByID(t *testing.T) {
	r := session.NewRegistry()
	names := []string{"z", "m", "a", "q"}
	for _, n := range names {
		if _, err := r.NewSession(n, nil); err != nil {
			t.Fatalf("NewSession(%q): %v", n, err)
		}
	}
	var ids []session.ID
	for s := range r.Sessions() {
		ids = append(ids, s.ID())
	}
	want := []session.ID{1, 2, 3, 4}
	if !slices.Equal(ids, want) {
		t.Fatalf("Sessions order: got %v, want %v", ids, want)
	}
}

func TestRemoveSession(t *testing.T) {
	r := session.NewRegistry()
	a, _ := r.NewSession("a", nil)
	r.RemoveSession(a.ID())
	if r.Len() != 0 {
		t.Fatalf("Len after remove: got %d, want 0", r.Len())
	}
	if r.FindSession(a.ID()) != nil {
		t.Fatalf("FindSession still returns removed session")
	}
	if r.FindSessionByName("a") != nil {
		t.Fatalf("FindSessionByName still returns removed session")
	}
	// RemoveSession on an unknown id is a no-op.
	r.RemoveSession(9999)
}

func TestSessionAppendWindowFirst(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	if s.CurrentWindow() != nil {
		t.Fatalf("fresh session has non-nil CurrentWindow")
	}
	w, err := s.AppendWindow("bash")
	if err != nil {
		t.Fatalf("AppendWindow: %v", err)
	}
	if w.Index() != 0 {
		t.Fatalf("window index: got %d, want 0", w.Index())
	}
	if w.Name() != "bash" {
		t.Fatalf("window name: got %q, want %q", w.Name(), "bash")
	}
	if s.CurrentWindow() != w {
		t.Fatalf("CurrentWindow did not return appended window")
	}
}

func TestSessionAppendWindowMultiple(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	w0, _ := s.AppendWindow("bash")
	w1, err := s.AppendWindow("vim")
	if err != nil {
		t.Fatalf("second AppendWindow: %v", err)
	}
	if w1.Index() != 1 {
		t.Fatalf("second window index: got %d, want 1", w1.Index())
	}
	// Newly appended window becomes the current one.
	if s.CurrentWindow() != w1 {
		t.Fatalf("CurrentWindow after second AppendWindow: got %v, want w1", s.CurrentWindow())
	}
	// Windows snapshot returns every window in creation order.
	got := s.Windows()
	if len(got) != 2 || got[0] != w0 || got[1] != w1 {
		t.Fatalf("Windows snapshot: got %v, want [w0 w1]", got)
	}
	// Windows returns a copy — mutating it does not alter internal state.
	got[0] = nil
	if s.CurrentWindow() != w1 {
		t.Fatalf("Windows snapshot mutation leaked into session")
	}
}

func TestSessionNextWindowWraps(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	// With no windows, NextWindow returns nil.
	if got := s.NextWindow(); got != nil {
		t.Fatalf("NextWindow on empty session: got %v, want nil", got)
	}
	w0, _ := s.AppendWindow("a")
	// Single window: NextWindow is a no-op and returns that window.
	if got := s.NextWindow(); got != w0 {
		t.Fatalf("NextWindow on single-window session: got %v, want w0", got)
	}
	if s.CurrentWindow() != w0 {
		t.Fatalf("CurrentWindow after single-window NextWindow moved")
	}
	w1, _ := s.AppendWindow("b")
	w2, _ := s.AppendWindow("c")
	// After AppendWindow, current is w2 (last appended).
	if s.CurrentWindow() != w2 {
		t.Fatalf("CurrentWindow after three appends: got %v, want w2", s.CurrentWindow())
	}
	// NextWindow wraps from index 2 back to index 0.
	if got := s.NextWindow(); got != w0 {
		t.Fatalf("NextWindow wrap: got %v, want w0", got)
	}
	if got := s.NextWindow(); got != w1 {
		t.Fatalf("NextWindow after wrap: got %v, want w1", got)
	}
}

func TestSessionPreviousWindowWraps(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	// With no windows, PreviousWindow returns nil.
	if got := s.PreviousWindow(); got != nil {
		t.Fatalf("PreviousWindow on empty session: got %v, want nil", got)
	}
	w0, _ := s.AppendWindow("a")
	if got := s.PreviousWindow(); got != w0 {
		t.Fatalf("PreviousWindow on single-window session: got %v, want w0", got)
	}
	w1, _ := s.AppendWindow("b")
	w2, _ := s.AppendWindow("c")
	// Current is w2. Previous wraps backward through w1, w0, then back to w2.
	if got := s.PreviousWindow(); got != w1 {
		t.Fatalf("PreviousWindow: got %v, want w1", got)
	}
	if got := s.PreviousWindow(); got != w0 {
		t.Fatalf("PreviousWindow: got %v, want w0", got)
	}
	if got := s.PreviousWindow(); got != w2 {
		t.Fatalf("PreviousWindow wrap: got %v, want w2", got)
	}
}

func TestWindowSetActivePane(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	w, _ := s.AppendWindow("bash")
	if w.ActivePane() != nil {
		t.Fatalf("fresh window has non-nil ActivePane")
	}
	// A real *pane.Pane needs a pty and is out of scope for a unit
	// test in the session package; the round-trip with a real pane is
	// exercised by the server's integration path. Here we assert the
	// idempotent-nil contract so accidental mutation in the setter
	// gets caught.
	w.SetActivePane(nil)
	if w.ActivePane() != nil {
		t.Fatalf("SetActivePane(nil) did not result in nil ActivePane")
	}
}

func TestRemoveWindowKeepsSurvivingIndexStable(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	w0, _ := s.AppendWindow("a")
	w1, _ := s.AppendWindow("b")
	w2, _ := s.AppendWindow("c")
	if w0.Index() != 0 || w1.Index() != 1 || w2.Index() != 2 {
		t.Fatalf("indices before remove: %d %d %d", w0.Index(), w1.Index(), w2.Index())
	}
	if !s.RemoveWindow(w1) {
		t.Fatalf("RemoveWindow(w1) returned false")
	}
	if got := s.Windows(); len(got) != 2 || got[0] != w0 || got[1] != w2 {
		t.Fatalf("windows after remove: %v", got)
	}
	if w0.Index() != 0 || w2.Index() != 2 {
		t.Fatalf("indices after remove changed: w0=%d w2=%d", w0.Index(), w2.Index())
	}
	// A fresh append does NOT fill the gap — indices are monotonic.
	w3, _ := s.AppendWindow("d")
	if w3.Index() != 3 {
		t.Fatalf("new window index after remove: got %d, want 3", w3.Index())
	}
}

func TestRemoveWindowAdjustsCurrent(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	w0, _ := s.AppendWindow("a")
	w1, _ := s.AppendWindow("b")
	w2, _ := s.AppendWindow("c")
	// Current is w2 (last appended). Removing w0 (before cursor)
	// shifts the cursor down so it still points at w2.
	if !s.RemoveWindow(w0) {
		t.Fatalf("RemoveWindow(w0) returned false")
	}
	if s.CurrentWindow() != w2 {
		t.Fatalf("current after removing-before: got %v, want w2", s.CurrentWindow())
	}
	// Removing current (w2): cursor wraps to 0 because w2 was at the end.
	if !s.RemoveWindow(w2) {
		t.Fatalf("RemoveWindow(w2) returned false")
	}
	if s.CurrentWindow() != w1 {
		t.Fatalf("current after removing-current-tail: got %v, want w1", s.CurrentWindow())
	}
	// Removing the last window: current becomes nil.
	if !s.RemoveWindow(w1) {
		t.Fatalf("RemoveWindow(w1) returned false")
	}
	if s.CurrentWindow() != nil {
		t.Fatalf("current after removing-last: got %v, want nil", s.CurrentWindow())
	}
}

func TestRemoveWindowMissingReturnsFalse(t *testing.T) {
	r := session.NewRegistry()
	s, _ := r.NewSession("main", nil)
	s.AppendWindow("a")
	var stranger session.Window
	if s.RemoveWindow(&stranger) {
		t.Fatalf("RemoveWindow on unknown window returned true")
	}
}
