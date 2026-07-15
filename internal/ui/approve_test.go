package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirmApplyYes(t *testing.T) {
	var out bytes.Buffer
	ok, err := ConfirmApply(strings.NewReader("y\n"), &out)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v out=%q", ok, err, out.String())
	}
	if !strings.Contains(out.String(), "Apply this plan?") {
		t.Fatalf("missing prompt: %q", out.String())
	}
}

func TestConfirmApplyYesWord(t *testing.T) {
	ok, err := ConfirmApply(strings.NewReader("yes\n"), ioDiscard{})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestConfirmApplyNoAndEmpty(t *testing.T) {
	for _, in := range []string{"n\n", "\n", "no\n", "maybe\n"} {
		ok, err := ConfirmApply(strings.NewReader(in), ioDiscard{})
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("expected abort for %q", in)
		}
	}
}

func TestConfirmApplyEOF(t *testing.T) {
	ok, err := ConfirmApply(strings.NewReader(""), ioDiscard{})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("EOF should abort")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
