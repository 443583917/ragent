package mcp

import (
	"encoding/xml"
	"strings"
)

// McpResult MCP 工具调用结果
type McpResult struct {
	SubQuestion string
	ToolName    string
	Content     string
	Error       string
}

// Formatter 将 MCP 结果格式化为 LLM 上下文
type Formatter struct{}

func NewFormatter() *Formatter { return &Formatter{} }

type toolDataSingle struct {
	XMLName xml.Name `xml:"tool-data"`
	Data    string   `xml:"data"`
}

type toolDataMulti struct {
	XMLName xml.Name     `xml:"tool-data"`
	Results []toolResult `xml:"result"`
}

type toolResult struct {
	Index    int    `xml:"index,attr"`
	Question string `xml:"question"`
	Data     string `xml:"data"`
}

// Format 将 MCP 结果列表格式化为 <tool-data> XML
func (f *Formatter) Format(results []McpResult) string {
	if len(results) == 0 {
		return ""
	}

	valid := make([]McpResult, 0, len(results))
	for _, r := range results {
		if r.Content != "" || r.Error != "" {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return ""
	}

	if len(valid) == 1 {
		r := valid[0]
		content := r.Content
		if r.Error != "" {
			content = "错误: " + r.Error
		}
		td := toolDataSingle{Data: content}
		b, _ := xml.MarshalIndent(td, "", "  ")
		return string(b)
	}

	var trs []toolResult
	for i, r := range valid {
		content := r.Content
		if r.Error != "" {
			content = "错误: " + r.Error
		}
		trs = append(trs, toolResult{
			Index:    i + 1,
			Question: r.SubQuestion,
			Data:     content,
		})
	}
	td := toolDataMulti{Results: trs}
	b, _ := xml.MarshalIndent(td, "", "  ")
	return string(b)
}

// FormatMixed MCP + KB 混合场景格式化
func (f *Formatter) FormatMixed(results []McpResult, kbText string) (toolData string, documents string) {
	toolData = f.Format(results)
	if kbText != "" {
		documents = "<documents>\n" + strings.TrimSpace(kbText) + "\n</documents>"
	}
	return
}
