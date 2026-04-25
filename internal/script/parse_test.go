package script

import (
	"errors"
	"reflect"
	"testing"
)

func TestTokenizeSimple(t *testing.T) {
	got, err := Tokenize("new-session")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	want := []string{"new-session"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeMultipleWords(t *testing.T) {
	got, err := Tokenize("client spawn c")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	want := []string{"client", "spawn", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeQuotedWithSpaces(t *testing.T) {
	got, err := Tokenize(`client at =A "echo hi\n"`)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	want := []string{"client", "at", "=A", "echo hi\n"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeBackslashEscapes(t *testing.T) {
	got, err := Tokenize(`send "\x1bOA\r"`)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	want := []string{"send", "\x1bOA\r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeUnterminatedQuote(t *testing.T) {
	_, err := Tokenize(`send "unterminated`)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrUnterminatedQuote) {
		t.Fatalf("error %v does not wrap ErrUnterminatedQuote", err)
	}
}

func TestTokenizeEmptyLine(t *testing.T) {
	got, err := Tokenize("")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty", got)
	}
}

func TestTokenizeWhitespaceOnly(t *testing.T) {
	got, err := Tokenize("   \t  ")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty", got)
	}
}

func TestTokenizeCommentLine(t *testing.T) {
	got, err := Tokenize("# this is a comment")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty (comment)", got)
	}
}

func TestTokenizeIndentedComment(t *testing.T) {
	got, err := Tokenize("   \t# indented")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty (indented comment)", got)
	}
}

func TestTokenizeAdjacentQuotedAndRaw(t *testing.T) {
	got, err := Tokenize(`prefix"suffix"`)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	want := []string{"prefixsuffix"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
