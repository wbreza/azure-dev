// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"encoding/json"
	"regexp"
	"strings"
)

// extractJSONFromMarkdown extracts JSON content from markdown-formatted responses.
// It handles cases where the LLM returns JSON wrapped in markdown code blocks like:
// ```json
// { "key": "value" }
// ```
func extractJSONFromMarkdown(response string) (string, error) {
	response = strings.TrimSpace(response)
	if len(response) == 0 {
		return "", nil
	}

	// Fast path: check first character to determine format
	firstChar := response[0]

	switch firstChar {
	case '{', '[':
		// Looks like direct JSON - try to unmarshal directly
		var testJSON interface{}
		if err := json.Unmarshal([]byte(response), &testJSON); err == nil {
			return response, nil
		}
		// If direct JSON parsing failed, fall through to markdown extraction

	case '`':
		// Looks like markdown code block - skip direct JSON attempt and go straight to extraction
		return extractFromMarkdownCodeBlocks(response)

	default:
		// Could be text with embedded JSON - try direct JSON first, then extraction
		var testJSON interface{}
		if err := json.Unmarshal([]byte(response), &testJSON); err == nil {
			return response, nil
		}
	}

	// Fallback to markdown extraction for all cases
	return extractFromMarkdownCodeBlocks(response)
}

// extractFromMarkdownCodeBlocks handles the regex-based extraction from markdown
func extractFromMarkdownCodeBlocks(response string) (string, error) {
	var testJSON interface{}

	// Look for JSON code blocks with various patterns
	patterns := []string{
		// Standard markdown JSON code block
		"```json\\s*\\n([\\s\\S]*?)\\n```",
		// Code block without language specifier
		"```\\s*\\n([\\s\\S]*?)\\n```",
		// Inline code with json prefix
		"`json\\s*([\\s\\S]*?)`",
		// Simple backticks
		"`([\\s\\S]*?)`",
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(response)
		if len(matches) > 1 {
			candidate := strings.TrimSpace(matches[1])

			// Test if the extracted content is valid JSON
			if err := json.Unmarshal([]byte(candidate), &testJSON); err == nil {
				return candidate, nil
			}
		}
	}

	// If no code blocks found, try to find JSON-like content in the response
	// Look for content that starts with { and ends with }
	startIdx := strings.Index(response, "{")
	if startIdx != -1 {
		// Find the matching closing brace
		braceCount := 0
		endIdx := -1
		for i := startIdx; i < len(response); i++ {
			switch response[i] {
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					endIdx = i
					break
				}
			}
		}

		if endIdx != -1 {
			candidate := strings.TrimSpace(response[startIdx : endIdx+1])

			// Test if the extracted content is valid JSON
			if err := json.Unmarshal([]byte(candidate), &testJSON); err == nil {
				return candidate, nil
			}
		}
	}

	// If all else fails, return the original response
	return response, nil
}

// unmarshalJSONResponse is a helper function that extracts JSON from markdown-formatted
// responses and unmarshals it into the provided interface
func unmarshalJSONResponse(response string, v interface{}) error {
	jsonContent, err := extractJSONFromMarkdown(response)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(jsonContent), v)
}
