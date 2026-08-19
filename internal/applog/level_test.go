package applog

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"debug":   LevelDebug,
		"info":    LevelInfo,
		"INFO":    LevelInfo,
		" warn":   LevelWarn,
		"warning": LevelWarn,
		"error":   LevelError,
		"":        LevelInfo, // unset config field keeps the default
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil {
			t.Fatalf("ParseLevel(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel(\"verbose\") should fail")
	}
}

// TestThreshold pins what each level lets through, including the property the
// option exists for: "warn" silences the per-request access log.
func TestThreshold(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(nil)
		log.SetFlags(log.LstdFlags)
		SetLevel(LevelInfo)
	})

	cases := []struct {
		level Level
		want  []string // substrings expected in the output
		gone  []string
	}{
		{LevelDebug, []string{"dbg", "inf", "wrn", "err"}, nil},
		{LevelInfo, []string{"inf", "wrn", "err"}, []string{"dbg"}},
		{LevelWarn, []string{"wrn", "err"}, []string{"dbg", "inf"}},
		{LevelError, []string{"err"}, []string{"dbg", "inf", "wrn"}},
	}
	for _, tc := range cases {
		buf.Reset()
		SetLevel(tc.level)
		Debugf("dbg")
		Infof("inf")
		Warnf("wrn")
		Errorf("err")
		out := buf.String()
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Fatalf("level %v: missing %q in %q", tc.level, w, out)
			}
		}
		for _, g := range tc.gone {
			if strings.Contains(out, g) {
				t.Fatalf("level %v: %q should be suppressed, got %q", tc.level, g, out)
			}
		}
	}

	// Prefixes are the syslog sink's only severity signal, so pin them.
	buf.Reset()
	SetLevel(LevelDebug)
	Debugf("x")
	Infof("y")
	Warnf("z")
	Errorf("q")
	want := "debug: x\ny\nwarning: z\nerror: q\n"
	if buf.String() != want {
		t.Fatalf("format:\n%q\nwant:\n%q", buf.String(), want)
	}
}
