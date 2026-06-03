package builtins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"runtime"

	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/tools"
)

type TerminalSession struct {
	cwd      string
	createAt time.Time
}

type TerminalTool struct {
	mu               sync.Mutex
	sessions         map[string]*TerminalSession
	defaultSessionID string
	timeout          time.Duration
	maxOutputSize    int64
	initDir          string
	workSpace        string
	allowCD          bool
	allowedCommands  map[string]struct{}
}

func (t *TerminalTool) Name() string {
	return "terminal_tool"
}

func (t *TerminalTool) Description() string {
	return `
  ═══ TERMINAL TOOL — COMMAND EXECUTION & DIRECTORY NAVIGATION ═══

  Execute shell commands in a persistent working directory. The tool maintains
  a per-session current directory — cd commands change it, and all subsequent
  commands run in that directory.

  ═══ HOW IT WORKS ═══

    1. Every command runs in the session's current directory (starts at project root).
    2. cd <path> is intercepted — no subprocess spawned, just updates the working dir.
       Supports relative paths (cd ../foo) and absolute paths (cd D:\projects).
    3. All other commands execute via the system shell (cmd /c on Windows, sh -c elsewhere).

  ═══ OUTPUT FORMAT ═══

    📁 E:\current\working\dir        ← always shown first

    <stdout content>

    [stderr]
    <stderr content, if any>

    Status markers:
      ⚠️ Return code: N              ← non-zero exit code
      ⚠️ Output truncated (N bytes)  ← output exceeded limit
      ✅ Executed (no output)        ← success with empty stdout+stderr
      ❌ Command timeout (>30s)      ← deadline exceeded

  ═══ WHEN TO USE ═══

    DO use for:
      - Reading/writing files (cat, dir/ls, type, Get-Content)
      - Running build commands (go build, go test, npm install)
      - Git operations (git status, git diff, git log)
      - Exploring directory structure (ls -R, dir /s, tree)
      - Installing dependencies (go get, pip install, npm install)
      - Any filesystem operation the agent can't do directly

    Do NOT use for:
      - Interactive commands (vim, less, ssh) — they will hang until timeout
      - Commands that require user input (read, input prompts)
      - Long-running servers or daemons (they will timeout)

  ═══ LIMITATIONS ═══

    - Timeout: 30s per command. Long builds may need to be split.
    - Output cap: 100KB truncated. Use head/tail/Select-Object to narrow output.
    - No persistent env vars across calls (each invocation is a fresh shell).
    - Interactive/blocking commands will hang and timeout.`
}

func NewTerminalTool(cfg core.TerminalConfig) (tools.Tool, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxOutputSize == 0 {
		cfg.MaxOutputSize = 100 * 1024
	}
	if cfg.InitDir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cfg.InitDir = dir
	}
	if cfg.WorkSpace == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cfg.WorkSpace = dir
	}
	defaultAllow := true
	if cfg.AllowCD == nil {
		cfg.AllowCD = &defaultAllow
	}

	allowed := make(map[string]struct{}, len(cfg.AllowedCommands))
	for _, name := range cfg.AllowedCommands {
		allowed[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	tt := &TerminalTool{
		sessions:        make(map[string]*TerminalSession),
		timeout:         cfg.Timeout,
		maxOutputSize:   cfg.MaxOutputSize,
		initDir:         cfg.InitDir,
		workSpace:       cfg.WorkSpace,
		allowCD:         *cfg.AllowCD,
		allowedCommands: allowed,
	}
	defaultSessionID := strconv.FormatInt(time.Now().UnixNano(), 10)
	tt.defaultSessionID = defaultSessionID
	err := tt.AddSession(tt.defaultSessionID, tt.initDir)
	if err != nil {
		return nil, err
	}
	return tt, nil
}

func (t *TerminalTool) AddSession(sessionID string, cwd string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cwd == "" {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cwd = dir
	}
	if sessionID == "" {
		sessionID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	t.sessions[sessionID] = &TerminalSession{
		cwd:      cwd,
		createAt: core.Now(),
	}
	if t.defaultSessionID == "" {
		t.defaultSessionID = sessionID
	}
	return nil
}

func (t *TerminalTool) Session(sessionID string) (*TerminalSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sessionID == "" {
		sessionID = t.defaultSessionID
	}
	if session, ok := t.sessions[sessionID]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session %s not found", sessionID)
}

func (t *TerminalTool) RemoveSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.sessions[sessionID]; !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	delete(t.sessions, sessionID)
	return nil
}

func (t *TerminalTool) UseSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.sessions[sessionID]; !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	t.defaultSessionID = sessionID
	return nil
}

