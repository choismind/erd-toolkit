// internal/drawio/erdattrs.go
package drawio

import "sort"

// ErdAttrPrefix는 이 도구가 읽는 커스텀 속성의 접두어다. draw.io의 «데이터
// 편집»(Ctrl+M)은 사용자들이 이미 자기 용도로 쓰는 기능이라, 접두어 없는
// `check` 같은 이름은 남의 데이터와 부딪친다.
const ErdAttrPrefix = "erd_"

// 읽는 이름 셋. 작업 기록이 예로 든 두 개(erd_check·erd_default)에
// 정의서가 가장 아쉬워하던 설명(erd_comment)을 더한 것이다.
//
// UNIQUE는 여기 없다 — 복합 UNIQUE는 키 셀과 rowspan이라는 **이미 있는
// 어휘**로 그릴 수 있고(복합 PK를 그리는 그 방식 그대로), 인덱스는 설계가
// 아니라 운영이라 순방향 범위 밖이다(원장 "«object» 커스텀 속성" 절).
const (
	AttrCheck   = "erd_check"
	AttrDefault = "erd_default"
	AttrComment = "erd_comment"
)

// columnAttrNames / tableAttrNames는 그 자리에서 뜻을 갖는 이름이다.
// erd_default는 테이블에 붙일 수 없다 — 기본값은 컬럼의 성질이다.
var (
	columnAttrNames = map[string]bool{AttrCheck: true, AttrDefault: true, AttrComment: true}
	tableAttrNames  = map[string]bool{AttrCheck: true, AttrComment: true}
)

// AttrConflict는 같은 이름이 한 컬럼(또는 한 테이블)에 둘 이상 붙었는데
// 값이 서로 다른 경우다. **어느 쪽도 채택하지 않는다** — 우선순위를 정해
// 조용히 하나를 고르면 사용자는 자기가 쓴 값이 사라진 것을 끝까지 모른다
// (2026-09-03 결정, 작업 기록).
type AttrConflict struct {
	Name string
	// Values는 서로 다른 값 전부다. 정렬해서 담는다 — 셀을 읽는 순서는
	// 문서 순서에 따라 달라질 수 있는데, 이 값이 검증 리포트의 문구가 되고
	// annotate가 그것을 그림에 쓴다. 순서가 흔들리면 두 번 돌린 결과가
	// 바이트 단위로 달라진다.
	Values []string
}

// ErdAttrs는 셀 하나 또는 여럿에서 모은 erd_* 속성의 최종 상태다.
type ErdAttrs struct {
	// Values는 값이 하나로 정해진 것만 담는다. 충돌한 이름은 여기 없다.
	Values map[string]string
	// Conflicts는 이름 순이다.
	Conflicts []AttrConflict
	// Unknown은 erd_ 접두어를 달았지만 이 자리에서 뜻이 없는 이름이다
	// (오타 `erd_chek`, 또는 테이블에 붙인 erd_default). 이름 순.
	//
	// 이것을 진단으로 올리지 않으면 오타가 **에러 없이 값을 없앤다** —
	// draw.io도 아무 말 하지 않고 파서도 모르는 이름이라 무시하기 때문이다.
	Unknown []string
}

// Get은 값이 하나로 정해진 속성을 돌려준다. 충돌했거나 없으면 빈 문자열이다.
func (a ErdAttrs) Get(name string) string { return a.Values[name] }

// HasConflict는 그 이름이 충돌했는지 본다. 리포터가 «값이 비었다»와
// «값을 고르지 않았다»를 갈라 써야 하기 때문이다 — 전자는 아무것도 안
// 적으면 되지만 후자는 그 자리에 주석을 남겨야 한다.
func (a ErdAttrs) HasConflict(name string) bool {
	for _, c := range a.Conflicts {
		if c.Name == name {
			return true
		}
	}
	return false
}

// ColumnErdAttrs는 컬럼 하나의 제약을 모은다. 행·키 셀·정의 셀을 **모두**
// 그 컬럼의 것으로 받는다 — 셋 다 같은 컬럼을 가리키기 때문이다. 「정의
// 셀만」으로 좁히면 충돌은 없어지지만 키 셀에 잘못 붙인 값이 아무 말 없이
// 사라져서, 실수가 더 조용해진다(작업 기록).
func ColumnErdAttrs(row, keyCell, defCell RawCell) ErdAttrs {
	return collectErdAttrs(columnAttrNames, row, keyCell, defCell)
}

// TableErdAttrs는 테이블 제약을 모은다. 붙일 자리가 테이블 셀 하나뿐이라
// 충돌이 날 수 없지만, 모르는 이름은 여기서도 잡는다.
func TableErdAttrs(table RawCell) ErdAttrs {
	return collectErdAttrs(tableAttrNames, table)
}

func collectErdAttrs(known map[string]bool, cells ...RawCell) ErdAttrs {
	// seen[name]은 그 이름으로 실제 들어온 «서로 다른» 값들이다.
	seen := map[string][]string{}
	for _, c := range cells {
		for name, value := range c.Attrs {
			if !hasErdPrefix(name) {
				continue // 남이 쓰던 데이터다. 우리 것이 아니므로 건드리지 않는다
			}
			if !containsString(seen[name], value) {
				seen[name] = append(seen[name], value)
			}
		}
	}

	out := ErdAttrs{}
	for name, values := range seen {
		if !known[name] {
			out.Unknown = append(out.Unknown, name)
			continue
		}
		if len(values) > 1 {
			sort.Strings(values)
			out.Conflicts = append(out.Conflicts, AttrConflict{Name: name, Values: values})
			continue
		}
		if out.Values == nil {
			out.Values = make(map[string]string)
		}
		out.Values[name] = values[0]
	}

	// map 순회 순서는 실행마다 다르다. 이 순서가 검증 리포트의 줄 순서로
	// 그대로 내려가므로(ir.Assemble의 중복 이름 정렬과 같은 이유) 여기서
	// 정렬한다.
	sort.Strings(out.Unknown)
	sort.Slice(out.Conflicts, func(i, j int) bool { return out.Conflicts[i].Name < out.Conflicts[j].Name })
	return out
}

func hasErdPrefix(name string) bool {
	return len(name) > len(ErdAttrPrefix) && name[:len(ErdAttrPrefix)] == ErdAttrPrefix
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
