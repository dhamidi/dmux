package format_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dhamidi/dmux/internal/format"
)

// stubRunner is a CommandRunner that returns canned responses.
type stubRunner struct {
	responses map[string]string
}

func (s *stubRunner) Run(name string, args []string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if out, ok := s.responses[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("stub: unexpected command %q", key)
}

// newStub creates a stubRunner with the given command→output mappings.
// Keys must be the full command string including arguments (space-joined).
func newStub(pairs ...string) *stubRunner {
	s := &stubRunner{responses: make(map[string]string)}
	for i := 0; i+1 < len(pairs); i += 2 {
		s.responses[pairs[i]] = pairs[i+1]
	}
	return s
}

// ctx is a shorthand for creating a MapContext.
func ctx(pairs ...string) format.MapContext {
	m := make(format.MapContext)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}

// ---- MapContext ----

func TestMapContextLookup(t *testing.T) {
	m := ctx("name", "alice")
	v, ok := m.Lookup("name")
	if !ok || v != "alice" {
		t.Fatalf("Lookup(%q): got (%q, %v), want (%q, true)", "name", v, ok, "alice")
	}
}

func TestMapContextLookupMissing(t *testing.T) {
	m := ctx()
	v, ok := m.Lookup("missing")
	if ok || v != "" {
		t.Fatalf("Lookup(missing key): got (%q, %v), want (%q, false)", v, ok, "")
	}
}

func TestMapContextChildrenIsNil(t *testing.T) {
	m := ctx("x", "1")
	if ch := m.Children("x"); ch != nil {
		t.Fatalf("Children: expected nil, got %v", ch)
	}
}

// ---- Literal text ----

func TestLiteralText(t *testing.T) {
	got, err := format.Expand("hello world", ctx())
	if err != nil || got != "hello world" {
		t.Fatalf("Expand literal: got (%q, %v)", got, err)
	}
}

func TestEmptyTemplate(t *testing.T) {
	got, err := format.Expand("", ctx())
	if err != nil || got != "" {
		t.Fatalf("Expand empty: got (%q, %v)", got, err)
	}
}

func TestHashNotFollowedByBrace(t *testing.T) {
	// A bare '#' not followed by '{' or '(' should pass through unchanged.
	got, err := format.Expand("100% done", ctx())
	if err != nil || got != "100% done" {
		t.Fatalf("Expand bare hash: got (%q, %v)", got, err)
	}
}

// ---- Simple variable substitution ----

func TestSimpleSubstitution(t *testing.T) {
	got, err := format.Expand("#{name}", ctx("name", "alice"))
	if err != nil || got != "alice" {
		t.Fatalf("Expand #{name}: got (%q, %v)", got, err)
	}
}

func TestSimpleSubstitutionMissing(t *testing.T) {
	got, err := format.Expand("#{missing}", ctx())
	if err != nil || got != "" {
		t.Fatalf("Expand #{missing}: got (%q, %v)", got, err)
	}
}

func TestMultipleSubstitutions(t *testing.T) {
	got, err := format.Expand("#{a}-#{b}", ctx("a", "foo", "b", "bar"))
	if err != nil || got != "foo-bar" {
		t.Fatalf("Expand multiple: got (%q, %v)", got, err)
	}
}

func TestSubstitutionMixedWithLiteral(t *testing.T) {
	got, err := format.Expand("prefix-#{v}-suffix", ctx("v", "mid"))
	if err != nil || got != "prefix-mid-suffix" {
		t.Fatalf("Expand mixed: got (%q, %v)", got, err)
	}
}

// ---- Conditional ----

func TestConditionalTrue(t *testing.T) {
	got, err := format.Expand("#{?active,yes,no}", ctx("active", "1"))
	if err != nil || got != "yes" {
		t.Fatalf("Expand conditional true: got (%q, %v)", got, err)
	}
}

func TestConditionalFalse(t *testing.T) {
	got, err := format.Expand("#{?active,yes,no}", ctx("active", "0"))
	if err != nil || got != "no" {
		t.Fatalf("Expand conditional false (0): got (%q, %v)", got, err)
	}
}

func TestConditionalMissing(t *testing.T) {
	got, err := format.Expand("#{?active,yes,no}", ctx())
	if err != nil || got != "no" {
		t.Fatalf("Expand conditional missing var: got (%q, %v)", got, err)
	}
}

func TestConditionalNonEmpty(t *testing.T) {
	got, err := format.Expand("#{?name,has name,no name}", ctx("name", "bob"))
	if err != nil || got != "has name" {
		t.Fatalf("Expand conditional non-empty: got (%q, %v)", got, err)
	}
}

func TestConditionalEmptyString(t *testing.T) {
	got, err := format.Expand("#{?name,has name,no name}", ctx("name", ""))
	if err != nil || got != "no name" {
		t.Fatalf("Expand conditional empty string: got (%q, %v)", got, err)
	}
}

func TestConditionalNestedYesBranch(t *testing.T) {
	got, err := format.Expand("#{?flag,#{val},none}", ctx("flag", "1", "val", "42"))
	if err != nil || got != "42" {
		t.Fatalf("Expand conditional nested yes: got (%q, %v)", got, err)
	}
}

func TestConditionalNestedNoBranch(t *testing.T) {
	got, err := format.Expand("#{?flag,yes,#{fallback}}", ctx("flag", "0", "fallback", "fb"))
	if err != nil || got != "fb" {
		t.Fatalf("Expand conditional nested no: got (%q, %v)", got, err)
	}
}

func TestConditionalNestedCondition(t *testing.T) {
	// Condition is itself a format expression.
	got, err := format.Expand("#{?#{active},yes,no}", ctx("active", "1"))
	if err != nil || got != "yes" {
		t.Fatalf("Expand conditional nested cond: got (%q, %v)", got, err)
	}
}

// ---- Truncation ----

func TestTruncationPositive(t *testing.T) {
	got, err := format.Expand("#{=5:title}", ctx("title", "hello world"))
	if err != nil || got != "hello" {
		t.Fatalf("Expand truncate positive: got (%q, %v)", got, err)
	}
}

func TestTruncationNegative(t *testing.T) {
	got, err := format.Expand("#{=-5:title}", ctx("title", "hello world"))
	if err != nil || got != "world" {
		t.Fatalf("Expand truncate negative: got (%q, %v)", got, err)
	}
}

func TestTruncationShorterThanWidth(t *testing.T) {
	got, err := format.Expand("#{=20:title}", ctx("title", "hi"))
	if err != nil || got != "hi" {
		t.Fatalf("Expand truncate shorter: got (%q, %v)", got, err)
	}
}

func TestTruncationExact(t *testing.T) {
	got, err := format.Expand("#{=5:title}", ctx("title", "hello"))
	if err != nil || got != "hello" {
		t.Fatalf("Expand truncate exact: got (%q, %v)", got, err)
	}
}

func TestTruncationZero(t *testing.T) {
	got, err := format.Expand("#{=0:title}", ctx("title", "hello"))
	if err != nil || got != "" {
		t.Fatalf("Expand truncate zero: got (%q, %v)", got, err)
	}
}

func TestTruncationUnicode(t *testing.T) {
	// Each rune is one character regardless of byte length.
	got, err := format.Expand("#{=2:word}", ctx("word", "héllo"))
	if err != nil || got != "hé" {
		t.Fatalf("Expand truncate unicode: got (%q, %v)", got, err)
	}
}

func TestTruncationMissingVar(t *testing.T) {
	got, err := format.Expand("#{=5:missing}", ctx())
	if err != nil || got != "" {
		t.Fatalf("Expand truncate missing var: got (%q, %v)", got, err)
	}
}

// ---- Regex substitution ----

func TestRegexSub(t *testing.T) {
	got, err := format.Expand("#{s/foo/bar/:msg}", ctx("msg", "foo baz foo"))
	if err != nil || got != "bar baz bar" {
		t.Fatalf("Expand regex sub: got (%q, %v)", got, err)
	}
}

func TestRegexSubNoMatch(t *testing.T) {
	got, err := format.Expand("#{s/x/y/:msg}", ctx("msg", "hello"))
	if err != nil || got != "hello" {
		t.Fatalf("Expand regex sub no match: got (%q, %v)", got, err)
	}
}

func TestRegexSubCapture(t *testing.T) {
	got, err := format.Expand(`#{s/(\w+)/[$1]/:msg}`, ctx("msg", "hello world"))
	if err != nil || got != "[hello] [world]" {
		t.Fatalf("Expand regex sub capture: got (%q, %v)", got, err)
	}
}

func TestRegexSubWithFlags(t *testing.T) {
	// flags before ':' are accepted and ignored (all substitutions are global)
	got, err := format.Expand("#{s/a/b/g:word}", ctx("word", "abba"))
	if err != nil || got != "bbbb" {
		t.Fatalf("Expand regex sub with flags: got (%q, %v)", got, err)
	}
}

func TestRegexSubMissingVar(t *testing.T) {
	got, err := format.Expand("#{s/x/y/:missing}", ctx())
	if err != nil || got != "" {
		t.Fatalf("Expand regex sub missing var: got (%q, %v)", got, err)
	}
}

func TestRegexSubInvalidPattern(t *testing.T) {
	_, err := format.Expand("#{s/[invalid/x/:v}", ctx("v", "test"))
	if err == nil {
		t.Fatal("Expand invalid regex: expected error, got nil")
	}
}

// ---- Boolean OR ----

func TestBoolOrBothTrue(t *testing.T) {
	got, err := format.Expand("#{||:a,b}", ctx("a", "1", "b", "1"))
	if err != nil || got != "1" {
		t.Fatalf("|| both true: got (%q, %v)", got, err)
	}
}

func TestBoolOrFirstTrue(t *testing.T) {
	got, err := format.Expand("#{||:a,b}", ctx("a", "1", "b", "0"))
	if err != nil || got != "1" {
		t.Fatalf("|| first true: got (%q, %v)", got, err)
	}
}

func TestBoolOrSecondTrue(t *testing.T) {
	got, err := format.Expand("#{||:a,b}", ctx("a", "0", "b", "1"))
	if err != nil || got != "1" {
		t.Fatalf("|| second true: got (%q, %v)", got, err)
	}
}

func TestBoolOrBothFalse(t *testing.T) {
	got, err := format.Expand("#{||:a,b}", ctx("a", "0", "b", "0"))
	if err != nil || got != "0" {
		t.Fatalf("|| both false: got (%q, %v)", got, err)
	}
}

func TestBoolOrMissing(t *testing.T) {
	got, err := format.Expand("#{||:a,b}", ctx())
	if err != nil || got != "0" {
		t.Fatalf("|| missing vars: got (%q, %v)", got, err)
	}
}

// ---- Boolean AND ----

func TestBoolAndBothTrue(t *testing.T) {
	got, err := format.Expand("#{&&:a,b}", ctx("a", "1", "b", "1"))
	if err != nil || got != "1" {
		t.Fatalf("&& both true: got (%q, %v)", got, err)
	}
}

func TestBoolAndFirstFalse(t *testing.T) {
	got, err := format.Expand("#{&&:a,b}", ctx("a", "0", "b", "1"))
	if err != nil || got != "0" {
		t.Fatalf("&& first false: got (%q, %v)", got, err)
	}
}

func TestBoolAndSecondFalse(t *testing.T) {
	got, err := format.Expand("#{&&:a,b}", ctx("a", "1", "b", "0"))
	if err != nil || got != "0" {
		t.Fatalf("&& second false: got (%q, %v)", got, err)
	}
}

func TestBoolAndBothFalse(t *testing.T) {
	got, err := format.Expand("#{&&:a,b}", ctx("a", "0", "b", "0"))
	if err != nil || got != "0" {
		t.Fatalf("&& both false: got (%q, %v)", got, err)
	}
}

// ---- Time formatting ----

func TestTimeDatetime(t *testing.T) {
	// Unix timestamp 0 = 1970-01-01 00:00:00 UTC
	got, err := format.Expand("#{T:ts}", ctx("ts", "0"))
	if err != nil || got != "1970-01-01 00:00:00" {
		t.Fatalf("#{T:ts} epoch: got (%q, %v)", got, err)
	}
}

func TestTimeDatetimeKnown(t *testing.T) {
	// 2001-09-09 01:46:40 UTC = Unix timestamp 1000000000
	got, err := format.Expand("#{T:ts}", ctx("ts", "1000000000"))
	if err != nil || got != "2001-09-09 01:46:40" {
		t.Fatalf("#{T:ts} known timestamp: got (%q, %v)", got, err)
	}
}

func TestTimeRelativeSeconds(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second).Unix()
	got, err := format.Expand("#{t:ts}", ctx("ts", fmt.Sprintf("%d", ts)))
	if err != nil {
		t.Fatalf("#{t:ts} seconds: unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "s") {
		t.Fatalf("#{t:ts} seconds: expected suffix 's', got %q", got)
	}
}

func TestTimeRelativeMinutes(t *testing.T) {
	ts := time.Now().Add(-5 * time.Minute).Unix()
	got, err := format.Expand("#{t:ts}", ctx("ts", fmt.Sprintf("%d", ts)))
	if err != nil {
		t.Fatalf("#{t:ts} minutes: unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "m") {
		t.Fatalf("#{t:ts} minutes: expected suffix 'm', got %q", got)
	}
}

func TestTimeRelativeHours(t *testing.T) {
	ts := time.Now().Add(-3 * time.Hour).Unix()
	got, err := format.Expand("#{t:ts}", ctx("ts", fmt.Sprintf("%d", ts)))
	if err != nil {
		t.Fatalf("#{t:ts} hours: unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "h") {
		t.Fatalf("#{t:ts} hours: expected suffix 'h', got %q", got)
	}
}

func TestTimeRelativeDays(t *testing.T) {
	ts := time.Now().Add(-48 * time.Hour).Unix()
	got, err := format.Expand("#{t:ts}", ctx("ts", fmt.Sprintf("%d", ts)))
	if err != nil {
		t.Fatalf("#{t:ts} days: unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "d") {
		t.Fatalf("#{t:ts} days: expected suffix 'd', got %q", got)
	}
}

func TestTimeMissingVar(t *testing.T) {
	got, err := format.Expand("#{T:missing}", ctx())
	if err != nil || got != "" {
		t.Fatalf("#{T:missing}: got (%q, %v)", got, err)
	}
}

func TestTimeNotATimestamp(t *testing.T) {
	// Non-integer value is returned unchanged.
	got, err := format.Expand("#{T:ts}", ctx("ts", "not-a-number"))
	if err != nil || got != "not-a-number" {
		t.Fatalf("#{T:ts} non-integer: got (%q, %v)", got, err)
	}
}

// ---- Shell commands ----

func TestShellCommand(t *testing.T) {
	e := format.New(newStub("echo hello", "hello\n"))
	got, err := e.Expand("#(echo hello)", format.MapContext{})
	if err != nil || got != "hello" {
		t.Fatalf("#(echo hello): got (%q, %v)", got, err)
	}
}

func TestShellCommandWithArgs(t *testing.T) {
	e := format.New(newStub("git rev-parse HEAD", "abc123\n"))
	got, err := e.Expand("#(git rev-parse HEAD)", format.MapContext{})
	if err != nil || got != "abc123" {
		t.Fatalf("#(git rev-parse HEAD): got (%q, %v)", got, err)
	}
}

func TestShellCommandNilRunner(t *testing.T) {
	// nil runner → empty string, no error
	got, err := format.Expand("#(echo hello)", format.MapContext{})
	if err != nil || got != "" {
		t.Fatalf("#(echo hello) nil runner: got (%q, %v)", got, err)
	}
}

func TestShellCommandMixedWithLiteral(t *testing.T) {
	e := format.New(newStub("hostname", "myhost"))
	got, err := e.Expand("host: #(hostname)", format.MapContext{})
	if err != nil || got != "host: myhost" {
		t.Fatalf("#(hostname) mixed: got (%q, %v)", got, err)
	}
}

func TestShellCommandTrailingNewlineStripped(t *testing.T) {
	e := format.New(newStub("uname", "Linux\n"))
	got, err := e.Expand("#(uname)", format.MapContext{})
	if err != nil || got != "Linux" {
		t.Fatalf("#(uname) trailing newline: got (%q, %v)", got, err)
	}
}

// ---- Expander type ----

func TestNewExpander(t *testing.T) {
	e := format.New(nil)
	if e == nil {
		t.Fatal("New returned nil")
	}
}

func TestExpanderExpandSameAsPackageExpand(t *testing.T) {
	e := format.New(nil)
	template := "#{x} #{y}"
	c := ctx("x", "foo", "y", "bar")
	got1, err1 := e.Expand(template, c)
	got2, err2 := format.Expand(template, c)
	if err1 != nil || err2 != nil || got1 != got2 {
		t.Fatalf("Expander.Expand vs Expand: (%q,%v) vs (%q,%v)", got1, err1, got2, err2)
	}
}

// ---- Error cases ----

func TestUnclosedBrace(t *testing.T) {
	_, err := format.Expand("#{name", ctx("name", "x"))
	if err == nil {
		t.Fatal("unclosed #{: expected error, got nil")
	}
}

func TestUnclosedParen(t *testing.T) {
	_, err := format.Expand("#(cmd", format.MapContext{})
	if err == nil {
		t.Fatal("unclosed #(: expected error, got nil")
	}
}

func TestConditionalWrongParts(t *testing.T) {
	_, err := format.Expand("#{?a,b}", ctx("a", "1"))
	if err == nil {
		t.Fatal("conditional with 2 parts: expected error, got nil")
	}
}

func TestBoolOrWrongParts(t *testing.T) {
	_, err := format.Expand("#{||:a}", ctx("a", "1"))
	if err == nil {
		t.Fatal("boolean || with 1 part: expected error, got nil")
	}
}

func TestTruncationInvalidWidth(t *testing.T) {
	_, err := format.Expand("#{=abc:var}", ctx("var", "x"))
	if err == nil {
		t.Fatal("truncation invalid width: expected error, got nil")
	}
}

// ---- Nested / combined ----

func TestNestedConditionalInConditional(t *testing.T) {
	// #{?a,#{?b,both,only-a},none}
	got, err := format.Expand("#{?a,#{?b,both,only-a},none}", ctx("a", "1", "b", "1"))
	if err != nil || got != "both" {
		t.Fatalf("nested conditional: got (%q, %v)", got, err)
	}
}

func TestConditionalWithBoolResult(t *testing.T) {
	// Use || result as a condition.
	got, err := format.Expand("#{?#{||:a,b},yes,no}", ctx("a", "0", "b", "1"))
	if err != nil || got != "yes" {
		t.Fatalf("conditional with || result: got (%q, %v)", got, err)
	}
}

func TestTruncationAfterSubstitution(t *testing.T) {
	// Substitute then truncate in separate directives.
	got, err := format.Expand("#{=3:word}", ctx("word", "foobar"))
	if err != nil || got != "foo" {
		t.Fatalf("truncation after sub: got (%q, %v)", got, err)
	}
}

func TestShellCommandResultInConditional(t *testing.T) {
	// Use shell command output as part of a template.
	e := format.New(newStub("true", "1"))
	got, err := e.Expand("result=#(true)", format.MapContext{})
	if err != nil || got != "result=1" {
		t.Fatalf("shell in conditional: got (%q, %v)", got, err)
	}
}
