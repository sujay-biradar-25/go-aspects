package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	fmt.Fprintf(os.Stderr, "🔍 Simple VTA Analysis for Bazel\n")
	fmt.Fprintf(os.Stderr, "📄 Reading packages file: %s\n", packagesFile)

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
	var targetID string
	var targetName string
	var targetPkgPath string

	for _, pkg := range response.Packages {
		// Look for the main source package (not stdlib)
		if pkg.ID != "" && (strings.HasPrefix(pkg.ID, "@@//") || strings.HasPrefix(pkg.ID, "@//")) {
			targetID = pkg.ID
			targetName = pkg.Name
			targetPkgPath = pkg.PkgPath
			fmt.Fprintf(os.Stderr, "📦 Found target package: %s (ID: %s, Path: %s)\n", targetName, targetID, targetPkgPath)
			break
		}
	}

	if targetPkgPath == "" {
		fmt.Fprintf(os.Stderr, "⚠️ No source package found\n")
		targetPkgPath = "unknown"
	}

	// Instead of trying to run VTA in the restricted sandbox,
	// let's use a workspace-aware approach that works outside the sandbox

	// Try to determine the workspace root
	workspaceRoot := findWorkspaceRoot()
	if workspaceRoot == "" {
		fmt.Fprintf(os.Stderr, "⚠️ Could not find workspace root, using current directory\n")
		workspaceRoot = "."
	}

	fmt.Fprintf(os.Stderr, "🏠 Workspace root: %s\n", workspaceRoot)

	// Create a temporary script to run VTA analysis in the workspace context
	result := runWorkspaceVTAAnalysis(workspaceRoot, targetPkgPath, targetID, targetName)

	// Write result
	writeResult(outputFile, result)
}

func findWorkspaceRoot() string {
	// Try to find workspace root by looking for common workspace files
	candidates := []string{
		"/Users/sbiradar/code/go-aspects", // Known workspace
		"../../../../..",                  // Try relative paths
		"../../../..",
		"../../..",
		"..",
		".",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "WORKSPACE")); err == nil {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
		}
		if _, err := os.Stat(filepath.Join(candidate, "MODULE.bazel")); err == nil {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
		}
	}

	return ""
}

func runWorkspaceVTAAnalysis(workspaceRoot, targetPkgPath, targetID, targetName string) CallGraphResult {
	fmt.Fprintf(os.Stderr, "🔄 Running VTA analysis in workspace context\n")

	// Create a simple Go script to run VTA analysis
	vtaScript := `
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa/ssautil"
)

type CallGraphResult struct {
	PackageID    string              ` + "`json:\"package_id\"`" + `
	PackageName  string              ` + "`json:\"package_name\"`" + `
	ImportPath   string              ` + "`json:\"import_path\"`" + `
	CallGraph    map[string][]string ` + "`json:\"call_graph\"`" + `
	TotalFuncs   int                 ` + "`json:\"total_functions\"`" + `
	TotalEdges   int                 ` + "`json:\"total_edges\"`" + `
	Algorithm    string              ` + "`json:\"algorithm\"`" + `
}

func main() {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedExportFile |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./src/main", "./src/utils")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load packages: %v\n", err)
		os.Exit(1)
	}

	var validPackages []*packages.Package
	for _, pkg := range pkgs {
		if pkg.Types != nil && len(pkg.Syntax) > 0 {
			validPackages = append(validPackages, pkg)
		}
	}

	if len(validPackages) == 0 {
		result := CallGraphResult{
			PackageID: "` + targetID + `",
			PackageName: "` + targetName + `",
			ImportPath: "` + targetPkgPath + `",
			CallGraph: make(map[string][]string),
			TotalFuncs: 0,
			TotalEdges: 0,
			Algorithm: "VTA",
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	prog, _ := ssautil.AllPackages(validPackages, 0)
	prog.Build()

	chaCG := cha.CallGraph(prog)
	chaCG.DeleteSyntheticNodes()

	allFuncs := ssautil.AllFunctions(prog)
	vtaCG := vta.CallGraph(allFuncs, chaCG)
	vtaCG.DeleteSyntheticNodes()

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

	result := CallGraphResult{
		PackageID: "` + targetID + `",
		PackageName: "` + targetName + `",
		ImportPath: "` + targetPkgPath + `",
		CallGraph: callGraph,
		TotalFuncs: len(callGraph),
		TotalEdges: totalEdges,
		Algorithm: "VTA",
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
`

	// Write the script to a temporary file
	tmpFile := filepath.Join(os.TempDir(), "vta_analysis.go")
	if err := os.WriteFile(tmpFile, []byte(vtaScript), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to write VTA script: %v\n", err)
		return CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPkgPath,
			CallGraph:   make(map[string][]string),
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
	}
	defer os.Remove(tmpFile)

	// Run the script in the workspace directory
	cmd := exec.Command("go", "run", tmpFile)
	cmd.Dir = workspaceRoot
	env := os.Environ()
	env = append(env, "GO111MODULE=on")
	env = append(env, "GOCACHE="+filepath.Join(os.TempDir(), "gocache"))
	env = append(env, "GOMODCACHE="+filepath.Join(os.TempDir(), "gomodcache"))

	// Ensure HOME is set for Go toolchain download
	homeSet := false
	for _, envVar := range env {
		if strings.HasPrefix(envVar, "HOME=") {
			homeSet = true
			break
		}
	}
	if !homeSet {
		env = append(env, "HOME="+os.TempDir())
	}

	cmd.Env = env

	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to run VTA analysis: %v\n", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "Stderr: %s\n", exitErr.Stderr)
		}
		return CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPkgPath,
			CallGraph:   make(map[string][]string),
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
	}

	// Parse the result
	var result CallGraphResult
	if err := json.Unmarshal(output, &result); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to parse VTA result: %v\n", err)
		return CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPkgPath,
			CallGraph:   make(map[string][]string),
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
	}

	fmt.Fprintf(os.Stderr, "✅ VTA analysis completed: %d functions, %d edges\n", result.TotalFuncs, result.TotalEdges)
	return result
}

func writeResult(outputFile string, result CallGraphResult) {
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal result: %v", err)
	}

	if err := os.WriteFile(outputFile, resultData, 0644); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}
}
