package utils

import (
	"encoding/json"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Format int

const (
	FormatPlainText Format = iota
	FormatJSON
	FormatYAML
	FormatTOML
)

// DetectFormat 自动检测文本格式
func DetectFormat(content string) Format {
	content = strings.TrimSpace(content)
	
	if content == "" {
		return FormatPlainText
	}
	
	// 检测 JSON
	if isValidJSON(content) {
		return FormatJSON
	}
	
	// 检测 YAML
	if isValidYAML(content) {
		return FormatYAML
	}
	
	// 检测 TOML
	if isValidTOML(content) {
		return FormatTOML
	}
	
	return FormatPlainText
}

// FormatContent 根据检测到的格式美化内容
func FormatContent(content string) (string, Format) {
	format := DetectFormat(content)
	formatted := content
	
	switch format {
	case FormatJSON:
		if f, err := formatJSON(content); err == nil {
			formatted = f
		}
	case FormatYAML:
		if f, err := formatYAML(content); err == nil {
			formatted = f
		}
	case FormatTOML:
		if f, err := formatTOML(content); err == nil {
			formatted = f
		}
	}
	
	return formatted, format
}

// GetFormatName 获取格式名称
func GetFormatName(format Format) string {
	switch format {
	case FormatJSON:
		return "JSON"
	case FormatYAML:
		return "YAML"
	case FormatTOML:
		return "TOML"
	default:
		return "TEXT"
	}
}

// isValidJSON 检查是否为有效的 JSON
func isValidJSON(content string) bool {
	var js interface{}
	return json.Unmarshal([]byte(content), &js) == nil
}

// isValidYAML 检查是否为有效的 YAML
func isValidYAML(content string) bool {
	var yml interface{}
	err := yaml.Unmarshal([]byte(content), &yml)
	return err == nil && (strings.Contains(content, ":") || strings.Contains(content, "-"))
}

// isValidTOML 检查是否为有效的 TOML
func isValidTOML(content string) bool {
	var tml interface{}
	_, err := toml.Decode(content, &tml)
	return err == nil && (strings.Contains(content, "=") || strings.Contains(content, "["))
}

// formatJSON 格式化 JSON
func formatJSON(content string) (string, error) {
	var obj interface{}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		return content, err
	}
	
	formatted, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return content, err
	}
	
	return string(formatted), nil
}

// formatYAML 格式化 YAML
func formatYAML(content string) (string, error) {
	var obj interface{}
	if err := yaml.Unmarshal([]byte(content), &obj); err != nil {
		return content, err
	}
	
	formatted, err := yaml.Marshal(obj)
	if err != nil {
		return content, err
	}
	
	return strings.TrimSpace(string(formatted)), nil
}

// formatTOML 格式化 TOML
func formatTOML(content string) (string, error) {
	var obj interface{}
	if _, err := toml.Decode(content, &obj); err != nil {
		return content, err
	}
	
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(obj); err != nil {
		return content, err
	}
	
	return strings.TrimSpace(buf.String()), nil
}

