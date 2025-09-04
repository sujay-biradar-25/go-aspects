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

type FunctionInfo struct {
	Name       string   `json:"name"`
	Package    string   `json:"package"`
	Signature  string   `json:"signature"`
	Parameters []string `json:"parameters"`
	Returns    []string `json:"returns"`
}

type CallEdge struct {
	Caller FunctionInfo `json:"caller"`
	Callee FunctionInfo `json:"callee"`
}

type CallGraphResult struct {
	PackageID   string                  `json:"package_id"`
	PackageName string                  `json:"package_name"`
	ImportPath  string                  `json:"import_path"`
	CallGraph   map[string][]string     `json:"call_graph"` // Legacy format
	Functions   map[string]FunctionInfo `json:"functions"`  // Function signatures
	CallEdges   []CallEdge              `json:"call_edges"` // Enhanced call relationships
	TotalFuncs  int                     `json:"total_functions"`
	TotalEdges  int                     `json:"total_edges"`
	Algorithm   string                  `json:"algorithm"`
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
	result := runWorkspaceVTAAnalysis(workspaceRoot, packagesFile, targetPkgPath, targetID, targetName)

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

func runWorkspaceVTAAnalysis(workspaceRoot, packagesFile, targetPkgPath, targetID, targetName string) CallGraphResult {
	fmt.Fprintf(os.Stderr, "🔄 Running VTA analysis in workspace context\n")

	// Create a dynamic Go script to run VTA analysis with runtime environment detection
	vtaScript := `
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type FunctionInfo struct {
	Name       string   ` + "`json:\"name\"`" + `
	Package    string   ` + "`json:\"package\"`" + `
	Signature  string   ` + "`json:\"signature\"`" + `
	Parameters []string ` + "`json:\"parameters\"`" + `
	Returns    []string ` + "`json:\"returns\"`" + `
}

type CallEdge struct {
	Caller FunctionInfo ` + "`json:\"caller\"`" + `
	Callee FunctionInfo ` + "`json:\"callee\"`" + `
}

type CallGraphResult struct {
	PackageID     string                     ` + "`json:\"package_id\"`" + `
	PackageName   string                     ` + "`json:\"package_name\"`" + `
	ImportPath    string                     ` + "`json:\"import_path\"`" + `
	CallGraph     map[string][]string        ` + "`json:\"call_graph\"`" + `
	Functions     map[string]FunctionInfo    ` + "`json:\"functions\"`" + `
	CallEdges     []CallEdge                 ` + "`json:\"call_edges\"`" + `
	TotalFuncs    int                        ` + "`json:\"total_functions\"`" + `
	TotalEdges    int                        ` + "`json:\"total_edges\"`" + `
	Algorithm     string                     ` + "`json:\"algorithm\"`" + `
}

func main() {
	// Read the packages JSON metadata from stdin or first argument
	var responseData []byte
	var err error
	
	if len(os.Args) > 1 {
		// Read from file if argument provided
		responseData, err = os.ReadFile(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read packages file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Usage: program <packages.json>\n")
		os.Exit(1)
	}
	
	// Parse the packages JSON response from Bazel
	var response struct {
		Roots    []string ` + "`json:\"Roots\"`" + `
		Packages []struct {
			ID      string   ` + "`json:\"ID\"`" + `
			Name    string   ` + "`json:\"Name\"`" + `
			PkgPath string   ` + "`json:\"PkgPath\"`" + `
			GoFiles []string ` + "`json:\"GoFiles\"`" + `
		} ` + "`json:\"Packages\"`" + `
	}
	
	if err := json.Unmarshal(responseData, &response); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse packages JSON: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Fprintf(os.Stderr, "🎯 Target roots: %v\n", response.Roots)

	// Detect runtime environment
	goarch := runtime.GOARCH
	goos := runtime.GOOS
	fmt.Fprintf(os.Stderr, "🏗️ Detected Go environment: %s/%s\n", goos, goarch)
	
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedExportFile |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Tests: false,
		Env: append(os.Environ(), 
			"GOOS=" + goos,
			"GOARCH=" + goarch,
		),
	}

	// Extract package paths from the JSON metadata provided by Bazel
	packagePaths := make([]string, 0)
	for _, pkg := range response.Packages {
		if pkg.ID != "" && (strings.HasPrefix(pkg.ID, "@@//") || strings.HasPrefix(pkg.ID, "@//")) {
			// Convert Bazel package path to Go module path
			pkgPath := "./" + pkg.PkgPath
			packagePaths = append(packagePaths, pkgPath)
			fmt.Fprintf(os.Stderr, "📦 Will analyze package: %s (from %s)\n", pkgPath, pkg.ID)
		}
	}
	
	// If no packages found, fallback to current directory
	if len(packagePaths) == 0 {
		packagePaths = []string{"."}
		fmt.Fprintf(os.Stderr, "⚠️ No packages found in Bazel context, using current directory\n")
	}
	
	fmt.Fprintf(os.Stderr, "🔄 Loading %d packages: %v\n", len(packagePaths), packagePaths)
	pkgs, err := packages.Load(cfg, packagePaths...)
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

	// Helper function to extract function signature
	extractFunctionInfo := func(fn *ssa.Function) FunctionInfo {
		if fn == nil {
			return FunctionInfo{Name: "unknown", Package: "unknown", Signature: "unknown()"}
		}
		
		var funcName, pkgPath string
		if fn.Pkg != nil && fn.Pkg.Pkg != nil {
			pkgPath = fn.Pkg.Pkg.Path()
			funcName = fmt.Sprintf("%s.%s", pkgPath, fn.Name())
		} else {
			funcName = fn.Name()
			pkgPath = "builtin"
		}
		
		// Extract parameters and return types
		var parameters, returns []string
		signature := ""
		
		if fn.Signature != nil {
			sig := fn.Signature
			
			// Extract parameters
			if sig.Params() != nil {
				for i := 0; i < sig.Params().Len(); i++ {
					param := sig.Params().At(i)
					paramType := param.Type().String()
					paramName := param.Name()
					if paramName != "" {
						parameters = append(parameters, fmt.Sprintf("%s %s", paramName, paramType))
					} else {
						parameters = append(parameters, paramType)
					}
				}
			}
			
			// Extract return types
			if sig.Results() != nil {
				for i := 0; i < sig.Results().Len(); i++ {
					result := sig.Results().At(i)
					resultType := result.Type().String()
					resultName := result.Name()
					if resultName != "" {
						returns = append(returns, fmt.Sprintf("%s %s", resultName, resultType))
					} else {
						returns = append(returns, resultType)
					}
				}
			}
			
			// Build full signature
			paramStr := strings.Join(parameters, ", ")
			returnStr := ""
			if len(returns) == 1 {
				returnStr = returns[0]
			} else if len(returns) > 1 {
				returnStr = "(" + strings.Join(returns, ", ") + ")"
			}
			
			if returnStr != "" {
				signature = fmt.Sprintf("%s(%s) %s", fn.Name(), paramStr, returnStr)
			} else {
				signature = fmt.Sprintf("%s(%s)", fn.Name(), paramStr)
			}
		} else {
			signature = fn.Name() + "()"
		}
		
		return FunctionInfo{
			Name:       funcName,
			Package:    pkgPath,
			Signature:  signature,
			Parameters: parameters,
			Returns:    returns,
		}
	}

	callGraph := make(map[string][]string)
	functions := make(map[string]FunctionInfo)
	var callEdges []CallEdge
	totalEdges := 0

	for fn, node := range vtaCG.Nodes {
		if fn == nil || node == nil {
			continue
		}

		callerInfo := extractFunctionInfo(fn)
		functions[callerInfo.Name] = callerInfo

		var callees []string
		for _, edge := range node.Out {
			if edge == nil || edge.Callee == nil || edge.Callee.Func == nil {
				continue
			}

			calleeInfo := extractFunctionInfo(edge.Callee.Func)
			functions[calleeInfo.Name] = calleeInfo
			
			callees = append(callees, calleeInfo.Name)
			callEdges = append(callEdges, CallEdge{
				Caller: callerInfo,
				Callee: calleeInfo,
			})
			totalEdges++
		}

		if len(callees) > 0 {
			callGraph[callerInfo.Name] = callees
		}
	}

	result := CallGraphResult{
		PackageID: "` + targetID + `",
		PackageName: "` + targetName + `",
		ImportPath: "` + targetPkgPath + `",
		CallGraph: callGraph,
		Functions: functions,
		CallEdges: callEdges,
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

	// Convert packages file to absolute path since we're changing working directory
	absPackagesFile, err := filepath.Abs(packagesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get absolute path for packages file: %v\n", err)
		return CallGraphResult{
			PackageID:   targetID,
			PackageName: targetName,
			ImportPath:  targetPkgPath,
			CallGraph:   make(map[string][]string),
			Functions:   make(map[string]FunctionInfo),
			CallEdges:   []CallEdge{},
			TotalFuncs:  0,
			TotalEdges:  0,
			Algorithm:   "VTA",
		}
	}

	// Run the script in the workspace directory with dynamic Go environment detection
	cmd := exec.Command("go", "run", tmpFile, absPackagesFile)
	cmd.Dir = workspaceRoot

	// Build environment with runtime Go detection
	env := buildDynamicGoEnvironment()
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

func buildDynamicGoEnvironment() []string {
	env := os.Environ()

	// Detect Go environment at runtime
	goVersion, _ := exec.Command("go", "version").Output()
	fmt.Fprintf(os.Stderr, "🔧 Go version: %s", string(goVersion))

	// Get Go environment variables dynamically
	if goroot, err := exec.Command("go", "env", "GOROOT").Output(); err == nil {
		env = append(env, "GOROOT="+strings.TrimSpace(string(goroot)))
		fmt.Fprintf(os.Stderr, "🏠 GOROOT: %s\n", strings.TrimSpace(string(goroot)))
	}

	if gopath, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
		env = append(env, "GOPATH="+strings.TrimSpace(string(gopath)))
		fmt.Fprintf(os.Stderr, "📁 GOPATH: %s\n", strings.TrimSpace(string(gopath)))
	}

	if goos, err := exec.Command("go", "env", "GOOS").Output(); err == nil {
		env = append(env, "GOOS="+strings.TrimSpace(string(goos)))
		fmt.Fprintf(os.Stderr, "🖥️ GOOS: %s\n", strings.TrimSpace(string(goos)))
	}

	if goarch, err := exec.Command("go", "env", "GOARCH").Output(); err == nil {
		env = append(env, "GOARCH="+strings.TrimSpace(string(goarch)))
		fmt.Fprintf(os.Stderr, "🏗️ GOARCH: %s\n", strings.TrimSpace(string(goarch)))
	}

	if gomod, err := exec.Command("go", "env", "GOMODCACHE").Output(); err == nil {
		modCache := strings.TrimSpace(string(gomod))
		if modCache != "" {
			env = append(env, "GOMODCACHE="+modCache)
			fmt.Fprintf(os.Stderr, "📦 GOMODCACHE: %s\n", modCache)
		}
	} else {
		// Fallback
		env = append(env, "GOMODCACHE="+filepath.Join(os.TempDir(), "gomodcache"))
	}

	if gocache, err := exec.Command("go", "env", "GOCACHE").Output(); err == nil {
		cache := strings.TrimSpace(string(gocache))
		if cache != "" && cache != "off" {
			env = append(env, "GOCACHE="+cache)
			fmt.Fprintf(os.Stderr, "🗂️ GOCACHE: %s\n", cache)
		} else {
			// Cache is disabled, enable it with temp directory
			tempCache := filepath.Join(os.TempDir(), "vta-gocache")
			env = append(env, "GOCACHE="+tempCache)
			fmt.Fprintf(os.Stderr, "🗂️ GOCACHE: %s (temporary, was %s)\n", tempCache, cache)
		}
	} else {
		// Fallback
		tempCache := filepath.Join(os.TempDir(), "vta-gocache")
		env = append(env, "GOCACHE="+tempCache)
		fmt.Fprintf(os.Stderr, "🗂️ GOCACHE: %s (fallback)\n", tempCache)
	}

	env = append(env, "GO111MODULE=on")

	// Ensure HOME is set
	homeSet := false
	for _, envVar := range env {
		if strings.HasPrefix(envVar, "HOME=") {
			homeSet = true
			break
		}
	}
	if !homeSet {
		if userHome, err := os.UserHomeDir(); err == nil {
			env = append(env, "HOME="+userHome)
		} else {
			env = append(env, "HOME="+os.TempDir())
		}
	}

	return env
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
