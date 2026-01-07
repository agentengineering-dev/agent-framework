package tool

import (
	"encoding/json"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/git_helpers"
	"github.com/agentengineering.dev/agent-framework/llm"
	"os"
	"path/filepath"
	"strings"
)

// region list_files
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
