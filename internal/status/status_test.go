package status

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// sgrRE matches ANSI SGR sequences (CSI ... m). Used to measure the
// visible width of rendered bytes by stripping the styling.
var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visible(b []byte) string {
	return sgrRE.ReplaceAllString(string(b), "")
}

func TestRender80Cells(t *testing.T) {
	out := Render(View{
		Session:    "dmux",
		WindowIdx:  0,
		WindowName: "bash",
		Current:    true,
		Cols:       80,
	})

	if !bytes.HasPrefix(out, []byte("\x1b[7m")) {
		t.Fatalf("expected SGR reverse-video prefix, got %q", out)
	}
	if !bytes.HasSuffix(out, []byte("\x1b[0m")) {
		t.Fatalf("expected SGR reset suffix, got %q", out)
	}

	v := visible(out)
	if len(v) != 80 {
		t.Fatalf("visible width = %d, want 80: %q", len(v), v)
	}
	if !strings.HasPrefix(v, "[dmux] 0:bash*") {
		t.Fatalf("visible content does not start with label: %q", v)
	}
	if !strings.HasSuffix(v, " ") {
		t.Fatalf("expected trailing padding: %q", v)
	}
}

func TestRenderTruncates(t *testing.T) {
	out := Render(View{
		Session:    "dmux",
		WindowIdx:  0,
		WindowName: "bash",
		Current:    true,
		Cols:       10,
	})

	v := visible(out)
	if len(v) != 10 {
		t.Fatalf("visible width = %d, want 10: %q", len(v), v)
	}
	// "[dmux] 0:bash*" is 14 bytes; truncated to 10 yields "[dmux] 0:b".
	if v != "[dmux] 0:b" {
		t.Fatalf("truncated content = %q, want %q", v, "[dmux] 0:b")
	}
}

func TestRenderEmptySession(t *testing.T) {
	out := Render(View{
		Session:    "",
		WindowIdx:  0,
		WindowName: "bash",
		Current:    true,
		Cols:       20,
	})

	v := visible(out)
	if len(v) != 20 {
		t.Fatalf("visible width = %d, want 20: %q", len(v), v)
	}
	if !strings.HasPrefix(v, "[] 0:bash*") {
		t.Fatalf("empty-session content = %q, want prefix %q", v, "[] 0:bash*")
	}
}

func TestRenderZeroColsReturnsNil(t *testing.T) {
	if out := Render(View{Cols: 0}); out != nil {
		t.Fatalf("Cols=0 should yield nil, got %q", out)
	}
}
