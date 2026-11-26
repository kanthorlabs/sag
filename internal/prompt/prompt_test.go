package prompt

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeTerminal struct {
	r io.Reader
	w io.Writer
}

func (f *fakeTerminal) Read(p []byte) (int, error)  { return f.r.Read(p) }
func (f *fakeTerminal) Write(p []byte) (int, error) { return f.w.Write(p) }
func (f *fakeTerminal) Close() error                { return nil }

func fakeAsker(input string) Asker {
	return Asker{Open: func() (io.ReadWriteCloser, error) {
		return &fakeTerminal{r: strings.NewReader(input), w: io.Discard}, nil
	}}
}

func capturingAsker(input string, out *bytes.Buffer) Asker {
	return Asker{Open: func() (io.ReadWriteCloser, error) {
		return &fakeTerminal{r: strings.NewReader(input), w: out}, nil
	}}
}

func panicAsker() Asker {
	return Asker{Open: func() (io.ReadWriteCloser, error) {
		panic("Open called unexpectedly")
	}}
}

func noTermAsker() Asker {
	return Asker{Open: func() (io.ReadWriteCloser, error) {
		return nil, ErrNoTerminal
	}}
}

func TestFlagBeatsStored(t *testing.T) {
	a := panicAsker()
	got, err := ResolveYesNo(a, "yes", "no", "question?", "--flag")
	if err != nil {
		t.Fatalf("ResolveYesNo flag: %v", err)
	}
	if got != "yes" {
		t.Fatalf("ResolveYesNo = %q; want yes", got)
	}
}

func TestStoredBeatsPrompt(t *testing.T) {
	a := panicAsker()
	got, err := ResolveYesNo(a, "", "no", "question?", "--flag")
	if err != nil {
		t.Fatalf("ResolveYesNo stored: %v", err)
	}
	if got != "no" {
		t.Fatalf("ResolveYesNo = %q; want no", got)
	}
}

func TestNoTerminalNamesFlag(t *testing.T) {
	a := noTermAsker()
	_, err := ResolveYesNo(a, "", "", "question?", "--myflag")
	if err == nil {
		t.Fatal("ResolveYesNo no terminal: want error")
	}
	var re *RefusalError
	if !errors.As(err, &re) {
		t.Fatalf("error type = %T; want *RefusalError", err)
	}
	if re.Flag != "--myflag" {
		t.Fatalf("RefusalError.Flag = %q; want --myflag", re.Flag)
	}
}

func TestOnlyYOrYesApproves(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"y\n", "yes"},
		{"yes\n", "yes"},
		{"n\n", "no"},
		{"no\n", "no"},
		{"Y\n", "no"},
		{"Yes\n", "no"},
		{"YES\n", "no"},
		{"\n", "no"},
		{"maybe\n", "no"},
		{"y ", "no"},
	}
	for _, tc := range cases {
		a := fakeAsker(tc.input)
		got, err := ResolveYesNo(a, "", "", "question?", "--flag")
		if err != nil {
			t.Fatalf("input %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("input %q: ResolveYesNo = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestYesNoDirectly(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"no\n", false},
		{"n\n", false},
		{"Y\n", false},
	}
	for _, tc := range cases {
		a := fakeAsker(tc.input)
		got, err := a.YesNo("q?")
		if err != nil {
			t.Fatalf("input %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("input %q: YesNo = %v; want %v", tc.input, got, tc.want)
		}
	}
}

func TestYesNoNoTerminal(t *testing.T) {
	a := noTermAsker()
	_, err := a.YesNo("q?")
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("YesNo no terminal = %v; want ErrNoTerminal", err)
	}
}

func TestFlagEmptyStoredEmpty(t *testing.T) {
	a := fakeAsker("yes\n")
	got, err := ResolveYesNo(a, "", "", "question?", "--flag")
	if err != nil {
		t.Fatalf("ResolveYesNo prompt: %v", err)
	}
	if got != "yes" {
		t.Fatalf("ResolveYesNo = %q; want yes", got)
	}
}

func TestQuestionWrittenToTerminal(t *testing.T) {
	var out bytes.Buffer
	a := capturingAsker("y\n", &out)
	_, err := a.YesNo("Start SAG at login? [y/N] ")
	if err != nil {
		t.Fatalf("YesNo: %v", err)
	}
	if out.String() != "Start SAG at login? [y/N] " {
		t.Fatalf("terminal output = %q; want %q", out.String(), "Start SAG at login? [y/N] ")
	}
}