func (t *TerminalTool) Run(parameters map[string]interface{}) (string, error) {
	session := t.resolveSession(parameters)
	cmd, _ := parameters["cmd"].(string)
	if cmd == "" {
		return "", fmt.Errorf("cmd is required")
	}

	if t.isCdCommand(cmd) {
		if !t.allowCD {
			return "❌ cd is disabled", nil
		}
		return t.handleCd(cmd, session), nil
	}

	if len(t.allowedCommands) > 0 {
		cmdName := t.extractCommandName(cmd)
		if _, ok := t.allowedCommands[cmdName]; !ok {
			return fmt.Sprintf("❌ Command not allowed: %s", cmdName), nil
		}
	}

	return t.executeCommand(cmd, session), nil
}

func (t *TerminalTool) resolveSession(params map[string]interface{}) *TerminalSession {
	t.mu.Lock()
	defer t.mu.Unlock()

	sessionID, _ := params["_session_id"].(string)
	if sessionID == "" {
		sessionID = t.defaultSessionID
	}
	session, ok := t.sessions[sessionID]
	if !ok {
		session = &TerminalSession{
			cwd:      t.initDir,
			createAt: core.Now(),
		}
		t.sessions[sessionID] = session
	}
	return session
}

func (t *TerminalTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{
			Name:        "cmd",
			Type:        tools.ParamString,
			Description: "The command to execute.",
			Required:    true,
		},
	}
}

func (t *TerminalTool) handleCd(cmdStr string, session *TerminalSession) string {
	targetDir := strings.TrimSpace(cmdStr[2:])
	targetDir = strings.Trim(targetDir, `"'`)

	if targetDir == "" || targetDir == "." {
		return "📁 Current: " + session.cwd
	}

	var newDir string
	switch {
	case targetDir == "~" || strings.HasPrefix(targetDir, "~/"):
		newDir = filepath.Join(t.workSpace, strings.TrimPrefix(targetDir, "~"))
		if targetDir == "~" {
			newDir = t.workSpace
		}
	case targetDir == ".." || targetDir == "../":
		newDir = filepath.Dir(session.cwd)
	case filepath.IsAbs(targetDir):
		newDir = filepath.Clean(targetDir)
	default:
		newDir = filepath.Join(session.cwd, targetDir)
	}

	newDir = filepath.Clean(newDir)

	rel, err := filepath.Rel(t.workSpace, newDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Sprintf("❌ Cannot navigate outside workspace: %s", newDir)
	}

	info, err := os.Stat(newDir)
	if os.IsNotExist(err) {
		return fmt.Sprintf("❌ Not found: %s", newDir)
	}
	if err != nil {
		return fmt.Sprintf("❌ Access error: %v", err)
	}
	if !info.IsDir() {
		return fmt.Sprintf("❌ Not a directory: %s", newDir)
	}

	session.cwd = newDir
	return fmt.Sprintf("✅ Changed to: %s", newDir)
}

func (t *TerminalTool) executeCommand(cmdStr string, session *TerminalSession) string {
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()

	shell, shellArg := shellArgs()
	cmd := exec.CommandContext(ctx, shell, shellArg, cmdStr)
	cmd.Dir = session.cwd
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("❌ Command timed out (> %v)", t.timeout)
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]\n" + stderr.String()
	}

	if output == "" && runErr == nil {
		return "✅ Executed successfully (no output)"
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			output = fmt.Sprintf("⚠️ Return code: %d\n\n%s", exitErr.ExitCode(), output)
		} else {
			return fmt.Sprintf("❌ Command failed: %v", runErr)
		}
	}

	if int64(len(output)) > t.maxOutputSize {
		output = output[:t.maxOutputSize]
		output += fmt.Sprintf("\n\n⚠️ Output truncated (exceeded %d bytes)", t.maxOutputSize)
	}

	return output
}

func (t *TerminalTool) isCdCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	return lower == "cd" ||
		strings.HasPrefix(lower, "cd ") ||
		strings.HasPrefix(lower, "cd\t")
}

// extractCommandName 从命令字符串中提取命令名（第一个 token）
// 处理引号包裹的路径：`"C:\Program Files\app\app.exe" --flag` → `c:\program files\app\app.exe`
func (t *TerminalTool) extractCommandName(cmdStr string) string {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return ""
	}
	// quoted path: extract everything between matching quotes
	if trimmed[0] == '"' || trimmed[0] == '\'' {
		quote := trimmed[0]
		if idx := strings.IndexByte(trimmed[1:], quote); idx >= 0 {
			return strings.ToLower(trimmed[1 : idx+1])
		}
		// unclosed quote — fall through to first-token behavior
	}
	// plain command: take first whitespace-delimited token
	if idx := strings.IndexAny(trimmed, " \t"); idx >= 0 {
		return strings.ToLower(trimmed[:idx])
	}
	return strings.ToLower(trimmed)
}

func shellArgs() (shell, arg string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}
