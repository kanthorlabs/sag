package agentsetup

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/sql"
)

func TestSkillMarker(t *testing.T) {
	marker := []byte("<!-- managed by sag agent setup -->")
	if !bytes.Contains(skillMD, marker) {
		t.Errorf("SKILL.md missing marker %q", marker)
	}
}

func TestDXAC49_SkillNamesToolsAndCodes(t *testing.T) {
	toolNames := []string{
		"mcp__sag__list_resources",
		"mcp__sag__" + sql.CapabilityName + "_" + sql.OperationListTables,
		"mcp__sag__" + sql.CapabilityName + "_" + sql.OperationDescribeTable,
		"mcp__sag__" + sql.CapabilityName + "_" + sql.OperationQuery,
	}
	for _, name := range toolNames {
		if !bytes.Contains(skillMD, []byte(name)) {
			t.Errorf("SKILL.md missing tool name %q", name)
		}
	}

	errorCodes := []core.Code{
		core.CodeUnauthenticated,
		core.CodeNotFound,
		core.CodeInvalidArgument,
		core.CodeQueryRejected,
		core.CodeTimeout,
		core.CodeRateLimited,
		core.CodeResourceUnavailable,
		core.CodeUpstreamError,
		core.CodeInternal,
	}
	for _, code := range errorCodes {
		needle := []byte("`" + string(code) + "`")
		if !bytes.Contains(skillMD, needle) {
			t.Errorf("SKILL.md missing error code backtick form %s", needle)
		}
	}
}

func TestEmbedExcludesTestFile(t *testing.T) {
	src, err := os.ReadFile("agentsetup.go")
	if err != nil {
		t.Fatalf("read agentsetup.go: %v", err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "agentsetup.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse agentsetup.go: %v", err)
	}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if bytes.Contains([]byte(c.Text), []byte(".test.ts")) {
				t.Errorf("go:embed in agentsetup.go references a test file: %s", c.Text)
			}
		}
	}
	if bytes.Contains(skillMD, []byte("node:test")) {
		t.Error("skillMD embedded content contains node:test (looks like a test file)")
	}
	if bytes.Contains(sagGuardTS, []byte("node:test")) {
		t.Error("sagGuardTS embedded content contains node:test (looks like a test file)")
	}
}
