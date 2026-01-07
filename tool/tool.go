package tool

import (
	"encoding/json"
	"errors"
	"github.com/agentengineering.dev/agent-framework/llm"
)

func ExecuteTool(name string, input json.RawMessage) (string, error) {
	def, ok := ToolMap[name]
	if !ok {
		return "", errors.New("Tool " + name + " not found")
	}
	return def.Func(input)
}

var ToolMap = map[string]llm.ToolDefinition{
	"list_files": ListFilesToolDefinition,
	"read_file":  ReadFileToolDefinition,
	"write_file": WriteFileToolDefinition,
}
