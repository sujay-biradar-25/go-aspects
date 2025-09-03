package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa/ssautil"
)

type CallGraphResult struct {
	PackageID   string              `json:"package_id"`
	PackageName string              `json:"package_name"`
	ImportPath  string              `json:"import_path"`
	CallGraph   map[string][]string `json:"call_graph"`
	TotalFuncs  int                 `json:"total_functions"`
	TotalEdges  int                 `json:"total_edges"`
	Algorithm   string              `json:"algorithm"`
}

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Usage: %s <packages_json_file> <output_file>", os.Args[0])
	}

	packagesFile := os.Args[1]
	outputFile := os.Args[2]

	fmt.Fprintf(os.Stderr, "🔍 VTA Analysis in Bazel Environment\n")
	fmt.Fprintf(os.Stderr, "📄 Reading packages file: %s\n", packagesFile)

	// Debug: List all files in current directory
	if files, err := os.ReadDir("."); err == nil {
		fmt.Fprintf(os.Stderr, "📁 Files in current directory:\n")
		for _, file := range files {
			fmt.Fprintf(os.Stderr, "  - %s\n", file.Name())
		}
	} else {
		fmt.Fprintf(os.Stderr, "❌ Failed to read current directory: %v\n", err)
	}

	// Read the packages JSON response to extract target information
	data, err := os.ReadFile(packagesFile)
	if err != nil {
		log.Fatalf("Failed to read packages file: %v", err)
	}

	// Parse just to get the root package information
	var response struct {
		Roots    []string `json:"Roots"`
		Packages []struct {
			ID      string   `json:"ID"`
			Name    string   `json:"Name"`
			PkgPath string   `json:"PkgPath"`
			GoFiles []string `json:"GoFiles"`
		} `json:"Packages"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		log.Fatalf("Failed to parse packages JSON: %v", err)
	}

	fmt.Fprintf(os.Stderr, "🎯 Target roots: %v\n", response.Roots)

	// Find the main package to analyze
	var targetPackage string
	var targetID string
	var targetName string

	for _, pkg := range response.Packages {
		// Look for the main source package (not stdlib)
		if pkg.ID != "" && (strings.HasPrefix(pkg.ID, "@@//") || strings.HasPrefix(pkg.ID, "@//")) {
			if len(pkg.GoFiles) > 0 {
				// Extract the actual package path for loading
				targetPackage = pkg.PkgPath
				targetID = pkg.ID
				targetName = pkg.Name
				fmt.Fprintf(os.Stderr, "📦 Found target package: %s (ID: %s, Path: %s)\n", targetName, targetID, targetPackage)
				break
			}
		}
	}

	if targetPackage == "" {
		fmt.Fprintf(os.Stderr, "⚠️ No source package found, trying to use workspace root pattern\n")
		targetPackage = "./..."
	}

	// Set up Go environment for Bazel sandbox
	setupGoEnvironment()

	// Configure packages.Load to load the target package and its dependencies
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedExportFile |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Tests: false,
		Env:   buildGoEnvironment(),
	}

	fmt.Fprintf(os.Stderr, "🔄 Loading packages with pattern: %s\n", targetPackage)

	// In Bazel environment, we need to work with a different approach
	// since the source directories may not exist in the sandbox.
	// Let's try to load the current directory as a module
	var pkgs []*packages.Package
	var loadErr error

	fmt.Fprintf(os.Stderr, "🔄 Loading current directory as Go module\n")
	pkgs, loadErr = packages.Load(cfg, ".")
	if loadErr != nil || len(pkgs) == 0 {
		fmt.Fprintf(os.Stderr, "⚠️ Current directory load failed: %v\n", loadErr)

		// Fallback: try to load any Go files in the current directory
		fmt.Fprintf(os.Stderr, "🔄 Trying fallback: loading Go files\n")

		// List Go files in current directory and subdirectories
		goFiles, err := findGoFiles(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to find Go files: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "📁 Found Go files: %v\n", goFiles)

			if len(goFiles) > 0 {
				// Try to load by file patterns
				filePatterns := make([]string, len(goFiles))
				for i, f := range goFiles {
					filePatterns[i] = "file=" + f
				}
				pkgs, loadErr = packages.Load(cfg, filePatterns...)
				if loadErr == nil && len(pkgs) > 0 {
					fmt.Fprintf(os.Stderr, "✅ File pattern loading success: loaded %d packages\n", len(pkgs))
				} else {
					fmt.Fprintf(os.Stderr, "⚠️ File pattern loading failed: %v\n", loadErr)
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "✅ Current directory loading success: loaded %d packages\n", len(pkgs))
	}

	if len(pkgs) == 0 {
		fmt.Fprintf(os.Stderr, "❌ All loading strategies failed\n")
		// Generate empty result
		emptyResult := CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPackage,
			CallGraph:   make(map[string][]string),
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
		writeResult(outputFile, emptyResult)
		return
	}

	fmt.Fprintf(os.Stderr, "📊 Analysis found %d packages:\n", len(pkgs))
	for i, pkg := range pkgs {
		fmt.Fprintf(os.Stderr, "  %d. %s (files: %d, errors: %d)\n", i+1, pkg.PkgPath, len(pkg.GoFiles), len(pkg.Errors))
		if len(pkg.Errors) > 0 {
			for _, err := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "     Error: %v\n", err)
			}
		}
	}

	// Filter valid packages
	validPackages := filterValidPackages(pkgs)
	if len(validPackages) == 0 {
		fmt.Fprintf(os.Stderr, "❌ No valid packages for SSA analysis\n")
		// Generate empty result but don't fail
		emptyResult := CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPackage,
			CallGraph:   make(map[string][]string),
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
		writeResult(outputFile, emptyResult)
		return
	}

	fmt.Fprintf(os.Stderr, "✅ Using %d valid packages for SSA\n", len(validPackages))

	// Build SSA representation
	prog, _ := ssautil.AllPackages(validPackages, 0)
	prog.Build()

	ssaPackages := prog.AllPackages()
	fmt.Fprintf(os.Stderr, "🔨 Built SSA for %d packages\n", len(ssaPackages))

	// List functions found
	allFuncs := ssautil.AllFunctions(prog)
	fmt.Fprintf(os.Stderr, "🔍 Found %d total functions\n", len(allFuncs))

	// Build CHA call graph
	chaCG := cha.CallGraph(prog)
	chaCG.DeleteSyntheticNodes()

	// Build VTA call graph
	vtaCG := vta.CallGraph(allFuncs, chaCG)
	vtaCG.DeleteSyntheticNodes()

	fmt.Fprintf(os.Stderr, "🕸️ VTA call graph has %d nodes\n", len(vtaCG.Nodes))

	// Extract call relationships
	callGraph := make(map[string][]string)
	totalEdges := 0

	for fn, node := range vtaCG.Nodes {
		if fn == nil || node == nil {
			continue
		}

		var funcName string
		if fn.Pkg != nil && fn.Pkg.Pkg != nil {
			funcName = fmt.Sprintf("%s.%s", fn.Pkg.Pkg.Path(), fn.Name())
		} else {
			funcName = fn.Name()
		}

		var callees []string
		for _, edge := range node.Out {
			if edge == nil || edge.Callee == nil || edge.Callee.Func == nil {
				continue
			}

			var calleeName string
			if edge.Callee.Func.Pkg != nil && edge.Callee.Func.Pkg.Pkg != nil {
				calleeName = fmt.Sprintf("%s.%s", edge.Callee.Func.Pkg.Pkg.Path(), edge.Callee.Func.Name())
			} else {
				calleeName = edge.Callee.Func.Name()
			}
			callees = append(callees, calleeName)
			totalEdges++
		}

		if len(callees) > 0 {
			callGraph[funcName] = callees
		}
	}

	// Create result
	result := CallGraphResult{
		PackageID:   targetID,
		PackageName: targetName,
		ImportPath:  targetPackage,
		CallGraph:   callGraph,
		TotalFuncs:  len(callGraph),
		TotalEdges:  totalEdges,
		Algorithm:   "VTA",
	}

	fmt.Fprintf(os.Stderr, "📊 Final result: %d functions, %d edges\n", result.TotalFuncs, result.TotalEdges)

	writeResult(outputFile, result)
}

func setupGoEnvironment() {
	// Set up Go environment for sandbox
	if os.Getenv("GOROOT") == "" {
		if goroot := findGoRoot(); goroot != "" {
			os.Setenv("GOROOT", goroot)
		}
	}

	// Create a temporary go.mod if needed
	tempDir, _ := os.Getwd()
	goModPath := filepath.Join(tempDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		goModContent := "module temp\n\ngo 1.21\n"
		os.WriteFile(goModPath, []byte(goModContent), 0644)
	}
}

func buildGoEnvironment() []string {
	env := os.Environ()
	env = append(env, "GO111MODULE=on")
	env = append(env, "CGO_ENABLED=0")
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		env = append(env, "GOROOT="+goroot)
		env = append(env, "PATH="+goroot+"/bin:"+os.Getenv("PATH"))
	}
	return env
}

func filterValidPackages(pkgs []*packages.Package) []*packages.Package {
	var validPackages []*packages.Package
	for _, pkg := range pkgs {
		// Accept packages with syntax (source) OR types (stdlib)
		hasSyntax := pkg.Syntax != nil && len(pkg.Syntax) > 0
		hasTypes := pkg.Types != nil

		if hasSyntax || hasTypes {
			validPackages = append(validPackages, pkg)
		}
	}
	return validPackages
}

func findGoRoot() string {
	// Common Bazel Go SDK paths
	candidates := []string{
		"../external/go_sdk",
		"../../external/go_sdk",
		"../../../external/go_sdk",
		"../../../../external/go_sdk",
	}

	for _, candidate := range candidates {
		matches, _ := filepath.Glob(candidate)
		for _, match := range matches {
			if stat, err := os.Stat(filepath.Join(match, "bin", "go")); err == nil && !stat.IsDir() {
				return match
			}
		}
	}

	// Fallback to system GOROOT
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		return goroot
	}

	return ""
}

func findGoFiles(dir string) ([]string, error) {
	var goFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".go") && !strings.Contains(path, "vendor/") {
			goFiles = append(goFiles, path)
		}

		return nil
	})

	return goFiles, err
}

func writeResult(outputFile string, result CallGraphResult) {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Write result as JSON
	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal result: %v", err)
	}

	if err := os.WriteFile(outputFile, resultData, 0644); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}
}
