package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/e2e"
)

var imageRef = imageFromEnv()

func imageFromEnv() string {
	if v := os.Getenv("SAG_E2E_IMAGE"); v != "" {
		return v
	}
	return "localhost/sag:e2e"
}

const (
	jsonFileName = "ac-report.json"
	mdFileName   = "ac-report.md"
	maxLayer     = 6
)

var majorDir = regexp.MustCompile(`^pg(\d+)$`)

type inputPaths []string

func (p *inputPaths) String() string {
	return strings.Join(*p, ",")
}

func (p *inputPaths) Set(path string) error {
	*p = append(*p, path)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr, probe))
}

func run(args []string, stderr io.Writer, probe func() buildInfo) int {
	if len(args) > 0 && args[0] == mergeCommand {
		return runMerge(args[1:], stderr)
	}
	flags := flag.NewFlagSet("acreport", flag.ContinueOnError)
	flags.SetOutput(stderr)
	layer := flags.Int("layer", 0, "gate layer, 1 to 6")
	var inputs inputPaths
	flags.Var(&inputs, "in", "go test -json events file at <dir>/pg<major>/<file>; repeat for each major")
	out := flags.String("out", "", "output directory for "+jsonFileName+" and "+mdFileName)
	skip := flags.String("skip", "", "AC IDs that the gate excludes, separated by comma or space")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := checkArgs(*layer, inputs, *out, flags.NArg()); err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	runs, err := readRuns(inputs)
	if err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	r := buildReport(*layer, runs, e2e.ACTable, skipIDs(*skip), probe())
	if err := writeReport(*out, r); err != nil {
		fmt.Fprintf(stderr, "acreport: %v\n", err)
		return 2
	}
	if !r.Gate.Passed {
		fmt.Fprintf(stderr, "acreport: gate layer %d failed:\n", r.Gate.Layer)
		for _, test := range r.Gate.Failing {
			fmt.Fprintf(stderr, "  %s\n", test)
		}
		return 1
	}
	return 0
}

func checkArgs(layer int, inputs []string, out string, extra int) error {
	switch {
	case layer < 1 || layer > maxLayer:
		return fmt.Errorf("-layer must be 1 to %d", maxLayer)
	case len(inputs) == 0:
		return errors.New("-in is required")
	case out == "":
		return errors.New("-out is required")
	case extra > 0:
		return errors.New("unexpected arguments")
	}
	return nil
}

func skipIDs(skip string) []string {
	return strings.FieldsFunc(skip, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
}

func majorFromPath(path string) (int, error) {
	m := majorDir.FindStringSubmatch(filepath.Base(filepath.Dir(path)))
	if m == nil {
		return 0, fmt.Errorf("%s: parent directory is not pg<major>", path)
	}
	return strconv.Atoi(m[1])
}

func readRuns(paths []string) ([]majorRun, error) {
	seen := map[int]bool{}
	var runs []majorRun
	for _, path := range paths {
		major, err := majorFromPath(path)
		if err != nil {
			return nil, err
		}
		if seen[major] {
			return nil, fmt.Errorf("%s: major %d given more than one time", path, major)
		}
		seen[major] = true
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		tr, err := parseEvents(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		runs = append(runs, majorRun{Major: major, Run: tr})
	}
	return runs, nil
}

func writeReport(dir string, r report) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, jsonFileName), append(data, '\n'), 0o644); err != nil {
		return err
	}
	var md bytes.Buffer
	if err := writeMarkdown(&md, r); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, mdFileName), md.Bytes(), 0o644)
}

func probe() buildInfo {
	return buildInfo{
		GeneratedAt:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Commit:       commandOutput("git", "rev-parse", "HEAD"),
		Dirty:        commandOutput("git", "status", "--porcelain") != "",
		Image:        commandOutput("podman", "image", "inspect", "--format", "{{.Digest}}", imageRef),
		ImageVersion: commandOutput("podman", "run", "--rm", "--network", "none", imageRef, "version"),
	}
}

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
