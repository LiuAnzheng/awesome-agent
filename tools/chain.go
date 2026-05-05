package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"
)

// Chain 工具链 — 将多个工具调用编排为有序步骤，对外实现 Tool 接口。
// LLM 看到的是一个普通工具，实际执行时串行/并行运行多个子工具。
type Chain struct {
	name        string
	description string
	inputParams []ToolParameter // 暴露给 LLM 的入参
	steps       []ChainStep
	registry    *ToolRegistry // 执行时从这里取子工具
}

func NewChain(name string,
	description string,
	inputParams []ToolParameter,
	steps []ChainStep,
	registry *ToolRegistry) *Chain {
	c := &Chain{
		name:        name,
		description: description,
		inputParams: inputParams,
		steps:       steps,
		registry:    registry,
	}
	if err := c.validate(); err != nil {
		panic(fmt.Sprintf("chain %q: %v", name, err))
	}
	return c
}

func (c *Chain) validate() error {
	// 记录并行组之前已定义的所有 StoreAs（串行步骤 + 前面并行组的输出）
	definedNames := make(map[string]bool)

	for i := 0; i < len(c.steps); {
		// 串行步骤：直接收入 definedNames
		if !c.steps[i].Parallel {
			if c.steps[i].StoreAs != "" {
				definedNames[c.steps[i].StoreAs] = true
			}
			i++
			continue
		}

		// 收集连续的 Parallel 步骤
		end := i
		for end < len(c.steps) && c.steps[end].Parallel {
			end++
		}
		group := c.steps[i:end]

		// 校验这个并行组
		if err := checkParallelGroup(group, definedNames, i+1); err != nil {
			return err
		}

		// 组内所有 StoreAs 收入 definedNames，供后续步骤引用
		for _, s := range group {
			if s.StoreAs != "" {
				definedNames[s.StoreAs] = true
			}
		}
		i = end
	}
	return nil
}

func checkParallelGroup(group []ChainStep, outside map[string]bool, stepOffset int) error {
	// 1. 检查组内 StoreAs 重名
	names := make(map[string]int, len(group))
	for idx, s := range group {
		if s.StoreAs == "" {
			continue
		}
		if prev, ok := names[s.StoreAs]; ok {
			return fmt.Errorf("step %d and step %d: duplicate StoreAs %q in same parallel group",
				stepOffset+prev, stepOffset+idx+1, s.StoreAs)
		}
		names[s.StoreAs] = idx + 1 // 存 1-based，避免 0 值歧义
	}

	// 2. 检查组内步骤是否引用了同组其他步骤的 $steps.xxx
	for idx, s := range group {
		for _, template := range s.ParamMap {
			for _, ref := range extractStepsRefs(template) {
				// 引用的是外部已定义的步骤 → 允许
				if outside[ref] {
					continue
				}
				// 引用的是同组内的步骤 → 禁止（含自引用和交叉引用）
				if otherIdx, inGroup := names[ref]; inGroup {
					if otherIdx == idx+1 {
						return fmt.Errorf(
							"step %d (%s) references its own output $steps.%s — "+
								"a step cannot depend on itself",
							stepOffset+idx+1, s.StoreAs, ref)
					}
					return fmt.Errorf(
						"step %d (%s) references $steps.%s from step %d (%s) in the same parallel "+
							"group — parallel steps cannot depend on each other",
						stepOffset+idx+1, s.StoreAs, ref,
						stepOffset+otherIdx, group[otherIdx-1].StoreAs)
				}
				// 引用了一个不存在的步骤 → 报错
				return fmt.Errorf(
					"step %d (%s) references $steps.%s which is not defined before this parallel group",
					stepOffset+idx+1, s.StoreAs, ref)
			}
		}
	}
	return nil
}

// extractStepsRefs 从 "$steps.s1 $steps.s2 深度分析" 中提取 "s1 s2"。
func extractStepsRefs(template string) []string {
	const prefix = "$steps."
	var refs []string
	s := template
	for {
		i := strings.Index(s, prefix)
		if i == -1 {
			break
		}
		rest := s[i+len(prefix):]
		end := strings.IndexFunc(rest, func(r rune) bool {
			return !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
		})
		if end == -1 {
			refs = append(refs, rest)
			break
		}
		refs = append(refs, rest[:end])
		s = rest[end:]
	}
	return refs
}

func (c *Chain) Name() string {
	return c.name
}

func (c *Chain) Description() string {
	return c.description
}

func (c *Chain) Parameters() []ToolParameter {
	return c.inputParams
}

