package chunker

import "errors"

var (
	ErrEmptyContent = errors.New("chunker: content is empty")
)
