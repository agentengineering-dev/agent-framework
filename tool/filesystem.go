package tool

import (
	"encoding/json"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/llm"
	"os"
	"strings"
)

// region list_files
var ListFilesToolDefinition = llm.ToolDefinition{
	Name:                "list_files",
	Description:         "Returns a list of files in the current directory.",
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

var ToolMap = map[string]llm.ToolDefinition{
	"list_files": ListFilesToolDefinition,
}
