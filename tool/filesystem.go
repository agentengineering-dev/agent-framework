// region imports
/*
Imports standard libraries for file handling, JSON, strings, and random bytes,
third-party library for generating diffs,
and internal packages for git operations and agent tool integration.
*/
package tool

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/git_helpers"
	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/pmezard/go-difflib/difflib"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// endregion

// region list_files
/*
Defines a tool that lists all files and directories in a given path.
Returns a newline-separated string, appending "/" to directory names and the
size of each file so the agent can tell a stub from a file worth a read_file
window before it spends the call.
*/
var ListFilesToolDefinition = llm.ToolDefinition{
	Name:                "list_files",
	Description:         "Returns a list of files in the given directory, with the size of each file. Directory names end in \"/\" and carry no size.",
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
			files = append(files, name+"/")
			continue
		}
		// a size we cannot read, a broken symlink say, is not worth failing
		// the whole listing over. the name still tells the agent something.
		info, err := entry.Info()
		if err != nil {
			files = append(files, name)
			continue
		}
		files = append(files, fmt.Sprintf("%s (%s)", name, humanBytes(info.Size())))
	}
	return strings.Join(files, "\n"), nil

}

// humanBytes renders a size the way ls -h does, in units the model reads
// without doing arithmetic on a ten digit number.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB", "PB"} {
		value /= unit
		if value < unit {
			// one decimal up to 10, none above it. 4.2 KB is worth knowing,
			// the .3 of 512.3 KB is noise.
			if value < 10 {
				return fmt.Sprintf("%.1f %s", value, suffix)
			}
			return fmt.Sprintf("%.0f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.0f EB", value/unit)
}

// endregion

// region read_file
/*
Defines a tool that reads the contents of a specified file,
blocks reading sensitive files like ".env", and returns the content as a string.
Reads a window of lines rather than the whole file so a large file can be
walked in pieces instead of blowing up the context in one call.
*/
var ReadFileToolDefinition = llm.ToolDefinition{
	Name: "read_file",
	Description: `Reads a file of the given path.

Output is line numbered, "<line number>\t<content>", so a line number read here can be passed straight back as 'offset'.

Reads a window of the file: 'offset' is the first line to read (1 based) and 'limit' is how many lines to read. Leave both at 0 to read the file from the start; a file longer than the read limit is cut off and the result says how many lines are left, so read the rest with a follow up call at the next offset.`,
	InputSchemaInstance: ReadFileInput{},
	Func:                ReadFileImpl,
}

const (
	// defaultReadLineLimit is how much of a file one call returns when the
	// caller does not ask for a window. Big enough for most source files,
	// small enough that reading a log file does not end the session.
	defaultReadLineLimit = 2000
	// maxReadLineWidth is where a single long line gets cut. Minified files
	// are one line, and that line is not worth the whole context.
	maxReadLineWidth = 2000
)

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The path to the file"`
	// Offset and Limit are plain ints rather than pointers, 0 is the unset
	// value. Some providers want every property to be required, so the model
	// sends both on every call whether it cares about them or not.
	Offset int `json:"offset" jsonschema_description:"The line to start reading from, 1 based. 0 reads from the start of the file."`
	Limit  int `json:"limit" jsonschema_description:"How many lines to read starting at offset. 0 reads to the end of the file, capped at 2000 lines."`
}

var ReadFileImpl = func(message json.RawMessage) (string, error) {
	var input ReadFileInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}
	path := input.Path
	if filepath.Base(path) == ".env" {
		return "", fmt.Errorf(".env file is not allowed to be read")
	}
	if input.Offset < 0 || input.Limit < 0 {
		return "", fmt.Errorf("offset and limit cannot be negative")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}
	defer file.Close()

	if info, err := file.Stat(); err == nil && info.IsDir() {
		return "", fmt.Errorf("error reading file: %s is a directory", path)
	}

	return readWindow(file, input.Offset, input.Limit)
}

// readWindow returns the requested lines, numbered, plus a note about what was
// left unread so the caller knows there is more file to ask for.
func readWindow(r io.Reader, offset, limit int) (string, error) {
	first := offset
	if first < 1 {
		first = 1
	}
	if limit == 0 {
		limit = defaultReadLineLimit
	}

	var out strings.Builder
	lineNo := 0
	shown := 0
	remaining := 0

	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if line == "" && err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error reading file: %w", err)
		}
		lineNo++
		if lineNo >= first && shown < limit {
			out.WriteString(fmt.Sprintf("%d\t%s\n", lineNo, truncateLine(strings.TrimRight(line, "\r\n"))))
			shown++
		} else if lineNo > first {
			remaining++
		}
		if err != nil {
			// last line of a file with no trailing newline
			break
		}
	}

	if lineNo == 0 {
		return "(file is empty)", nil
	}
	if shown == 0 {
		return "", fmt.Errorf("offset %d is past the end of the file, it has %d lines", first, lineNo)
	}
	if remaining > 0 {
		out.WriteString(fmt.Sprintf("\n(read lines %d-%d of %d, %d lines not read, continue at offset %d)",
			first, first+shown-1, lineNo, remaining, first+shown))
	}
	return out.String(), nil
}

// truncateLine keeps one absurdly long line, a minified bundle say, from
// taking the space the rest of the read was meant to have.
func truncateLine(line string) string {
	if len(line) <= maxReadLineWidth {
		return line
	}
	return line[:maxReadLineWidth] + fmt.Sprintf("... (line truncated, %d bytes total)", len(line))
}

// endregion

// region write_file
/*
Defines a tool that writes content to a specified file, creating directories if needed.
The git flavour of the tool also commits the change with a provided commit message.
*/
var WriteFileToolDefinition = llm.ToolDefinition{
	Name:                "write_file",
	Description:         "Writes a file of the given path relative to the root project directory.",
	InputSchemaInstance: WriteFileInput{},
	Func:                WriteFileImpl,
}

var GitWriteFileToolDefinition = llm.ToolDefinition{
	Name:                "write_file",
	Description:         "Writes a file of the given path relative to the root project directory and commits it.",
	InputSchemaInstance: GitWriteFileInput{},
	Func:                GitWriteFileImpl,
}

type WriteFileInput struct {
	Path    string `json:"path" jsonschema_description:"The path to the file relative to the root project directory."`
	Content string `json:"content" jsonschema_description:"Content of the file"`
}

type GitWriteFileInput struct {
	Path          string `json:"path" jsonschema_description:"The path to the file relative to the root project directory."`
	Content       string `json:"content" jsonschema_description:"Content of the file"`
	CommitMessage string `json:"commit_message" jsonschema_description:"Commit message of the file"`
}

func WriteFileImpl(message json.RawMessage) (string, error) {
	var input WriteFileInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}
	return writeFile(input.Path, input.Content)
}

func GitWriteFileImpl(message json.RawMessage) (string, error) {
	var input GitWriteFileInput
	if err := json.Unmarshal(message, &input); err != nil {
		return "", err
	}

	result, err := writeFile(input.Path, input.Content)
	if err != nil {
		return "", err
	}

	err = git_helpers.AddAllAndCommit(input.CommitMessage, "agent-framework", "agent-framework@sanap.io")
	if err != nil {
		return "", fmt.Errorf("error write file: %w", err)
	}
	return result, nil
}

func writeFile(path string, content string) (string, error) {
	err := os.MkdirAll(filepath.Dir(path), 0777)
	if err != nil {
		return "", fmt.Errorf("error creating directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("error creating file: %w", err)
	}

	defer file.Close()
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("error writing file: %w", err)
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
