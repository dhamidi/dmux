// Package termcaps identifies the user's real terminal and derives a
// feature profile for dmux.
//
// # Profiles (closed set)
//
//	Ghostty           full modern: KKP, kitty graphics, sixel, OSC 8
//	XTermJSModern     xterm.js 6.1+ (VS Code 1.109+): KKP, sixel, OSC 8
//	XTermJSLegacy     older xterm.js: modifyOtherKeys, no kitty graphics
//	WindowsTerminal   recent Windows Terminal: modifyOtherKeys, sixel 1.22+
//	Unknown           fallback: modifyOtherKeys, no graphics
//
// This is a closed set by design. Adding a profile is a deliberate act
// (new enum value, new branch in Features, new branches in termin and
// termout), not an open extension point. dmux is not a general terminal
// library.
//
// # Detection
//
// Order in Detect:
//
//  1. TERM_PROGRAM=ghostty                -> Ghostty
//  2. WT_SESSION set                      -> WindowsTerminal
//  3. DA2 response (CSI > c) plus KKP
//     query (CSI ? u) answered            -> XTermJSModern
//  4. DA2 response without KKP            -> XTermJSLegacy
//  5. Otherwise                           -> Unknown
//
// The probe runs on the client at startup over a short timeout. The
// resulting Profile ships to the server in the Identify frame; the
// server uses it to construct a per-client termin.Parser and
// termout.Renderer.
//
// # Interface
//
//	Detect(probe ProbeIO, env func(string) string) Profile
//	(Profile) Features() Features
//
// Features is a flat struct of booleans (TrueColor, KKP, Sixel,
// KittyGraphics, OSC8, FocusEvents, BracketedPaste). It is the only
// thing higher layers should consult.
//
// # What this replaces
//
// tmux's tty-term.c and tty-features.c, which read terminfo(5) and
// reason about hundreds of capability codes. dmux does not use
// terminfo; the five-profile matrix is the single source of truth.
package termcaps
