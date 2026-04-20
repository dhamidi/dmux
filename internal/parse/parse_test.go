package parse

import (
	"reflect"
	"testing"
)

// cl is a convenience helper to build a CommandList inline.
func cl(cmds ...Command) CommandList {
	return CommandList(cmds)
}

// cmd builds a Command with no body.
func cmd(name string, args ...string) Command {
	if args == nil {
		args = []string{}
	}
	return Command{Name: name, Args: args}
}

// cmdBody builds a Command with a body block.
func cmdBody(name string, args []string, body CommandList) Command {
	if args == nil {
		args = []string{}
	}
	return Command{Name: name, Args: args, Body: &body}
}

// cmdIfElse builds an if command with body and else block.
func cmdIfElse(name string, args []string, body CommandList, elseBody CommandList) Command {
	if args == nil {
		args = []string{}
	}
	return Command{Name: name, Args: args, Body: &body, Else: &elseBody}
}

// ---- simple commands -------------------------------------------------------

func TestSimpleCommand(t *testing.T) {
	got, err := Parse("new-window")
	assertNoError(t, err)
	want := cl(cmd("new-window"))
	assertEqual(t, want, got)
}

func TestCommandWithArgs(t *testing.T) {
	got, err := Parse("bind-key -T prefix q kill-pane")
	assertNoError(t, err)
	want := cl(cmd("bind-key", "-T", "prefix", "q", "kill-pane"))
	assertEqual(t, want, got)
}

func TestMultipleCommandsNewline(t *testing.T) {
	got, err := Parse("new-window\nkill-pane\n")
	assertNoError(t, err)
	want := cl(cmd("new-window"), cmd("kill-pane"))
	assertEqual(t, want, got)
}

func TestMultipleCommandsSemicolon(t *testing.T) {
	got, err := Parse("new-window; kill-pane")
	assertNoError(t, err)
	want := cl(cmd("new-window"), cmd("kill-pane"))
	assertEqual(t, want, got)
}

func TestEmptyInput(t *testing.T) {
	got, err := Parse("")
	assertNoError(t, err)
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestWhitespaceOnly(t *testing.T) {
	got, err := Parse("   \n\t\n")
	assertNoError(t, err)
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

// ---- quoted strings --------------------------------------------------------

func TestDoubleQuotedArg(t *testing.T) {
	got, err := Parse(`set-option -g status-left "hello world"`)
	assertNoError(t, err)
	want := cl(cmd("set-option", "-g", "status-left", "hello world"))
	assertEqual(t, want, got)
}

func TestSingleQuotedArg(t *testing.T) {
	got, err := Parse("set-option -g status-left 'hello world'")
	assertNoError(t, err)
	want := cl(cmd("set-option", "-g", "status-left", "hello world"))
	assertEqual(t, want, got)
}

func TestDoubleQuotedEscapes(t *testing.T) {
	got, err := Parse(`echo "line1\nline2\ttab\\"`)
	assertNoError(t, err)
	want := cl(cmd("echo", "line1\nline2\ttab\\"))
	assertEqual(t, want, got)
}

func TestDoubleQuotedEscapeQuote(t *testing.T) {
	got, err := Parse(`echo "say \"hi\""`)
	assertNoError(t, err)
	want := cl(cmd("echo", `say "hi"`))
	assertEqual(t, want, got)
}

func TestSingleQuotedNoEscape(t *testing.T) {
	// inside single quotes, backslash is literal
	got, err := Parse(`echo '\n'`)
	assertNoError(t, err)
	want := cl(cmd("echo", `\n`))
	assertEqual(t, want, got)
}

// ---- comments --------------------------------------------------------------

func TestComment(t *testing.T) {
	src := `# this is a comment
new-window`
	got, err := Parse(src)
	assertNoError(t, err)
	want := cl(cmd("new-window"))
	assertEqual(t, want, got)
}

func TestInlineComment(t *testing.T) {
	got, err := Parse("new-window # create window")
	assertNoError(t, err)
	want := cl(cmd("new-window"))
	assertEqual(t, want, got)
}

func TestOnlyComment(t *testing.T) {
	got, err := Parse("# just a comment")
	assertNoError(t, err)
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

// ---- line continuation -----------------------------------------------------

func TestLineContinuation(t *testing.T) {
	src := "bind-key -T prefix \\\n  q kill-pane"
	got, err := Parse(src)
	assertNoError(t, err)
	want := cl(cmd("bind-key", "-T", "prefix", "q", "kill-pane"))
	assertEqual(t, want, got)
}

// ---- command blocks --------------------------------------------------------

func TestCommandBlock(t *testing.T) {
	src := `if-shell "tmux -V" {
  new-window
  split-window
}`
	got, err := Parse(src)
	assertNoError(t, err)
	body := cl(cmd("new-window"), cmd("split-window"))
	want := cl(cmdBody("if-shell", []string{"tmux -V"}, body))
	assertEqual(t, want, got)
}

func TestEmptyBlock(t *testing.T) {
	src := "confirm-before {}"
	got, err := Parse(src)
	assertNoError(t, err)
	body := CommandList(nil)
	want := cl(cmdBody("confirm-before", []string{}, body))
	assertEqual(t, want, got)
}

func TestNestedBlock(t *testing.T) {
	src := `outer {
  inner {
    leaf
  }
}`
	got, err := Parse(src)
	assertNoError(t, err)
	leaf := cl(cmd("leaf"))
	innerBody := cl(cmdBody("inner", []string{}, leaf))
	want := cl(cmdBody("outer", []string{}, innerBody))
	assertEqual(t, want, got)
}

// ---- if/else blocks --------------------------------------------------------

func TestIfElseBlock(t *testing.T) {
	src := `if-shell "test" {
  new-window
} else {
  kill-pane
}`
	got, err := Parse(src)
	assertNoError(t, err)
	body := cl(cmd("new-window"))
	elseBody := cl(cmd("kill-pane"))
	want := cl(cmdIfElse("if-shell", []string{"test"}, body, elseBody))
	assertEqual(t, want, got)
}

func TestIfNoElse(t *testing.T) {
	src := `if-shell "test" { new-window }`
	got, err := Parse(src)
	assertNoError(t, err)
	body := cl(cmd("new-window"))
	want := cl(cmdBody("if-shell", []string{"test"}, body))
	assertEqual(t, want, got)
	if got[0].Else != nil {
		t.Fatalf("expected nil Else, got %v", got[0].Else)
	}
}

// ---- error cases -----------------------------------------------------------

func TestUnterminatedDoubleQuote(t *testing.T) {
	_, err := Parse(`echo "unterminated`)
	if err == nil {
		t.Fatal("expected error for unterminated double-quoted string")
	}
}

func TestUnterminatedSingleQuote(t *testing.T) {
	_, err := Parse("echo 'unterminated")
	if err == nil {
		t.Fatal("expected error for unterminated single-quoted string")
	}
}

func TestUnterminatedBlock(t *testing.T) {
	_, err := Parse("if-shell \"cond\" { new-window")
	if err == nil {
		t.Fatal("expected error for unterminated block")
	}
}

func TestMissingBraceAfterElse(t *testing.T) {
	_, err := Parse(`if-shell "cond" { new-window } else kill-pane`)
	if err == nil {
		t.Fatal("expected error for missing '{' after else")
	}
}

// ---- helpers ---------------------------------------------------------------

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual(t *testing.T, want, got CommandList) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}
