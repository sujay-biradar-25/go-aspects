package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	fmt.Println("🔍 Direct VTA Analysis Test")

	// Load packages for the main application
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedExportFile |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Tests: false,
	}

	// Load the main package directly
	pkgs, err := packages.Load(cfg, "./src/main")
	if err != nil {
		log.Fatalf("Failed to load packages: %v", err)
	}

	fmt.Printf("📦 Loaded %d packages\n", len(pkgs))
	for i, pkg := range pkgs {
		fmt.Printf("  Package %d: %s\n", i, pkg.PkgPath)
		fmt.Printf("    Files: %v\n", pkg.GoFiles)
		fmt.Printf("    Syntax nodes: %d\n", len(pkg.Syntax))
		fmt.Printf("    Types: %v\n", pkg.Types != nil)
		if len(pkg.Errors) > 0 {
			fmt.Printf("    Errors: %d\n", len(pkg.Errors))
			for _, e := range pkg.Errors {
				fmt.Printf("      - %v\n", e)
			}
		}
	}

	// Filter valid packages
	var validPkgs []*packages.Package
	for _, pkg := range pkgs {
		if pkg.Types != nil && len(pkg.Syntax) > 0 {
			validPkgs = append(validPkgs, pkg)
		}
	}

	if len(validPkgs) == 0 {
		log.Fatalf("No valid packages found for SSA analysis")
	}

	fmt.Printf("✅ Using %d valid packages for SSA\n", len(validPkgs))

	// Build SSA representation
	prog, _ := ssautil.AllPackages(validPkgs, 0)
	prog.Build()

	ssaPackages := prog.AllPackages()
	fmt.Printf("🔨 Built SSA for %d packages\n", len(ssaPackages))

	// List functions found
	allFuncs := ssautil.AllFunctions(prog)
	fmt.Printf("🔍 Found %d functions:\n", len(allFuncs))
	count := 0
	for fn := range allFuncs {
		if fn.Pkg != nil && fn.Pkg.Pkg != nil {
			fmt.Printf("  %s.%s\n", fn.Pkg.Pkg.Path(), fn.Name())
			count++
			if count >= 10 { // Show first 10 functions
				fmt.Printf("  ... and %d more\n", len(allFuncs)-10)
				break
			}
		}
	}

	// Build CHA call graph
	chaCG := cha.CallGraph(prog)
	chaCG.DeleteSyntheticNodes()

	// Build VTA call graph
	vtaCG := vta.CallGraph(allFuncs, chaCG)
	vtaCG.DeleteSyntheticNodes()

	fmt.Printf("🕸️ VTA call graph has %d nodes\n", len(vtaCG.Nodes))

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
	result := map[string]interface{}{
		"total_functions": len(callGraph),
		"total_edges":     totalEdges,
		"algorithm":       "VTA",
		"call_graph":      callGraph,
	}

	// Print results
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("📊 Results:\n%s\n", string(resultJSON))

	// Write to file
	if len(os.Args) > 1 {
		outputFile := os.Args[1]
		err = os.WriteFile(outputFile, resultJSON, 0644)
		if err != nil {
			log.Fatalf("Failed to write output: %v", err)
		}
		fmt.Printf("✅ Wrote results to %s\n", outputFile)
	}
}
