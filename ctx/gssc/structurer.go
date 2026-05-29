package gssc

import (
	"awesome-agent/ctx"
	"strings"
)

// SectionName 分区名称。
type SectionName string

const (
	SectionRolePolicies SectionName = "Role & Policies"
	SectionTask         SectionName = "Task"
	SectionEvidence     SectionName = "Evidence"
	SectionContext      SectionName = "Context"
	SectionOutput       SectionName = "Output"
)

var defaultSectionOrder = []SectionName{
	SectionRolePolicies,
	SectionTask,
	SectionEvidence,
	SectionContext,
	SectionOutput,
}

// SectionClassifier 将 ContextPacket 映射到分区。
type SectionClassifier func(packet ctx.ContextPacket) SectionName

// StructurerOption 配置 StructurerImpl。
type StructurerOption func(*StructurerImpl)

// StructurerImpl 默认 Structurer 实现。
type StructurerImpl struct {
	classifier   SectionClassifier
	sectionOrder []SectionName
	outputPrompt string
}

// NewStructurer 创建默认 Structurer 实现。
func NewStructurer(opts ...StructurerOption) *StructurerImpl {
	s := &StructurerImpl{
		classifier:   defaultClassifier,
		sectionOrder: defaultSectionOrder,
		outputPrompt: "Please provide an accurate and well-founded answer based on the information provided above.",
	}
	for _, o := range opts {
		o(s)
	}

	return s
}

// WithClassifier 设置自定义分组函数。
func WithClassifier(fn SectionClassifier) StructurerOption {
	return func(s *StructurerImpl) { s.classifier = fn }
}

// WithSectionOrder 设置分区输出顺序。
func WithSectionOrder(order []SectionName) StructurerOption {
	return func(s *StructurerImpl) { s.sectionOrder = order }
}

// WithOutputPrompt 设置输出提示语。
func WithOutputPrompt(prompt string) StructurerOption {
	return func(s *StructurerImpl) { s.outputPrompt = prompt }
}

// Structure 将选中的信息包组织成结构化的上下文模板。
func (s *StructurerImpl) Structure(packets []ctx.ContextPacket, query string) string {
	groups := make(map[SectionName][]ctx.ContextPacket, 4)
	for _, p := range packets {
		sec := s.classifier(p)
		groups[sec] = append(groups[sec], p)
	}

	parts := make([]string, 0, len(s.sectionOrder))
	for _, sec := range s.sectionOrder {
		switch sec {
		case SectionTask:
			parts = append(parts, formatSection(sec, query))
		case SectionOutput:
			parts = append(parts, formatSection(sec, s.outputPrompt))
		default:
			items, ok := groups[sec]
			if !ok || len(items) == 0 {
				continue
			}
			parts = append(parts, formatSectionItems(sec, items))
		}
	}

	return strings.Join(parts, "\n\n")
}

func formatSection(sec SectionName, content string) string {
	return "[" + string(sec) + "]\n" + content
}

func formatSectionItems(sec SectionName, items []ctx.ContextPacket) string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(string(sec))
	b.WriteString("]\n")
	for i, item := range items {
		if i > 0 {
			if sec == SectionEvidence {
				b.WriteString("\n---\n")
			} else {
				b.WriteByte('\n')
			}
		}
		b.WriteString(item.Content)
	}
	return b.String()
}

// defaultClassifier 按 Source 字段做默认分组。
//   - SystemInstructions  → Role & Policies
//   - RAG                  → Evidence
//   - Memory(semantic)     → Evidence（抽象知识）
//   - Memory(working/etc)  → Context
//   - 其他                → Context
func defaultClassifier(p ctx.ContextPacket) SectionName {
	switch p.Source {
	case ctx.SystemInstructions:
		return SectionRolePolicies
	case ctx.RAG:
		return SectionEvidence
	case ctx.Memory:
		if t, ok := p.Metadata["memory_type"]; ok && t == "semantic" {
			return SectionEvidence
		}
		return SectionContext
	default:
		return SectionContext
	}
}
