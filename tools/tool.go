package tools

import (
	"fmt"
)

type ParamType string

const (
	ParamString  ParamType = "string"
	ParamInteger ParamType = "integer"
	ParamNumber  ParamType = "number"
	ParamBoolean ParamType = "boolean"
	ParamObject  ParamType = "object"
	ParamArray   ParamType = "array"
)

type Tool interface {
	Name() string
	Description() string
	Run(parameters map[string]interface{}) (string, error)
	Parameters() []ToolParameter
}

type ToolParameter struct {
	Name        string
	Type        ParamType
	Description string
	Required    bool
	Default     interface{}
	ItemsType   ParamType
}

// ToolToOpenAISchema
// 将Tool转换为兼容 OpenAI Function Calling 兼容的map
func ToolToOpenAISchema(tool Tool) map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	for _, param := range tool.Parameters() {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}

		if param.Default != nil {
			prop["description"] = fmt.Sprintf("%s (Default：%v)", param.Description, param.Default)
		}

		if param.Type == ParamArray {
			prop["items"] = map[string]string{
				"type": string(param.ItemsType),
			}
		}

		properties[param.Name] = prop

		if param.Required {
			required = append(required, param.Name)
		}
	}

	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}
