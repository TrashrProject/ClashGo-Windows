package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestFrontendWailsImportsMatchAppMethods(t *testing.T) {
	appMethods := map[string]bool{}
	fset := token.NewFileSet()

	rootEntries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range rootEntries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || !fn.Name.IsExported() {
				continue
			}
			recv := fn.Recv.List[0].Type
			isApp := false
			switch typed := recv.(type) {
			case *ast.StarExpr:
				if ident, ok := typed.X.(*ast.Ident); ok && ident.Name == "App" {
					isApp = true
				}
			case *ast.Ident:
				isApp = typed.Name == "App"
			}
			if isApp {
				appMethods[fn.Name.Name] = true
			}
		}
	}

	importRE := regexp.MustCompile("(?s)import\\s*\\{([^}]*)\\}\\s*from\\s*['\"][^'\"]*wailsjs/go/main/App['\"]")
	identifierRE := regexp.MustCompile("^[A-Za-z_$][A-Za-z0-9_$]*$")

	var missing []string
	err = filepath.WalkDir("web/src", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range importRE.FindAllStringSubmatch(string(data), -1) {
			for _, raw := range strings.Split(match[1], ",") {
				name := strings.TrimSpace(raw)
				if fields := strings.Fields(name); len(fields) >= 3 && fields[len(fields)-2] == "as" {
					name = fields[0]
				}
				if name == "" || !identifierRE.MatchString(name) {
					continue
				}
				if !appMethods[name] {
					missing = append(missing, path+": "+name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan frontend imports: %v", err)
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("frontend imports Wails methods not exposed by Go App:\\n%s", strings.Join(missing, "\\n"))
	}
}
