// region imports
/*
Imports standard libraries for file handling, JSON, strings, and random bytes,
third-party library for generating diffs,
and internal packages for git operations and agent tool integration.
*/
package tool

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/git_helpers"
	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/pmezard/go-difflib/difflib"
	"os"
	"path/filepath"
	"strings"
)

// endregion

// region list_files
/*
Defines a tool that lists all files and directories in a given path.
Returns a newline-separated string, appending "/" to directory names.
*/
var ListFilesToolDefinition = llm.ToolDefinition{
	Name:                "list_files",
	Description:         "Returns a list of files in the given directory.",
	InputSchemaInstance: ListFilesInput{},
	Func:                ListFileImpl,
}

type ListFilesInput struct {
	Directory string `json:"directory" jsonschema_description:"Path of the directory"`
}

var ListFileImpl = func(message json.RawMessage) (string, error) {
	var input ListFilesInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(input.Directory)
	if err != nil {
		return "", fmt.Errorf("error reading directory: %w", err)
	}

	var files []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}
	return strings.Join(files, "\n"), nil

}

// endregion

// region read_file
/*
Defines a tool that reads the contents of a specified file,
blocks reading sensitive files like ".env", and returns the content as a string.
*/
var ReadFileToolDefinition = llm.ToolDefinition{
	Name:                "read_file",
	Description:         "Reads a file of the given path.",
	InputSchemaInstance: ReadFileInput{},
	Func:                ReadFileImpl,
}

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The path to the file"`
}

var ReadFileImpl = func(message json.RawMessage) (string, error) {
	var input ReadFileInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}
	path := input.Path
	if path == ".env" {
		return "", fmt.Errorf(".env file is not allowed to be read")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}
	return string(data), nil
}

// endregion

// region write_file
/*
Defines a tool that writes content to a specified file, creating directories if needed,
and commits the change to git with a provided commit message.
*/
var WriteFileToolDefinition = llm.ToolDefinition{
	Name:                "write_file",
	Description:         "Writes a file of the given path relative to the root project directory.",
	InputSchemaInstance: WriteFileInput{},
	Func:                WriteFileImpl,
}

type WriteFileInput struct {
	Path          string `json:"path" jsonschema_description:"The path to the file relative to the root project directory."`
	Content       string `json:"content" jsonschema_description:"Content of the file"`
	CommitMessage string `json:"commit_message" jsonschema_description:"Commit message of the file"`
}

func WriteFileImpl(message json.RawMessage) (string, error) {
	var input WriteFileInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}
	path := input.Path

	err := os.MkdirAll(filepath.Dir(path), 0777)
	if err != nil {
		return "", fmt.Errorf("error creating directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("error creating file: %w", err)
	}

	defer file.Close()
	_, err = file.WriteString(input.Content)
	if err != nil {
		return "", fmt.Errorf("error writing file: %w", err)
	}

	err = git_helpers.AddAllAndCommit(input.CommitMessage, "agent-framework", "agent-framework@sanap.io")
	if err != nil {
		return "", fmt.Errorf("error write file: %w", err)
	}
	return "Successfully create file: " + file.Name(), nil

}

// endregion

// region edit_file
/*
Defines a tool that edits a file by replacing text via exact or fuzzy matching,
preserves indentation, generates a unified diff, supports dry runs,
and safely writes changes to disk.
*/
var EditFileDefinition = llm.ToolDefinition{
	Name: "edit_file",
	Description: `Make edit to a text file.

Supports edit per file. Each edit replaces 'old_str' with 'new_str'. By default, each replacement applies to the first exact match found in the file.

If no exact match is found, the system falls back to a line-by-line fuzzy match that compares lines using normalized whitespace. Original indentation is preserved where possible.

If the file doesn't exist, it will be created with the 'new_str' of the first edit (only if 'old_str' is empty).

If dryRun is true, the file won't be changed, but a unified diff of the proposed result will be returned.`,
	InputSchemaInstance: EditFileInput{},
	Func:                EditFile,
}

type Edit struct {
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

type EditFileInput struct {
	Path   string `json:"path"`
	Edit   Edit   `json:"edits"`
	DryRun bool   `json:"dryRun,omitempty"`
}

func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func generateTempFilePath(originalPath string) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	ext := filepath.Ext(originalPath)
	base := strings.TrimSuffix(originalPath, ext)
	return fmt.Sprintf("%s.%s.tmp%s", base, hex.EncodeToString(randomBytes), ext), nil
}

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func EditFile(rawInput json.RawMessage) (string, error) {
	var input EditFileInput
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return "", err
	}

	if input.Path == "" {
		return "", fmt.Errorf("invalid input: file path is required")
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(input.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	// Ensure file exists (create empty if not)
	if _, err := os.Stat(input.Path); os.IsNotExist(err) {
		if err := os.WriteFile(input.Path, []byte(""), 0644); err != nil {
			return "", fmt.Errorf("failed to create file: %w", err)
		}
	}

	contentBytes, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}
	original := normalizeLineEndings(string(contentBytes))
	modified := original

	oldText := normalizeLineEndings(input.Edit.OldStr)
	newText := normalizeLineEndings(input.Edit.NewStr)

	// Try direct replacement
	if strings.Contains(modified, oldText) {
		modified = strings.Replace(modified, oldText, newText, 1)
	} else {
		// Fallback to fuzzy matching line-by-line
		modLines := strings.Split(modified, "\n")
		oldLines := strings.Split(oldText, "\n")
		found := false

		for i := 0; i <= len(modLines)-len(oldLines); i++ {
			match := true
			for j := 0; j < len(oldLines); j++ {
				if strings.TrimSpace(modLines[i+j]) != strings.TrimSpace(oldLines[j]) {
					match = false
					break
				}
			}
			if match {
				// Preserve indentation
				firstIndent := leadingWhitespace(modLines[i])
				newLines := strings.Split(newText, "\n")
				for k := range newLines {
					newLines[k] = firstIndent + strings.TrimLeft(newLines[k], " \t")
				}
				modLines = append(modLines[:i], append(newLines, modLines[i+len(oldLines):]...)...)
				modified = strings.Join(modLines, "\n")
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("could not find exact match for:\n%s", input.Edit.OldStr)
		}
	}

	if original == modified {
		return "", errors.New("no changes made")
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(modified),
		FromFile: input.Path,
		ToFile:   input.Path,
		Context:  3,
	}
	diffText, _ := difflib.GetUnifiedDiffString(diff)

	numBackticks := 3
	for strings.Contains(diffText, strings.Repeat("`", numBackticks)) {
		numBackticks++
	}
	formattedDiff := fmt.Sprintf("%sdiff\n%s%s\n\n", strings.Repeat("`", numBackticks), diffText, strings.Repeat("`", numBackticks))

	if input.DryRun {
		return formattedDiff, nil
	}

	tmpPath, err := generateTempFilePath(input.Path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(tmpPath, []byte(modified), 0644); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, input.Path); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return formattedDiff, nil
}

// endregion
