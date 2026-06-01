package engine_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEnginePurity enforces the architectural invariant that the engine is a
// pure render library: it may import only the standard library, third-party
// packages, the domain types (internal/domain) and its own subpackages
// (internal/engine/...). It must NOT reach into any other internal layer —
// adapters (anthropic/openai/postgres), handlers, usecases or prompts —
// because rendering runs in-process in microseconds and must never touch the
// network, a database, or the LLM pipeline.
//
// This is the guard behind the "don't split the pipeline out of v5" decision
// (see memory: project-architecture-audit). The only architectural argument
// for splitting would be engine purity, and that purity is far cheaper to keep
// true with this test than to enforce by carving out a separate service. The
// leak this test was born to prevent was internal/engine/tokens/count_tokens.go
// importing adapters/anthropic; that token-measurement scaffolding now lives at
// internal/measure/tokens, outside the engine tree.
//
// If this test fails: you added a dependency that breaks the modular-monolith
// boundary. Move the offending code out of internal/engine/ rather than
// loosening this rule.
func TestEnginePurity(t *testing.T) {
	const modulePrefix = "keepstar_v5/internal/"

	allowed := func(imp string) bool {
		if !strings.HasPrefix(imp, modulePrefix) {
			return true // stdlib or third-party
		}
		rest := strings.TrimPrefix(imp, modulePrefix)
		switch {
		case rest == "domain" || strings.HasPrefix(rest, "domain/"):
			return true
		case rest == "engine" || strings.HasPrefix(rest, "engine/"):
			return true
		default:
			return false
		}
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path via runtime.Caller")
	}
	engineDir := filepath.Dir(thisFile)

	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(engineDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Scan non-test production sources only. Test files do not ship in the
		// render path, and build-tagged measurement files live elsewhere now.
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// parser.ImportsOnly parses regardless of build tags, so a tagged leak
		// would still be caught.
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			if !allowed(imp) {
				rel, _ := filepath.Rel(engineDir, path)
				t.Errorf("engine purity violation: %s imports %q\n"+
					"  engine may import only stdlib, third-party, internal/domain and internal/engine/...\n"+
					"  move the offending code out of internal/engine/ instead of importing across the boundary", rel, imp)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking engine dir %s: %v", engineDir, walkErr)
	}
}
