package complexity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestCountLines(t *testing.T) {
	source := `package main

// This is a comment
func main() {
	if true {
		fmt.Println("a")
	}
}
`
	sloc, indent, maxDepth := countLines(strings.Split(source, "\n"))
	// Non-blank, non-comment lines: package, func, if, Println, two closing braces
	if sloc != 6 {
		t.Errorf("SLOC = %d, want 5", sloc)
	}

	if indent == 0 {
		t.Error("indentation should be > 0 for indented code")
	}

	if maxDepth < 2 {
		t.Errorf("maxDepth = %d, want >= 2", maxDepth)
	}
}

func TestCountLinesBlanksAndComments(t *testing.T) {
	source := "\n\n   \n// comment\n  /* block */\n  * continuation\n"

	sloc, _, _ := countLines(strings.Split(source, "\n"))
	if sloc != 0 {
		t.Errorf("SLOC = %d, want 0 (all blank/comment)", sloc)
	}
}

func TestLeadingIndent(t *testing.T) {
	cases := map[string]int{
		"no indent":      0,
		"\ttab":          tabWidth,
		"\t\tdouble tab": tabWidth * 2,
		"    four space": 4,
		"  two space":    2,
	}
	for line, want := range cases {
		if got := leadingIndent(line); got != want {
			t.Errorf("leadingIndent(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestIsCommentLine(t *testing.T) {
	cases := map[string]bool{
		"// line comment":  true,
		"/* block start":   true,
		"* continuation":   true,
		"-- sql comment":   true,
		"#!/bin/bash":      false,
		"# python comment": true,
		"normal code":      false,
		"":                 false,
	}
	for line, want := range cases {
		if got := isCommentLine(line); got != want {
			t.Errorf("isCommentLine(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestAnalyzeGoCyclomatic(t *testing.T) {
	source := `package main

import "fmt"

func simple() {
	fmt.Println("hello")
}

func complex(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 && i > 1 {
				fmt.Println(i)
			}
		}
	}
	switch x {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 0
	}
}

type Foo struct{}

func (f Foo) method() bool {
	return true
}
`
	path := writeFile(t, "test.go", source)
	fc := Analyze(path)

	if fc.Language != "Go" {
		t.Errorf("Language = %q, want Go", fc.Language)
	}
	// complex(): if(+1) for(+1) if(+1) &&(+1) switch(+1) case1(+1) case2(+1) = 7 decisions + 1 base = 8
	// simple(): 1, method(): 1
	// Total = 1 + (8-1) + (1-1) + (1-1) = 8
	if fc.Cyclomatic != 8 {
		t.Errorf("Cyclomatic = %d, want 8", fc.Cyclomatic)
	}

	if len(fc.Functions) != 3 {
		t.Errorf("Functions = %d, want 3", len(fc.Functions))
	}

	names := make(map[string]bool)
	for _, fn := range fc.Functions {
		names[fn.Name] = true
	}

	if !names["simple"] {
		t.Error("missing function 'simple'")
	}

	if !names["complex"] {
		t.Error("missing function 'complex'")
	}

	if !names["Foo.method"] {
		t.Error("missing method 'Foo.method'")
	}

	var complexFn FuncComplexity

	for _, fn := range fc.Functions {
		if fn.Name == "complex" {
			complexFn = fn
		}
	}

	if complexFn.Cyclomatic != 8 {
		t.Errorf("complex() cyclomatic = %d, want 8", complexFn.Cyclomatic)
	}
}

func TestAnalyzeGoSelectStmt(t *testing.T) {
	source := `package main

func selectFunc(ch chan int) int {
	select {
	case v := <-ch:
		if v > 0 || v < -100 {
			return v
		}
		return 0
	default:
		return -1
	}
}
`
	path := writeFile(t, "select.go", source)
	fc := Analyze(path)
	// select(+1) case(+1) if(+1) ||(+1) default-case(+1) = 5 decisions + 1 base = 6
	if fc.Cyclomatic != 6 {
		t.Errorf("Cyclomatic = %d, want 6", fc.Cyclomatic)
	}
}

func TestAnalyzeNonGo(t *testing.T) {
	source := `def hello():
    if True:
        print("a")
    else:
        print("b")
`
	path := writeFile(t, "test.py", source)
	fc := Analyze(path)

	if fc.Language != "Python" {
		t.Errorf("Language = %q, want Python", fc.Language)
	}

	if fc.SLOC == 0 {
		t.Error("SLOC should be > 0")
	}

	if fc.Indentation == 0 {
		t.Error("Indentation should be > 0")
	}

	if len(fc.Functions) != 0 {
		t.Error("Non-Go should have no function breakdown")
	}

	if fc.Cyclomatic < 1 {
		t.Error("Cyclomatic should be >= 1")
	}
}

func TestAnalyzeMissingFile(t *testing.T) {
	fc := Analyze("/nonexistent/file.go")
	if fc.SLOC != 0 || fc.Cyclomatic != 0 {
		t.Error("missing file should return zero-value complexity")
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := map[string]string{
		"main.go":     "Go",
		"app.py":      "Python",
		"index.js":    "JavaScript",
		"App.tsx":     "TSX",
		"main.rs":     "Rust",
		"Main.java":   "Java",
		"main.c":      "C",
		"main.cpp":    "C++",
		"Program.cs":  "C#",
		"app.rb":      "Ruby",
		"index.php":   "PHP",
		"run.sh":      "Bash",
		"build.scala": "Scala",
		"main.swift":  "Swift",
		"Main.kt":     "Kotlin",
		"config.lua":  "Lua",
		"README.md":   "Other",
		"unknown.xyz": "Other",
	}
	for path, want := range cases {
		if got := detectLanguage(path); got != want {
			t.Errorf("detectLanguage(%q) = %q, want %q", path, got, want)
		}
	}
}

func BenchmarkAnalyze(b *testing.B) {
	source := `package main

import "fmt"

func alpha(x int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 && i > 1 {
				fmt.Println(i)
			}
		}
	}
	switch x {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 0
	}
}

func beta(s string) bool {
	for _, c := range s {
		if c == 'x' || c == 'y' {
			return true
		}
	}
	return false
}
`

	path := filepath.Join(b.TempDir(), "bench.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for range b.N {
		Analyze(path)
	}
}
