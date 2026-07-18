package prompt

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync"
)

//go:embed prompts/*
var promptFS embed.FS

// TemplateLoader Prompt 模板加载器（Prompt 模板加载器）
type TemplateLoader struct {
	mu    sync.RWMutex
	cache map[string]string            // path → content
	secCache map[string]map[string]string // path → (section → content)
}

// NewTemplateLoader 创建模板加载器
func NewTemplateLoader() *TemplateLoader {
	return &TemplateLoader{
		cache:    make(map[string]string),
		secCache: make(map[string]map[string]string),
	}
}

// Load 加载模板文件（带缓存）
// path 格式: "intent-classifier.st"（自动拼 prompts/ 前缀）
func (l *TemplateLoader) Load(path string) (string, error) {
	l.mu.RLock()
	if content, ok := l.cache[path]; ok {
		l.mu.RUnlock()
		return content, nil
	}
	l.mu.RUnlock()

	fullPath := "prompts/" + path
	data, err := fs.ReadFile(promptFS, fullPath)
	if err != nil {
		return "", fmt.Errorf("读取模板 %s 失败: %w", path, err)
	}

	content := string(data)
	l.mu.Lock()
	l.cache[path] = content
	l.mu.Unlock()

	return content, nil
}

// Render 加载模板并填充占位符 {{.Key}}
func (l *TemplateLoader) Render(path string, slots map[string]string) (string, error) {
	template, err := l.Load(path)
	if err != nil {
		return "", err
	}
	filled := fillSlots(template, slots)
	return cleanupPrompt(filled), nil
}

// LoadSection 加载模板中指定 section
// section 对应 "--- section: name ---" 中的 name
func (l *TemplateLoader) LoadSection(path, section string) (string, error) {
	sections, err := l.loadSections(path)
	if err != nil {
		return "", err
	}
	template, ok := sections[section]
	if !ok {
		return "", fmt.Errorf("模板 section 不存在: %s -> %s", path, section)
	}
	return template, nil
}

// RenderSection 渲染模板中的指定 section
func (l *TemplateLoader) RenderSection(path, section string, slots map[string]string) (string, error) {
	template, err := l.LoadSection(path, section)
	if err != nil {
		return "", err
	}
	filled := fillSlots(template, slots)
	return cleanupPrompt(filled), nil
}

func (l *TemplateLoader) loadSections(path string) (map[string]string, error) {
	l.mu.RLock()
	if secs, ok := l.secCache[path]; ok {
		l.mu.RUnlock()
		return secs, nil
	}
	l.mu.RUnlock()

	content, err := l.Load(path)
	if err != nil {
		return nil, err
	}

	sections := parseSections(content)
	l.mu.Lock()
	l.secCache[path] = sections
	l.mu.Unlock()

	return sections, nil
}

// ========== 模板工具函数==========

var (
	multiBlankLines = regexp.MustCompile(`(\n){3,}`)
	sectionHeader   = regexp.MustCompile(`(?m)^---\s*section:\s*(\S+)\s*---$`)
)

func cleanupPrompt(prompt string) string {
	if prompt == "" {
		return ""
	}
	return strings.TrimSpace(multiBlankLines.ReplaceAllString(prompt, "\n\n"))
}

func fillSlots(template string, slots map[string]string) string {
	result := template
	for key, val := range slots {
		placeholder := "{{." + key + "}}"
		result = strings.ReplaceAll(result, placeholder, val)
	}
	return result
}

func parseSections(content string) map[string]string {
	sections := make(map[string]string)
	if content == "" {
		return sections
	}

	matches := sectionHeader.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return sections
	}

	for i, match := range matches {
		nameStart := match[2]
		nameEnd := match[3]
		name := content[nameStart:nameEnd]

		bodyStart := match[1] // section header 的结束位置
		bodyEnd := len(content)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0] // 下一个 section 的开始位置
		}

		body := strings.TrimSpace(content[bodyStart:bodyEnd])
		// 去掉开头的换行
		body = strings.TrimLeft(body, "\n")
		body = strings.TrimRight(body, " \t\n")

		sections[name] = body
	}

	return sections
}
