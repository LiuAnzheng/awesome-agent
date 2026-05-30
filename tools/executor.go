package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"memoria/core"
	"strings"
)

type ToolExecutor struct {
	registry *ToolRegistry
}

func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return &ToolExecutor{registry: registry}
}

func (e *ToolExecutor) Execute(toolCalls []core.ToolCall) ([]core.Message, error) {
	if e.registry == nil {
		return nil, errors.New("ToolRegistry has not been initialized")
	}

	results := make([]core.Message, 0, len(toolCalls))
	for _, tc := range toolCalls {
		msg := e.executeOne(tc)
		results = append(results, msg)
	}
	return results, nil
}

func (e *ToolExecutor) executeOne(tc core.ToolCall) core.Message {
	tool, ok := e.registry.Tool(tc.Function.Name)
	if !ok {
		return core.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("tool %q not found", tc.Function.Name),
			ToolCallID: tc.ID,
			Timestamp:  core.Now(),
		}
	}

	var params map[string]interface{}
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
			return core.Message{
				Role:       "tool",
				Content:    fmt.Sprintf("failed to parse arguments: %v", err),
				ToolCallID: tc.ID,
			}
		}
	}
	if params == nil {
		params = make(map[string]interface{})
	}

	params, err := validateParams(tool, params)
	if err != nil {
		return core.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("parameter validation failed: %v", err),
			ToolCallID: tc.ID,
		}
	}

	result, err := tool.Run(params)
	if err != nil {
		slog.Error("tool execution failed", "error", err)
		return core.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("tool execution failed: %v", err),
			ToolCallID: tc.ID,
		}
	}
	slog.Debug("tool executed", "name", tc.Function.Name, "result", result)

	return core.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: tc.ID,
	}
}

func validateParams(tool Tool, params map[string]interface{}) (map[string]interface{}, error) {
	specs := make(map[string]ToolParameter, len(tool.Parameters()))
	for _, p := range tool.Parameters() {
		specs[p.Name] = p
	}

	// 校验必填 + 类型 + 填充默认值
	for name, spec := range specs {
		val, ok := params[name]
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("missing required parameter: %s", name)
			}
			if spec.Default != nil {
				params[name] = spec.Default
			}
			continue
		}
		if err := checkType(name, val, spec.Type); err != nil {
			return nil, err
		}
	}

	// 移除非系统参数（_ 前缀保留给框架注入）
	for name := range params {
		if _, ok := specs[name]; !ok && !strings.HasPrefix(name, "_") {
			delete(params, name)
		}
	}

	return params, nil
}

func checkType(name string, val interface{}, expected ParamType) error {
	switch expected {
	case ParamString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameter %q expect string, got %T", name, val)
		}
	case ParamInteger:
		f, ok := toFloat(val)
		if !ok {
			return fmt.Errorf("parameter %q expect integer, got %T", name, val)
		}
		if math.Trunc(f) != f {
			return fmt.Errorf("parameter %q expect integer, got %v", name, f)
		}
	case ParamNumber:
		if _, ok := toFloat(val); !ok {
			return fmt.Errorf("parameter %q expect number, got %T", name, val)
		}
	case ParamBoolean:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameter %q expect boolean, got %T", name, val)
		}
	case ParamObject:
		if _, ok := val.(map[string]interface{}); !ok {
			return fmt.Errorf("parameter %q expect object, got %T", name, val)
		}
	case ParamArray:
		if _, ok := val.([]interface{}); !ok {
			return fmt.Errorf("parameter %q expect array, got %T", name, val)
		}
	}
	return nil
}

func toFloat(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
