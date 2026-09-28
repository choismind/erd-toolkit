// internal/glossary/dict.go
package glossary

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Dict는 표준용어사전 xlsx에서 읽은 두 벌의 대응표다.
//
//	terms — 공통표준용어: 복합어 통째로 등록된 것 (고객번호 -> CUST_NO)
//	words — 공통표준단어: 낱말 단위 (고객 -> CUST)
//
// 변환은 terms를 먼저 보고, 없으면 words로 쪼개 맞춘다.
type Dict struct {
	terms        map[string]string
	words        map[string]string
	maxWordRunes int
}

// Load는 표준용어사전 xlsx를 읽는다.
//
// 시트를 «이름»이 아니라 «순서»로 집는다: 첫 시트가 용어사전, 둘째가
// 단어사전이다. 레퍼런스 구현(trans_logic2physic.py)도 sheet_name=0/1을 썼고,
// 실물 파일의 셋째 시트(공통표준도메인)는 Phase 1c가 쓰지 않는다. 시트 이름은
// 배포본마다 달라질 수 있지만 순서는 표준 배포 형식이다.
//
// 각 시트는 1행이 헤더, A열이 한글, B열이 영문약어다. 빈 칸이 있는 행은
// 건너뛴다. 같은 한글이 두 번 나오면 앞에 있는 것이 이긴다 — 실행마다 결과가
// 달라지면 안 된다.
func Load(path string) (*Dict, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open dictionary %q: %w", path, err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) < 2 {
		return nil, fmt.Errorf("dictionary %q has %d sheet(s); the first two must be 용어사전 and 단어사전", path, len(sheets))
	}

	terms, err := loadPairs(f, sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheets[0], err)
	}
	words, err := loadPairs(f, sheets[1])
	if err != nil {
		return nil, fmt.Errorf("read sheet %q: %w", sheets[1], err)
	}
	if len(terms) == 0 && len(words) == 0 {
		return nil, fmt.Errorf("dictionary %q has no usable entries in its first two sheets", path)
	}

	d := &Dict{terms: terms, words: words}
	for k := range words {
		if n := len([]rune(k)); n > d.maxWordRunes {
			d.maxWordRunes = n
		}
	}
	return d, nil
}

func loadPairs(f *excelize.File, sheet string) (map[string]string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for i, r := range rows {
		if i == 0 || len(r) < 2 {
			continue
		}
		k, v := strings.TrimSpace(r[0]), strings.TrimSpace(r[1])
		if k == "" || v == "" {
			continue
		}
		if _, dup := m[k]; !dup {
			m[k] = v
		}
	}
	return m, nil
}

// Term은 용어사전에서 찾는다.
func (d *Dict) Term(korean string) (string, bool) {
	v, ok := d.terms[korean]
	return v, ok
}

// Word는 단어사전에서 찾는다.
func (d *Dict) Word(korean string) (string, bool) {
	v, ok := d.words[korean]
	return v, ok
}

// MaxWordRunes는 단어사전 표제어 중 가장 긴 것의 글자 수다. 분해 DP가
// 후보 구간의 상한으로 쓴다.
func (d *Dict) MaxWordRunes() int { return d.maxWordRunes }
