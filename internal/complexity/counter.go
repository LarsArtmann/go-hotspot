// Package complexity computes source code complexity metrics.
//
// For Go files, it uses go/ast to compute true cyclomatic complexity with zero
// CGo. For all other languages, it uses indentation-based complexity — the same
// approach CodeScene uses in production, which is language-neutral and correlates
// well with branching/looping structure.
package complexity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// FileComplexity holds complexity metrics for a single source file.
type FileComplexity struct {
	Path        string
	Language    string
	SLOC        int // non-blank, non-comment source lines
	Indentation int // total indentation units (language-neutral complexity proxy)
	MaxDepth    int // deepest nesting level (indentation units on the most-indented line)
	Cyclomatic  int // Go: true McCabe via go/ast; other: estimated from indentation
	Functions   []FuncComplexity
}

// FuncComplexity holds per-function metrics (Go only).
type FuncComplexity struct {
	Name       string
	Cyclomatic int
	StartLine  int
	LineCount  int
}

// tabWidth is the number of space-equivalents for one tab character.
const tabWidth = 4

// Analyze computes complexity metrics for a file. Returns zero-value on error.
func Analyze(path string) FileComplexity {
	fc := FileComplexity{Path: path, Language: detectLanguage(path)}

	data, err := os.ReadFile(path)
	if err != nil {
		return fc
	}

	lines := strings.Split(string(data), "\n")
	fc.SLOC, fc.Indentation, fc.MaxDepth = countLines(lines)

	if fc.Language == "Go" {
		fc.Cyclomatic, fc.Functions = analyzeGo(path, data)
	} else {
		// Estimate cyclomatic from indentation: deeper nesting ≈ more branches.
		// A reasonable heuristic: indentation / 4 + 1 (minimum complexity).
		fc.Cyclomatic = fc.Indentation/tabWidth + 1
	}

	return fc
}

// countLines returns SLOC, total indentation, and max nesting depth.
func countLines(lines []string) (sloc, totalIndent, maxDepth int) {
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		if isCommentLine(trimmed) {
			continue
		}

		sloc++
		indent := leadingIndent(raw)
		totalIndent += indent

		depth := indent / tabWidth
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return sloc, totalIndent, maxDepth
}

// leadingIndent returns the space-equivalent count of leading whitespace.
func leadingIndent(line string) int {
	n := 0

	for _, ch := range line {
		switch ch {
		case '\t':
			n += tabWidth
		case ' ':
			n++
		default:
			return n
		}
	}

	return n
}

// isCommentLine reports whether a trimmed line is comment-only.
func isCommentLine(t string) bool {
	return strings.HasPrefix(t, "//") ||
		strings.HasPrefix(t, "/*") ||
		strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "--") ||
		strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!")
}

// analyzeGo parses Go source and returns cyclomatic complexity + function breakdown.
func analyzeGo(path string, src []byte) (int, []FuncComplexity) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return 1, nil
	}

	var funcs []FuncComplexity

	total := 1

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		cyc := cyclomaticOfFunc(fn)
		total += cyc - 1 // function starts at 1, so subtract the base
		funcs = append(funcs, FuncComplexity{
			Name:       funcName(fn),
			Cyclomatic: cyc,
			StartLine:  fset.Position(fn.Body.Pos()).Line,
			LineCount:  fset.Position(fn.Body.End()).Line - fset.Position(fn.Body.Pos()).Line,
		})

		return true
	})

	if total < 1 {
		total = 1
	}

	return total, funcs
}

// cyclomaticOfFunc computes McCabe cyclomatic complexity for a Go function.
// Base = 1, +1 for each decision point.
func cyclomaticOfFunc(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 1
	}

	cyc := 1

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
			*ast.TypeSwitchStmt, *ast.SelectStmt:
			cyc++
		case *ast.CaseClause:
			// default case doesn't add complexity, only case with values
			if len(n.List) > 0 {
				cyc++
			}
		case *ast.CommClause:
			cyc++ // case in select
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				cyc++
			}
		}

		return true
	})

	return cyc
}

// funcName returns a readable name for a function declaration.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		// Method: ReceiverType.MethodName
		recv := typeString(fn.Recv.List[0].Type)

		return recv + "." + fn.Name.Name
	}

	return fn.Name.Name
}

// typeString returns a rough string representation of a type expression.
func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.IndexExpr:
		return typeString(t.X)
	default:
		return "T"
	}
}

// detectLanguage returns the language name based on file extension.
func detectLanguage(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js", ".mjs", ".cjs":
		return "JavaScript"
	case ".jsx":
		return "JSX"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TSX"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	case ".sh", ".bash":
		return "Bash"
	case ".scala", ".sc":
		return "Scala"
	case ".swift":
		return "Swift"
	case ".kt", ".kts":
		return "Kotlin"
	case ".lua":
		return "Lua"
	default:
		return "Other"
	}
}
