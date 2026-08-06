package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// WriteJsonStringBytes 把 UTF-8 字节流直接写成 JSON 字符串，避免大正文先复制为
// string 再整体编码。该函数不做 HTML 转义，但会保留 JSON 必需的字符转义。
func WriteJsonStringBytes(writer io.Writer, data []byte) error {
	if writer == nil {
		return errors.New("JSON 字符串写入器为空")
	}
	if !utf8.Valid(data) {
		return errors.New("JSON 字符串不是有效 UTF-8")
	}
	if _, err := io.WriteString(writer, `"`); err != nil {
		return err
	}
	start := 0
	for index, value := range data {
		var escaped string
		switch value {
		case '\\':
			escaped = `\\`
		case '"':
			escaped = `\"`
		case '\b':
			escaped = `\b`
		case '\f':
			escaped = `\f`
		case '\n':
			escaped = `\n`
		case '\r':
			escaped = `\r`
		case '\t':
			escaped = `\t`
		default:
			if value < 0x20 {
				escaped = `\u00` + string("0123456789abcdef"[value>>4]) + string("0123456789abcdef"[value&0x0f])
			}
		}
		if escaped == "" {
			continue
		}
		if start < index {
			if _, err := writer.Write(data[start:index]); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(writer, escaped); err != nil {
			return err
		}
		start = index + 1
	}
	if start < len(data) {
		if _, err := writer.Write(data[start:]); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, `"`)
	return err
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
