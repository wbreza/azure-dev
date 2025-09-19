// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BenchmarkJSONExtraction demonstrates the performance improvement of the optimized JSON extraction
func BenchmarkJSONExtraction() {
	// Test cases with different formats
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Direct JSON (optimized path)",
			input:    `{"message": "Hello", "reasoning": "Direct JSON"}`,
			expected: `{"message": "Hello", "reasoning": "Direct JSON"}`,
		},
		{
			name: "Markdown JSON code block (optimized path)",
			input: "```json\n" +
				`{"message": "Hello from markdown", "reasoning": "Code block"}` + "\n```",
			expected: `{"message": "Hello from markdown", "reasoning": "Code block"}`,
		},
		{
			name: "Text with embedded JSON (fallback path)",
			input: "Here's the response:\n" +
				`{"message": "Embedded JSON", "reasoning": "Mixed content"}` + "\nThat's it!",
			expected: `{"message": "Embedded JSON", "reasoning": "Mixed content"}`,
		},
	}

	fmt.Println("=== JSON Extraction Performance Test ===")

	for _, tc := range testCases {
		fmt.Printf("\nTest: %s\n", tc.name)
		fmt.Printf("Input: %s\n", truncateString(tc.input, 60))

		start := time.Now()
		result, err := extractJSONFromMarkdown(tc.input)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		// Verify the result is valid JSON
		var testJSON interface{}
		if err := json.Unmarshal([]byte(result), &testJSON); err != nil {
			fmt.Printf("❌ Invalid JSON result: %v\n", err)
			continue
		}

		fmt.Printf("✅ Success in %v\n", duration)
		fmt.Printf("Output: %s\n", truncateString(result, 60))
	}

	fmt.Println("\n=== Performance Optimization Summary ===")
	fmt.Println("🚀 First character check optimizations:")
	fmt.Println("  • '{' or '[' → Direct JSON parsing (fastest)")
	fmt.Println("  • '`' → Skip direct parsing, go straight to markdown extraction")
	fmt.Println("  • Other → Try direct parsing first, fallback to extraction")
	fmt.Println("  • Avoids expensive regex operations when not needed")
	fmt.Println("  • Reduces JSON unmarshaling attempts on large markdown responses")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// DemoOptimizationPaths shows which path each format takes
func DemoOptimizationPaths() {
	examples := []string{
		`{"direct": "json"}`,                     // Fast path: direct JSON
		"```json\n{\"markdown\": \"json\"}\n```", // Fast path: markdown detection
		"Here is: {\"embedded\": \"json\"}",      // Fallback path: text with JSON
		"```\n{\"generic\": \"codeblock\"}\n```", // Fallback path: generic code block
	}

	fmt.Println("=== Optimization Path Demo ===")
	for i, example := range examples {
		firstChar := strings.TrimSpace(example)[0]
		var path string

		switch firstChar {
		case '{', '[':
			path = "🚀 FAST: Direct JSON parsing"
		case '`':
			path = "⚡ FAST: Markdown extraction (skip JSON attempt)"
		default:
			path = "🔄 FALLBACK: Try JSON first, then extraction"
		}

		fmt.Printf("%d. %s\n", i+1, path)
		fmt.Printf("   Input: %s\n", truncateString(example, 40))
		fmt.Println()
	}
}
