package builtins

import "awesome-agent/tools"

type RAGTool struct {
}

func (r *RAGTool) Name() string {
	//TODO implement me
	panic("implement me")
}

func (r *RAGTool) Description() string {
	//TODO implement me
	panic("implement me")
}

func (r *RAGTool) Run(parameters map[string]interface{}) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RAGTool) Parameters() []tools.ToolParameter {
	//TODO implement me
	panic("implement me")
}

func NewRAGTool(knowledgeBasePath string, collectionName string, ragNamespace string) tools.Tool {
	return nil
}
