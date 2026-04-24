package dmuxtest

import (
	"errors"
	"reflect"
	"testing"
)

func TestTokenizeSimple(t *testing.T) {
	got, err := tokenize("new-session")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"new-session"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeMultipleWords(t *testing.T) {
	got, err := tokenize("client spawn c")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"client", "spawn", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeQuotedWithSpaces(t *testing.T) {
	got, err := tokenize(`client at =A "echo hi\n"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"client", "at", "=A", "echo hi\n"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeBackslashEscapes(t *testing.T) {
	got, err := tokenize(`send "\x1bOA\r"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"send", "\x1bOA\r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestTokenizeUnterminatedQuote(t *testing.T) {
	_, err := tokenize(`send "unterminated`)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrUnterminatedQuote) {
		t.Fatalf("error %v does not wrap ErrUnterminatedQuote", err)
	}
}

func TestTokenizeEmptyLine(t *testing.T) {
	got, err := tokenize("")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty", got)
	}
}

func TestTokenizeWhitespaceOnly(t *testing.T) {
	got, err := tokenize("   \t  ")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v want empty", got)
	}
}

func TestTokenizeAdjacentQuotedAndRaw(t *testing.T) {
	// "a""b" concatenates into "ab" within one token; a raw run
	// butting up against a quoted run does the same. This matters
	// for scenarios that want a literal string containing a quote
	// character.
	got, err := tokenize(`prefix"suffix"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"prefixsuffix"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
