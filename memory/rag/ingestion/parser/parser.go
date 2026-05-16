package parser

import (
	"io"
	"log/slog"
	"strings"
)

type Document struct {
	ID       string
	Name     string
	Format   string
	Content  string
	Size     int64
	Metadata map[string]string
}

type Parser interface {
	SupportedFormats() []string
	Parse(reader io.Reader, opts ParseOptions) (*Document, error)
}

type ParseOptions struct {
	MaxSize  int64
	Encoding string
	Filename string
}

type Registry struct {
	parsers []Parser
	index   map[string]Parser
}

func NewParserRegistry() *Registry {
	r := &Registry{
		parsers: []Parser{},
		index:   map[string]Parser{},
	}
	r.Register(&NativeParser{})
	return r
}

func (r *Registry) Register(p Parser) {
	formats := p.SupportedFormats()
	for _, format := range formats {
		if _, ok := r.index[format]; !ok {
			r.index[format] = p
		} else {
			slog.Warn("duplicate parser extension", "ext", format)
		}
	}
	r.parsers = append(r.parsers, p)
}

func (r *Registry) Find(ext string) (Parser, string, error) {
	ext = strings.ToLower(ext)
	if parser, ok := r.index[ext]; ok {
		return parser, ext, nil
	}
	return nil, "", ErrUnsupportedFormat
}

func (r *Registry) Supports(ext string) bool {
	_, _, err := r.Find(ext)
	return err == nil
}

func (r *Registry) SupportedFormats() []string {
	var formats []string
	for format, _ := range r.index {
		formats = append(formats, format)
	}
	return formats
}
