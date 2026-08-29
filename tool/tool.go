package tool

import (
	"encoding/json"
	"errors"
	"github.com/agentengineering.dev/agent-framework/llm"
)

func ExecuteTool(name string, input json.RawMessage) (string, error) {
	return Execute(ToolMap, name, input)
}

// Execute runs a tool out of the given tool set, an agent that does no git work
// runs a different write_file than one that commits every change.
func Execute(tools map[string]llm.ToolDefinition, name string, input json.RawMessage) (string, error) {
	def, ok := tools[name]
	if !ok {
		return "", errors.New("Tool " + name + " not found")
	}
	return def.Func(input)
}

// ToolMap is the tool set that touches the filesystem only.
var ToolMap = map[string]llm.ToolDefinition{
	"list_files": ListFilesToolDefinition,
	"read_file":  ReadFileToolDefinition,
	"write_file": WriteFileToolDefinition,
}

// GitToolMap is the tool set for agents running on a branch, every write is
// committed. git_helpers.Init() must have been called before it is used.
var GitToolMap = map[string]llm.ToolDefinition{
	"list_files": ListFilesToolDefinition,
	"read_file":  ReadFileToolDefinition,
	"write_file": GitWriteFileToolDefinition,
}
