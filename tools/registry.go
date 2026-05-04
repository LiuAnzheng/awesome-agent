package tools

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "[tools] ", log.LstdFlags|log.Lshortfile)

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(t Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if _, ok := r.tools[t.Name()]; ok {
		logger.Printf("tool %s already registered, will be replaced", t.Name())
	}
	r.tools[t.Name()] = t
}

func (r *ToolRegistry) Tool(name string) (Tool, bool) {
	if r.tools == nil {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) List() []Tool {
	if r.tools == nil {
		return nil
	}
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *ToolRegistry) ToOpenAISchemas() []map[string]interface{} {
	if r.tools == nil || len(r.tools) == 0 {
		return nil
	}
	schemas := make([]map[string]interface{}, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, ToolToOpenAISchema(t))
	}
	return schemas
}
