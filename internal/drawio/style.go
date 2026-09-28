package drawio

import "strings"

// ParseStyle은 draw.io의 "key=value;key=value;flag;" 형식 스타일 문자열을
// map으로 정확히 파싱한다. 부분 문자열 포함 검사(strings.Contains)로 도형을
// 판정하면 "shape=table"이 "shape=tableRow"에도 매칭되는 버그가 재발하므로,
// 반드시 이 맵을 통해 정확히(==) 비교한다.
func ParseStyle(style string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(style, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "="); idx >= 0 {
			key := part[:idx]
			value := part[idx+1:]
			result[key] = value
		} else {
			result[part] = ""
		}
	}
	return result
}
