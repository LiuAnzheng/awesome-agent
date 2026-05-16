package parser

import "errors"

var (
	ErrUnsupportedFormat = errors.New("unsupported document format")
	ErrDocumentTooLarge  = errors.New("document exceeds maximum size")
	ErrEmptyContent      = errors.New("document content is empty")
	ErrParseFailed       = errors.New("document parse failed")
)
