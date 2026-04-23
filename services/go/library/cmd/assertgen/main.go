package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
)

//go:embed template.go.tmpl
var rawTemplate string

// FuncDef defines the components used to generate a Go test helper function.
// It acts as the data transfer object between the AST parser and the text template.
type FuncDef struct {
	// Name is the name of the function to be generated (e.g., "Equal").
	Name string

	// TypeParams defines the generic type parameters including brackets.
	// Example: "[T any]".
	TypeParams string

	// Params defines the input parameters for the generated function,
	// excluding testing.T and message arguments.
	Params string

	// ParamNames defines the names of the parameters to be passed to the underlying call.
	ParamNames string

	// Doc is the processed comment block for the function.
	Doc string
}

func main() {
	// 1. Load the compare package
	// We use NeedSyntax and NeedTypes to inspect the actual source code structure.
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, "../../internal/compare")
	if err != nil || len(pkgs) == 0 {
		slog.Error("failed to load compare package", "error", err)
		os.Exit(1)
	}

	var funcs []FuncDef
	hasError := false
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				// Only process exported function declarations.
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}

				// Check if the function signature matches the expected helper pattern.
				if isTestFunc(fn) {
					f, err := parseFunc(fn)
					if err != nil {
						slog.Error("failed to parse func definition", "name", fn.Name.Name, "error", err)
						hasError = true
						continue
					}
					funcs = append(funcs, f)
				}
			}
		}
	}

	if len(funcs) == 0 {
		slog.Error("no valid functions found to generate")
		os.Exit(1)
	}

	// 2. Generate Layer 1 (Public Packages)
	// We generate two packages from the same source:
	// - assert: uses t.Errorf (non-terminating)
	// - require: uses t.Fatalf (terminating)
	targets := []struct {
		dir      string
		pkg      string
		failFunc string
	}{
		{"../../assert", "assert", "Error"},
		{"../../require", "require", "Fatal"},
	}

	for _, target := range targets {
		if err := generate(target.dir, target.pkg, target.failFunc, funcs); err != nil {
			slog.Error("generation failed", "package", target.pkg, "error", err)
			hasError = true
		}
	}

	if hasError {
		slog.Error("test helper generation completed with errors; check logs above")
		os.Exit(1)
	}

	slog.Info("test helper generation successful", "count", len(funcs))
}

// isTestFunc performs a structural check to ensure the function is a testing helper.
// It verifies that:
// 1. The function has at least one parameter (the *testing.T).
// 2. The function returns exactly two values: (string, bool).
func isTestFunc(fn *ast.FuncDecl) bool {
	// 1. Check Parameters
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}

	// 2. Check Return Values: must be (string, bool)
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 2 {
		return false
	}

	// Verify first return is 'string'
	res1, err := exprToString(fn.Type.Results.List[0].Type)
	if err != nil {
		slog.Error("failed to convert the first result type: %v", "err", err)
		os.Exit(1)
	}
	if res1 != "string" {
		return false
	}

	// Verify second return is 'bool'
	res2, err := exprToString(fn.Type.Results.List[1].Type)
	if err != nil {
		slog.Error("failed to convert the second result type: %v", "err", err)
		os.Exit(1)
	}
	return res2 == "bool"
}

// parseFunc extracts metadata from a function declaration to populate a FuncDef.
// It filters out standard testing parameters to create a clean API for the public packages.
func parseFunc(fn *ast.FuncDecl) (FuncDef, error) {
	var tParams, params, pNames []string

	// Handle Generics: extracts [T any, K comparable] etc.
	if fn.Type.TypeParams != nil {
		for _, field := range fn.Type.TypeParams.List {
			fieldType, err := exprToString(field.Type)
			if err != nil {
				slog.Error("failed to convert type params: %w", "err", err)
				os.Exit(1)
			}
			for _, name := range field.Names {
				tParams = append(tParams, fmt.Sprintf("%s %s", name.Name, fieldType))
			}
		}
	}

	// Handle Parameters:
	// We skip the first parameter (t *testing.T) and the trailing (msg, msgArgs)
	// because they are injected manually by the template.
	for i, field := range fn.Type.Params.List {
		if i == 0 {
			continue
		}

		fieldType, err := exprToString(field.Type)
		if err != nil {
			slog.Error("failed to convert param type: %w", "err", err)
			os.Exit(1)
		}
		for _, name := range field.Names {
			if name.Name == "msg" || name.Name == "msgArgs" {
				continue
			}
			params = append(params, fmt.Sprintf("%s %s", name.Name, fieldType))
			pNames = append(pNames, name.Name)
		}
	}

	tp := ""
	if len(tParams) > 0 {
		tp = "[" + strings.Join(tParams, ", ") + "]"
	}

	// Extract and format docstrings for the generated file.
	var docLines []string
	if fn.Doc != nil {
		rawDoc := strings.TrimSpace(fn.Doc.Text())
		if rawDoc != "" {
			for line := range strings.SplitSeq(rawDoc, "\n") {
				docLines = append(docLines, "// "+line)
			}
		}
	}

	return FuncDef{
		Name:       fn.Name.Name,
		TypeParams: tp,
		Params:     suffix(strings.Join(params, ", ")),
		ParamNames: suffix(strings.Join(pNames, ", ")),
		Doc:        strings.Join(docLines, "\n"),
	}, nil
}

// suffix adds a trailing comma and space to a string if it is not empty.
// This ensures generated function signatures like (t, a, b, msg) remain valid.
func suffix(s string) string {
	if s == "" {
		return ""
	}
	return s + ", "
}

// exprToString converts an AST expression back into its Go source code string representation.
func exprToString(expr ast.Expr) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generate executes the template for a specific package (assert or require),
// formats the output with gofmt, and writes the result to a file.
func generate(dir, pkgName, failFunc string, funcs []FuncDef) error {
	t := template.Must(template.New("gen").Parse(rawTemplate))
	var buf bytes.Buffer

	data := map[string]any{
		"PackageName": pkgName,
		"FailFunc":    failFunc,
		"Funcs":       funcs,
	}

	if err := t.Execute(&buf, data); err != nil {
		return err
	}

	// format.Source ensures the generated code follows gofmt rules before saving.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt failed on generated code: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, pkgName+".go"), formatted, 0o644)
}
