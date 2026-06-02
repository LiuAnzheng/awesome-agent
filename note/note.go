package note

type NoteType string

const (
	NoteTypeTaskState  NoteType = "task_state" // 任务状态跟踪
	NoteTypeConclusion NoteType = "conclusion" // 结论
	NoteTypeBlocker    NoteType = "blocker"    // 阻塞项
	NoteTypeAction     NoteType = "action"     // 行动项
	NoteTypeReference  NoteType = "reference"  // 参考资料
	NoteTypeGeneral    NoteType = "general"    // 通用笔记
)

type NoteMetadata struct {
	ID        string   `json:"id" yaml:"id"`
	Title     string   `json:"title" yaml:"title"`
	Type      NoteType `json:"type" yaml:"type"`
	Tags      []string `json:"tags" yaml:"tags"`
	CreatedAt string   `json:"created_at" yaml:"created_at"`
	UpdatedAt string   `json:"updated_at" yaml:"updated_at"`
	FilePath  string   `json:"file_path" yaml:"file_path"`
}

type NoteIndex map[string]NoteMetadata
