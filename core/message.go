package core

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`

	Timestamp time.Time              `json:"-"`
	Metadata  map[string]interface{} `json:"-"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ContentType string

const (
	Text  ContentType = "text"
	Image ContentType = "image_url"
	Audio ContentType = "input_audio"
	File  ContentType = "file"
)

type MIMEType string

const (
	// 图片
	JPEG MIMEType = "image/jpeg"
	PNG  MIMEType = "image/png"
	GIF  MIMEType = "image/gif"
	WEBP MIMEType = "image/webp"
	// 音频
	WAV MIMEType = "audio/wav"
	MP3 MIMEType = "audio/mp3"
	// 视频/普通文件
	MP4   MIMEType = "video/mp4"
	PDF   MIMEType = "application/pdf"
	WORD  MIMEType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	EXCEL MIMEType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	PPT   MIMEType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	TEXT  MIMEType = "text/plain"
)

type ContentPart struct {
	Type       ContentType  `json:"type"`
	Text       string       `json:"text,omitempty"`
	ImageURL   *ImageURL    `json:"image_url,omitempty"`
	InputAudio *InputAudio  `json:"input_audio,omitempty"`
	File       *FileContent `json:"file,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type InputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type FileContent struct {
	Filename string `json:"filename,omitempty"`
	Data     string `json:"data"`
}

func NewTextContentPart(text string) ContentPart {
	return ContentPart{
		Type: Text,
		Text: text,
	}
}

func NewImageContentPart(image *ImageURL) ContentPart {
	return ContentPart{
		Type:     Image,
		ImageURL: image,
	}
}

func NewAudioContentPart(audio *InputAudio) ContentPart {
	return ContentPart{
		Type:       Audio,
		InputAudio: audio,
	}
}

func NewFileContentPart(file *FileContent) ContentPart {
	return ContentPart{
		Type: File,
		File: file,
	}
}

func BuildBase64URL(data []byte, mimeType MIMEType) string {
	base64Str := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str)
}

const maxDataLen = 100

func (m Message) String() string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(m.Role)
	b.WriteString("]")

	if m.ToolCallID != "" {
		b.WriteString("[call_id=")
		b.WriteString(m.ToolCallID)
		b.WriteString("]")
	}
	if m.Name != "" {
		b.WriteString("[name=")
		b.WriteString(m.Name)
		b.WriteString("]")
	}

	content := formatContent(m.Content)
	if content != "" {
		b.WriteString(" ")
		b.WriteString(content)
	}

	if len(m.ToolCalls) > 0 {
		b.WriteString(" [tool_calls:")
		for i, tc := range m.ToolCalls {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(" ")
			b.WriteString(tc.Function.Name)
			b.WriteString("(")
			args := tc.Function.Arguments
			if len(args) > 200 {
				args = args[:200] + "..."
			}
			b.WriteString(args)
			b.WriteString(")")
		}
		b.WriteString("]")
	}

	return b.String()
}

func formatContent(c interface{}) string {
	switch v := c.(type) {
	case string:
		return v
	case []ContentPart:
		parts := make([]string, 0, len(v))
		for _, p := range v {
			parts = append(parts, p.format())
		}
		return strings.Join(parts, " ")
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (cp ContentPart) format() string {
	switch cp.Type {
	case Text:
		return cp.Text
	case Image:
		url := ""
		if cp.ImageURL != nil {
			url = truncate(cp.ImageURL.URL, maxDataLen)
		}
		return "[image: " + url + "]"
	case Audio:
		data := ""
		format := ""
		if cp.InputAudio != nil {
			data = truncate(cp.InputAudio.Data, maxDataLen)
			format = cp.InputAudio.Format
		}
		return "[audio: " + format + " " + data + "]"
	case File:
		filename := ""
		data := ""
		if cp.File != nil {
			filename = cp.File.Filename
			data = truncate(cp.File.Data, maxDataLen)
		}
		return "[file: " + filename + " " + data + "]"
	default:
		return ""
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
