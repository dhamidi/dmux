package command

import (
	"fmt"
	"strconv"
	"strings"
)

// resolveTarget parses raw (the value of the -t flag, possibly empty) and
// resolves it to a Target using store and the client's current context.
func resolveTarget(spec TargetSpec, raw string, store Server, client ClientView) (Target, error) {
	if spec.Kind == TargetNone {
		return Target{}, nil
	}
	if raw == "" {
		// Use the client's current context as the default target.
		return defaultTarget(spec, store, client)
	}
	return parseAndLookupTarget(spec, raw, store, client)
}

// defaultTarget returns a Target based on the client's currently attached
// session/window/pane when no -t flag was given.
func defaultTarget(spec TargetSpec, store Server, client ClientView) (Target, error) {
	if !client.IsAttached() {
		return Target{}, fmt.Errorf("no target: client is not attached to a session")
	}
	sess, ok := store.GetSession(client.SessionID)
	if !ok {
		return Target{}, fmt.Errorf("no target: session %q not found", client.SessionID)
	}
	t := Target{Kind: TargetSession, Session: sess}
	if spec.Kind <= TargetSession {
		return t, nil
	}

	win := sess.CurrentWindow()
	if win.ID == "" {
		return Target{}, fmt.Errorf("no target: session %q has no current window", sess.Name)
	}
	t.Kind = TargetWindow
	t.Window = win
	if spec.Kind <= TargetWindow {
		return t, nil
	}

	pane := win.ActivePane()
	if pane.ID == 0 {
		return Target{}, fmt.Errorf("no target: window %q has no active pane", win.Name)
	}
	t.Kind = TargetPane
	t.Pane = pane
	return t, nil
}

// parseAndLookupTarget parses the -t string and resolves it against store.
//
// Supported target formats:
//
//	session               – session by name or ID
//	$id                   – session by numeric-style ID ($ prefix)
//	session:window        – session + window by name or index
//	session:window.pane   – session + window + pane by ID/index
//	@id                   – window by ID within the client's session
//	%id                   – pane by ID within the client's current window
//	{last}, {next}, {marked} – special markers (resolved to default target)
//	~                     – last session (last in ListSessions)
func parseAndLookupTarget(spec TargetSpec, raw string, store Server, client ClientView) (Target, error) {
	// ~ means "last session" — return the last in ListSessions as best-effort.
	if raw == "~" {
		sessions := store.ListSessions()
		if len(sessions) == 0 {
			return Target{}, fmt.Errorf("no sessions exist")
		}
		return Target{Kind: TargetSession, Session: sessions[len(sessions)-1]}, nil
	}

	// {last}, {next}, {marked} etc. — fall back to default target.
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		return defaultTarget(spec, store, client)
	}

	// %id — pane by ID, resolved in the client's current window.
	if strings.HasPrefix(raw, "%") {
		id, err := strconv.Atoi(raw[1:])
		if err != nil {
			return Target{}, fmt.Errorf("invalid pane target %q", raw)
		}
		base, err := defaultTarget(TargetSpec{Kind: TargetWindow}, store, client)
		if err != nil {
			return Target{}, err
		}
		pane, err := lookupPane(base.Window, raw)
		if err != nil {
			return Target{}, err
		}
		_ = id
		return Target{Kind: TargetPane, Session: base.Session, Window: base.Window, Pane: pane}, nil
	}

	// @id — window by ID, resolved in the client's current session.
	if strings.HasPrefix(raw, "@") {
		base, err := defaultTarget(TargetSpec{Kind: TargetSession}, store, client)
		if err != nil {
			return Target{}, err
		}
		win, err := lookupWindow(base.Session, raw)
		if err != nil {
			return Target{}, err
		}
		t := Target{Kind: TargetWindow, Session: base.Session, Window: win}
		if spec.Kind <= TargetWindow {
			return t, nil
		}
		pane := win.ActivePane()
		if pane.ID == 0 {
			return Target{}, fmt.Errorf("window @%s has no active pane", raw[1:])
		}
		t.Kind = TargetPane
		t.Pane = pane
		return t, nil
	}

	// General format: [session][:window[.pane]]
	sessRef, rest, hasColon := strings.Cut(raw, ":")
	var winRef, paneRef string
	if hasColon && rest != "" {
		winRef, paneRef, _ = strings.Cut(rest, ".")
	}

	// Resolve session.
	var sess SessionView
	var ok bool
	if sessRef == "" {
		// No session part — use client's current session.
		if !client.IsAttached() {
			return Target{}, fmt.Errorf("no target: client is not attached to a session")
		}
		sess, ok = store.GetSession(client.SessionID)
	} else {
		sess, ok = lookupSession(store, sessRef)
	}
	if !ok {
		return Target{}, fmt.Errorf("session %q not found", sessRef)
	}
	t := Target{Kind: TargetSession, Session: sess}
	if spec.Kind <= TargetSession || winRef == "" {
		return t, nil
	}

	// Resolve window.
	win, err := lookupWindow(sess, winRef)
	if err != nil {
		return Target{}, err
	}
	t.Kind = TargetWindow
	t.Window = win
	if spec.Kind <= TargetWindow || paneRef == "" {
		return t, nil
	}

	// Resolve pane.
	pane, err := lookupPane(win, paneRef)
	if err != nil {
		return Target{}, err
	}
	t.Kind = TargetPane
	t.Pane = pane
	return t, nil
}

// lookupSession finds a session by ID (with optional $ prefix) or by name.
func lookupSession(store SessionStore, ref string) (SessionView, bool) {
	id := ref
	if strings.HasPrefix(ref, "$") {
		id = ref[1:]
	}
	// Try exact ID.
	if s, ok := store.GetSession(id); ok {
		return s, true
	}
	// Try by name.
	return store.GetSessionByName(ref)
}

// lookupWindow finds a window within sess by @id, numeric index, or name.
func lookupWindow(sess SessionView, ref string) (WindowView, error) {
	if strings.HasPrefix(ref, "@") {
		id := ref[1:]
		for _, w := range sess.Windows {
			if w.ID == id {
				return w, nil
			}
		}
		return WindowView{}, fmt.Errorf("window %q not found in session %q", ref, sess.Name)
	}
	if idx, err := strconv.Atoi(ref); err == nil {
		for _, w := range sess.Windows {
			if w.Index == idx {
				return w, nil
			}
		}
		return WindowView{}, fmt.Errorf("window index %d not found in session %q", idx, sess.Name)
	}
	for _, w := range sess.Windows {
		if w.Name == ref {
			return w, nil
		}
	}
	return WindowView{}, fmt.Errorf("window %q not found in session %q", ref, sess.Name)
}

// lookupPane finds a pane within win by %id or 0-based positional index.
func lookupPane(win WindowView, ref string) (PaneView, error) {
	if strings.HasPrefix(ref, "%") {
		id, err := strconv.Atoi(ref[1:])
		if err != nil {
			return PaneView{}, fmt.Errorf("invalid pane id %q", ref)
		}
		for _, p := range win.Panes {
			if p.ID == id {
				return p, nil
			}
		}
		return PaneView{}, fmt.Errorf("pane %q not found in window %q", ref, win.Name)
	}
	if idx, err := strconv.Atoi(ref); err == nil {
		if idx >= 0 && idx < len(win.Panes) {
			return win.Panes[idx], nil
		}
		return PaneView{}, fmt.Errorf("pane index %d out of range in window %q", idx, win.Name)
	}
	return PaneView{}, fmt.Errorf("invalid pane reference %q in window %q", ref, win.Name)
}
