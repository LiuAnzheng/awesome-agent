package retrieval

import (
	"sync"

	"github.com/go-ego/gse"
)

type Tokenizer struct {
	seg gse.Segmenter
}

var (
	instance *Tokenizer
	once     sync.Once
)

func GetTokenizer() *Tokenizer {
	once.Do(func() {
		instance = &Tokenizer{}
		instance.seg, _ = gse.New()
	})
	return instance
}

// CutForIndex 索引模式：CutSearch 对长词二次切分，丰富倒排索引词条
func (t *Tokenizer) CutForIndex(text string) []string {
	return t.seg.CutSearch(text, true)
}

// CutForQuery 查询模式：Cut 精确切分，减少噪声匹配
func (t *Tokenizer) CutForQuery(text string) []string {
	return t.seg.Cut(text, true)
}
