package model

import "strings"

// keyTokens는 키 셀 값("PK", "pk", "PK, FK1", "PK/FK" 등)을 개별 키 토큰으로
// 쪼갠다. 대소문자와 공백은 무시한다 — draw.io에서 사람이 직접 타이핑하는
// 값이라 표기가 제각각이다.
func keyTokens(key string) []string {
	upper := strings.ToUpper(key)
	return strings.FieldsFunc(upper, func(r rune) bool {
		return r == ',' || r == '/' || r == '+' || r == '|' || r == ' ' || r == '\t'
	})
}

func hasKeyPrefix(key, prefix string) bool {
	for _, tok := range keyTokens(key) {
		if strings.HasPrefix(tok, prefix) {
			return true
		}
	}
	return false
}

// IsPrimaryKey는 이 컬럼이 기본키 구성 컬럼인지 판정한다. 판정 로직이
// tables.go/required.go/sql.go 세 곳에 각기 다른 대소문자 규칙으로 복제되어
// 있던 것을 이 한 곳으로 통합했다(M1) — 같은 다이어그램을 두고 검증
// 리포트는 "PK 없음"이라 하고 SQL DDL은 PRIMARY KEY를 뽑는 식의 불일치를
// 구조적으로 막는다.
func (c Column) IsPrimaryKey() bool { return hasKeyPrefix(c.Key, "PK") }

// IsForeignKey는 이 컬럼이 외래키 구성 컬럼인지 판정한다. "PK, FK1"처럼
// 두 역할을 겸하는 값도 있으므로 IsPrimaryKey와 배타적이지 않다.
func (c Column) IsForeignKey() bool { return hasKeyPrefix(c.Key, "FK") }

// IsKey는 이 컬럼이 PK/FK 중 어느 하나라도 해당하는지 판정한다. 키 셀에
// 실제로 키 표기가 들어있는지 확인하는 용도(rowspan 병합 상속 판정 등).
func (c Column) IsKey() bool { return c.IsPrimaryKey() || c.IsForeignKey() }

// ForeignKeyTokens는 키 셀에 적힌 FK 표기를 원문 그대로 돌려준다("FK1",
// "FK1 FK2"). 정의서의 FK 칸이 이 값을 쓴다 — 번호를 ●로 바꾸면 어느 관계에
// 딸린 외래키인지가 사라진다.
func (c Column) ForeignKeyTokens() string { return joinTokens(c.Key, "FK") }

// UnrecognizedKeyTokens는 키 셀에 적혀 있으나 PK도 FK도 아닌 낱말이다.
// 오타("P K"의 "P"와 "K")나 이 도구가 모르는 표기가 여기 잡힌다.
//
// 정의서가 키 셀을 PK 칸과 FK 칸으로 나눠 담으면서 필요해졌다. 나누는 순간
// 어느 칸에도 안 들어가는 값이 생기는데, **표에서 조용히 사라지면** 사용자는
// 자기가 그 칸에 무엇을 썼는지 문서만 보고는 알 수 없다.
func (c Column) UnrecognizedKeyTokens() string {
	var out []string
	for _, tok := range keyTokens(c.Key) {
		if strings.HasPrefix(tok, "PK") || strings.HasPrefix(tok, "FK") {
			continue
		}
		out = append(out, tok)
	}
	return strings.Join(out, " ")
}

// joinTokens는 키 셀 토큰 중 접두어가 맞는 것만 원래 순서대로 잇는다.
// 대문자로 정규화된 토큰을 쓴다 — 키 셀은 사람이 직접 타이핑하는 값이라
// 표기가 제각각인데(pk, Fk1), 정의서에 그 흔들림을 그대로 내보내면 같은
// 뜻인 줄이 서로 달라 보인다.
func joinTokens(key, prefix string) string {
	var out []string
	for _, tok := range keyTokens(key) {
		if strings.HasPrefix(tok, prefix) {
			out = append(out, tok)
		}
	}
	return strings.Join(out, " ")
}
