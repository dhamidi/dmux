// Package proto defines the wire format between dmux clients and the
// dmux server.
//
// The protocol is a bidirectional stream of length-prefixed frames
// over a single AF_UNIX connection (the same transport on Unix and
// on Windows 10 build 17063+). It replaces tmux's imsg-based
// protocol, which relies on SCM_RIGHTS file-descriptor passing —
// a feature AF_UNIX on Windows does not support, so the wire
// format carries no fds.
//
// # Frame format
//
// Every frame on the wire:
//
//	+--------+--------+--------+----------+---------+
//	| length |  type  | flags  | reserved | payload |
//	|  u32   |  u16   |  u16   |   u32    |   ...   |
//	+--------+--------+--------+----------+---------+
//
//	length    Total bytes of (type + flags + reserved + payload).
//	          Does NOT include the length field itself.
//	          Maximum value MaxFrameSize = 1 MiB; receivers reject larger.
//	type      One of the MsgType constants below.
//	flags     Reserved for compression / continuation framing in a
//	          later milestone. Senders set 0 in M1; receivers ignore
//	          the value and do not reject nonzero flags, so future
//	          milestones can introduce new bits without a protocol
//	          version bump.
//	reserved  Currently zero. Reserved for a sequence number.
//	payload   Type-specific; see "Message types" below.
//
// All multi-byte integers are little-endian. Strings are UTF-8 with
// no NUL terminator; their length is always carried by an explicit
// u32 prefix immediately before the bytes (call this `lenstr`):
//
//	lenstr := u32 length || bytes
//
// String slices use:
//
//	stringslice := u32 count || lenstr * count
//
// Booleans are u8 (0 or 1). Profiles (see termcaps) are u8 enums.
//
// # Message types
//
// Type values are stable; new types add new values rather than
// reusing existing ones.
//
//	0x0001  Identify          C->S  one-shot, must precede Command
//	0x0002  CommandList       C->S  one or more commands; see semantics
//	0x0003  Input             C->S  raw bytes from the user's terminal
//	0x0004  Resize            C->S  client size in cells changed
//	0x0005  CapsUpdate        C->S  late-arriving capability info (after DA2)
//	0x0006  Bye               C->S  client is going away (clean detach intent)
//
//	0x0080  Output            S->C  bytes to write to the user's terminal
//	0x0081  CommandResult     S->C  reply to one CommandList Command frame
//	0x0082  Exit              S->C  server is closing this connection
//	0x0083  Beep              S->C  bell event for the active pane
//
// Bit 0x0080 distinguishes server->client. Implementers can switch
// on the high bit to dispatch quickly.
//
// # Identify (0x0001)
//
//	payload :=
//	    u8     protocol_version              (current value: 1)
//	    u8     termcaps_profile              (Ghostty=1, XTermJSModern=2,
//	                                          XTermJSLegacy=3, WindowsTerminal=4,
//	                                          Unknown=0)
//	    u32    initial_cols
//	    u32    initial_rows
//	    lenstr cwd                            (absolute path)
//	    lenstr ttyname                        (informational; "" on Windows)
//	    lenstr term_env                       ($TERM as observed)
//	    stringslice env                       ("KEY=VALUE" entries; full env
//	                                           less ignored prefixes — see EnvFilter)
//	    u8     features_count
//	    [features_count] u8 feature_id        (extensions; reserved, count=0 in M1)
//
// Identify must be the first frame the client sends. The server
// rejects any other type before Identify with Exit{ProtocolError}.
// See "Frame ordering" below.
//
// # CommandList (0x0002)
//
//	payload :=
//	    u32    command_count                  (1..=64; 0 is invalid)
//	    [command_count] command
//
//	command :=
//	    u32    command_id                     (client-assigned, monotonic;
//	                                          appears in CommandResult)
//	    stringslice argv                      ("new-session", "-d", "-s", "work")
//
// The CommandList carries one or more commands as an ordered group.
// The server runs them in order; if any returns Err, subsequent
// commands in the same CommandList are NOT executed (tmux command
// list semantics). This matters for the bootstrap path:
//
//	CommandList{
//	    {id=1, argv=["new-session"]},
//	    {id=2, argv=["attach-session"]},
//	}
//
// is the default invocation. attach-session only runs if new-session
// succeeded; if new-session fails, the client receives one
// CommandResult for command_id=1 with the error, no result for
// command_id=2, and the connection drops.
//
// The 64-command cap is a sanity check; real CommandLists are 1-3
// entries.
//
// # Input (0x0003)
//
//	payload := bytes (no prefix; length is the frame length minus
//	          header overhead)
//
// Raw bytes from the user's terminal stdin. The server feeds these
// into its per-client termin.Parser (M2-1+) or directly to the
// focused pane's SendBytes (M1).
//
// # Resize (0x0004)
//
//	payload :=
//	    u32 cols
//	    u32 rows
//
// Sent by the client when its terminal window changes size.
//
// # CapsUpdate (0x0005)
//
//	payload :=
//	    u8 termcaps_profile
//	    (additional fields reserved for granular caps in M5)
//
// Late-arriving capability information. Triggers reconstruction of
// the client's per-server termin.Parser and termout.Renderer.
//
// # Bye (0x0006)
//
//	payload := (empty)
//
// Client tells the server "I'm exiting cleanly, do not log this as
// a connection drop." Server responds with Exit{Detached}.
//
// # Output (0x0080)
//
//	payload := bytes (no prefix; length is the frame length minus
//	          header overhead)
//
// Bytes the client should write to its real terminal stdout.
//
// # CommandResult (0x0081)
//
//	payload :=
//	    u32     command_id
//	    u8      status                         (0 = ok, 1 = error,
//	                                            2 = skipped)
//	    lenstr  message                        (formatted error or info text;
//	                                            "" on success)
//
// One CommandResult per CommandList Command. Status=skipped means a
// previous Command in the same CommandList failed.
//
// # Exit (0x0082)
//
//	payload :=
//	    u8     reason                          (see ExitReason)
//	    lenstr message                         (human-readable)
//
//	ExitReason values:
//	    0  Detached         normal detach (user pressed prefix-d)
//	    1  DetachedOther    another client requested -d on attach
//	    2  ServerExit       server is shutting down
//	    3  Killed           server received kill-server
//	    4  ProtocolError    client violated protocol (e.g. pre-Identify command)
//	    5  Lost             connection dropped without Bye (server-side
//	                        synthesis from socket EOF)
//	    6  ExitedShell      user's shell exited and session went empty
//
// # Beep (0x0083)
//
//	payload := (empty)
//
// Notifies the client that the focused pane wants to ring the bell.
// The client may emit '\a', flash the screen, etc., per the user's
// terminal preferences. M1: client emits '\a' literally.
//
// # Frame ordering
//
// Three rules:
//
//  1. The client MUST send Identify before any other frame. The
//     server reads frames into a 16-byte buffer until Identify
//     arrives, then drains. If a non-Identify frame arrives first,
//     the server replies Exit{ProtocolError} and closes.
//
//  2. The client MAY send CommandList frames immediately after
//     Identify; the server buffers up to 10 CommandList frames
//     received before Identify processing completes, then drains
//     them in arrival order. Beyond 10, additional CommandLists
//     trigger Exit{ProtocolError}. (Identify processing is fast in
//     practice — under 1ms — so the buffer rarely fills.)
//
//  3. Within a single CommandList, commands run in order. The
//     server sends one CommandResult per command in order; the
//     client matches by command_id.
//
// Input frames are independent of CommandLists; they may arrive at
// any time after Identify and are dispatched immediately.
//
// # Encoding
//
// Marshalling and unmarshalling are by hand using encoding/binary
// for fixed-width fields and byte-slice append for variable
// payloads. No reflection, no gob, no protobuf — the message set is
// small enough that hand-rolled code is the cheapest option.
//
// Frame is an interface satisfied by every concrete message type
// (Identify, Input, CommandList, ...). Each implements
// encoding.BinaryMarshaler and encoding.BinaryUnmarshaler, plus
// Type() MsgType for dispatch. Callers construct messages with
// plain struct literals and switch on concrete types on receive:
//
//	f := &proto.Input{Data: []byte("ls\n")}
//	payload, err := f.MarshalBinary()
//
//	switch m := f.(type) {
//	case *proto.Identify:
//	    // ...
//	case *proto.Input:
//	    // ...
//	}
//
// NewFrame returns a zero-valued Frame for a MsgType. A decoder
// (see internal/xio) reads the envelope, calls NewFrame to allocate
// the correct concrete type, and then calls UnmarshalBinary on the
// payload bytes:
//
//	f, err := proto.NewFrame(t)
//	if err != nil { return err }
//	if err := f.UnmarshalBinary(payload); err != nil { return err }
//
// EncodeEnvelope and DecodeEnvelope handle the 12-byte frame header
// in terms of raw byte slices; xio uses them to stream frames over
// io.Reader / io.Writer without pulling I/O concerns into proto.
//
// # Scope
//
// This package does no I/O (see internal/xio), opens no connections
// (see internal/socket), and parses no commands (see internal/cmd).
// It defines types, encodes, and decodes.
package proto
