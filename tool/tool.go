package tool

import (
	"encoding/json"
	"errors"
)

func ExecuteTool(name string, input json.RawMessage) (string, error) {
	def, ok := ToolMap[name]
	if !ok {
		return "", errors.New("Tool " + name + " not found")
	}
	return def.Func(input)
}
