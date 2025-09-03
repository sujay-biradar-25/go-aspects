package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Node represents a dependency node in the JSON structure
type Node struct {
	OriginalLabel string `json:"original_label,omitempty"`
	// Add other fields as needed
}

// JSONData represents the structure of the JSON files
type JSONData struct {
	Nodes []json.RawMessage `json:"nodes"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: merge_json_deps <output_file> [input_files...]\n")
		os.Exit(1)
	}

	outputFile := os.Args[1]
	inputFiles := os.Args[2:]

	err := mergeJSONFiles(outputFile, inputFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func mergeJSONFiles(outputFile string, inputFiles []string) error {
	seenLabels := make(map[string]bool)
	var mergedNodes []json.RawMessage

	for _, inputFile := range inputFiles {
		// Check if file exists
		if _, err := os.Stat(inputFile); os.IsNotExist(err) {
			continue
		}

		// Read and parse JSON file
		data, err := ioutil.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to read %s: %v\n", inputFile, err)
			continue
		}

		var jsonData JSONData
		if err := json.Unmarshal(data, &jsonData); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to parse %s: %v\n", inputFile, err)
			continue
		}

		// Process each node
		for _, nodeData := range jsonData.Nodes {
			// Parse node to check for original_label
			var node map[string]interface{}
			if err := json.Unmarshal(nodeData, &node); err != nil {
				// If we can't parse it, just include it
				mergedNodes = append(mergedNodes, nodeData)
				continue
			}

			originalLabel, hasLabel := node["original_label"].(string)
			if hasLabel && originalLabel != "" {
				if !seenLabels[originalLabel] {
					seenLabels[originalLabel] = true
					mergedNodes = append(mergedNodes, nodeData)
				}
			} else {
				// No original_label, include it
				mergedNodes = append(mergedNodes, nodeData)
			}
		}
	}

	// Create result structure
	result := JSONData{Nodes: mergedNodes}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Write merged result
	resultData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %v", err)
	}

	if err := ioutil.WriteFile(outputFile, resultData, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %v", err)
	}

	return nil
}