// Run 执行工具链。串行步骤逐一执行；连续的 Parallel 步骤合并为一组并发执行。
func (c *Chain) Run(inputParameters map[string]interface{}) (string, error) {
	if c.steps == nil || len(c.steps) == 0 {
		return "", errors.New("no steps defined")
	}
	if c.registry == nil {
		return "", errors.New("no registry defined")
	}
	ctx := &chainContext{
		mu:    sync.RWMutex{},
		input: inputParameters,
		steps: map[string]string{},
		last:  "",
	}

	for i := 0; i < len(c.steps); i++ {
		step := c.steps[i]
		if !step.Parallel {
			err := c.runSerialStep(i, step, ctx)
			if err != nil {
				return "", err
			}
			continue
		}
		start := i
		end := i
		for end < len(c.steps) && c.steps[end].Parallel {
			end++
		}
		err := c.runParallelGroup(start, c.steps[start:end], ctx)
		if err != nil {
			return "", err
		}
		i = end - 1 // for 循环的 i++ 会把指针移到并行组的末尾
	}

	return ctx.last, nil
}

func (c *Chain) runSerialStep(i int, step ChainStep, ctx *chainContext) error {
	input, err := ctx.resolve(step.ParamMap)
	if err != nil {
		return fmt.Errorf("step %d (%s): resolve params: %w", i+1, step.ToolName, err)
	}
	tool, ok := c.registry.Tool(step.ToolName)
	if !ok {
		return fmt.Errorf("step %d: tool %q not found", i+1, step.ToolName)
	}
	toolRes, err := tool.Run(input)
	if err != nil {
		return fmt.Errorf("step %d (%s): %w", i+1, step.ToolName, err)
	}
	ctx.store(step.StoreAs, toolRes)
	return nil
}

func (c *Chain) runParallelGroup(i int, steps []ChainStep, ctx *chainContext) error {
	var wg sync.WaitGroup
	errs := make([]error, len(steps))
	for j, step := range steps {
		wg.Add(1)
		go func(idx int, step ChainStep) {
			defer wg.Done()
			input, err := ctx.resolve(step.ParamMap)
			if err != nil {
				errs[idx] = fmt.Errorf("step %d (%s): resolve params: %w", i+idx+1, step.ToolName, err)
				return
			}
			tool, ok := c.registry.Tool(step.ToolName)
			if !ok {
				errs[idx] = fmt.Errorf("step %d: tool %q not found", i+idx+1, step.ToolName)
				return
			}
			toolRes, err := tool.Run(input)
			if err != nil {
				errs[idx] = fmt.Errorf("step %d (%s): %w", i+idx+1, step.ToolName, err)
				return
			}
			ctx.store(step.StoreAs, toolRes)
		}(j, step)
	}
	wg.Wait()

	var joined error
	for _, err := range errs {
		joined = errors.Join(joined, err)
	}

	if joined != nil {
		return fmt.Errorf("parallel group steps %d-%d: %w", i+1, i+len(steps), joined)
	}

	return nil
}

type ChainStep struct {
	ToolName string            // 子工具名，必须在 registry 中
	ParamMap map[string]string // 参数映射，值可含 $input.xxx / $steps.xxx 占位符
	StoreAs  string            // 输出别名，供后续步骤通过 $steps.xxx 引用；空表示不保存
	Parallel bool              // 默认 false（串行）；为 true 时与相邻 Parallel 步骤并发执行
}

// chainContext 运行时上下文，持有 Chain.Run 的入参和各步骤输出。
// mu 保护 steps/last，并行步骤通过 store/load 并发安全访问。
type chainContext struct {
	mu    sync.RWMutex
	input map[string]interface{} // Chain 入参，只读，无需加锁
	steps map[string]string      // 已执行步骤的输出，key 为 StoreAs
	last  string                 // 最近一步输出
}

func (ctx *chainContext) store(as string, toolRes string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if as != "" {
		ctx.steps[as] = toolRes
	}
	ctx.last = toolRes
}

func (ctx *chainContext) load(as string) (string, bool) {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	if v, ok := ctx.steps[as]; ok {
		return v, true
	}
	return "", false
}

func (ctx *chainContext) resolve(raw map[string]string) (map[string]interface{}, error) {
	res := make(map[string]interface{}, len(raw))
	for k, template := range raw {
		val, err := ctx.expand(template)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve template '%s': %w", template, err)
		}
		res[k] = val
	}
	return res, nil
}

// expand 把模板中的 $input.xxx / $steps.xxx 替换为实际值，其余文本原样保留。
func (ctx *chainContext) expand(s string) (string, error) {
	var err error = nil
	res := os.Expand(s, func(place string) string {
		if err != nil {
			return ""
		}
		if strings.HasPrefix(place, "input.") {
			field := strings.TrimPrefix(place, "input.")
			val, ok := ctx.input[field]
			if !ok {
				err = fmt.Errorf("no such parameter '%s'", field)
				return ""
			}
			marshal, e := json.Marshal(val)
			if e != nil {
				err = e
			}
			return string(marshal)
		}
		if strings.HasPrefix(place, "steps.") {
			field := strings.TrimPrefix(place, "steps.")
			val, ok := ctx.load(field)
			if !ok {
				err = fmt.Errorf("no such step '%s'", field)
				return ""
			}
			return val
		}
		err = fmt.Errorf("$%s: unknown prefix, use $input.xxx or $steps.xxx", place)
		return ""
	})
	return res, err
}
