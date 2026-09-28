// internal/validate/optional_test.go
package validate

import (
	"testing"

	"erdtool/internal/config"
	"erdtool/internal/model"
)

func TestOptional_ANSITypeViolation(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "물리ERD",
		Tables: []model.Table{{Name: "T", Columns: []model.Column{
			{Name: "id", Type: "int", Key: "PK"},
			{Name: "weird", Type: "한글타입"},
		}}},
	}}}
	cfg := config.Config{}
	cfg.Validation.ANSISQLTypes = true

	findings := Optional(doc, cfg)
	if !hasRule(findings, "non_ansi_type") {
		t.Fatalf("expected non_ansi_type finding, got %+v", findings)
	}
}

func TestOptional_DisabledPerPage(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "논리ERD",
		Tables: []model.Table{{Name: "T", Columns: []model.Column{
			{Name: "고객명", Type: "문자열"}, // 논리 ERD라 비-ANSI 타입이 정상
		}}},
	}}}
	cfg := config.Config{Pages: map[string]config.PageOverride{
		"논리ERD": {Validation: nil},
	}}
	cfg.Validation.ANSISQLTypes = false // 전역에서 이미 꺼짐

	findings := Optional(doc, cfg)
	if hasRule(findings, "non_ansi_type") {
		t.Fatalf("expected no non_ansi_type finding when validation disabled, got %+v", findings)
	}
}

func TestOptional_NamingConvention(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "물리ERD",
		Tables: []model.Table{{Name: "T", Columns: []model.Column{
			{Name: "customerName", Type: "varchar"}, // camelCase, snake_case 아님
		}}},
	}}}
	cfg := config.Config{}
	cfg.Validation.NamingConvention = true

	findings := Optional(doc, cfg)
	if !hasRule(findings, "naming_convention") {
		t.Fatalf("expected naming_convention finding, got %+v", findings)
	}
}

func TestOptional_NamingConventionAcceptsUpperSnake(t *testing.T) {
	// 실측: 픽스처를 실제로 돌려보니 CUST_NO/ORDR_YMD 같은 평범한 물리
	// 컬럼명이 전부 위반으로 잡혔다. 한국 공공/기업 물리 ERD는 대문자
	// 스네이크가 관례라, 이대로면 규칙을 켜는 순간 모든 컬럼에 warning이
	// 붙어 리포트가 쓸모없어진다. 규칙의 뜻은 "표기를 일관되게 쓰라"이지
	// "소문자를 쓰라"가 아니므로, 대문자 스네이크도 통과시킨다.
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "물리ERD",
		Tables: []model.Table{{Name: "T", Columns: []model.Column{
			{Name: "CUST_NO", Type: "int"},
			{Name: "cust_no", Type: "int"},
		}}},
	}}}
	cfg := config.Config{}
	cfg.Validation.NamingConvention = true

	if findings := Optional(doc, cfg); hasRule(findings, "naming_convention") {
		t.Fatalf("대·소문자 스네이크는 둘 다 통과해야 한다, got %+v", findings)
	}
}

func TestOptional_NamingConventionStillFlagsMixedCase(t *testing.T) {
	// 대문자를 허용한다고 해서 규칙이 무력해지면 안 된다. 섞어 쓰는 표기
	// (camelCase/PascalCase)는 그대로 잡아야 한다.
	for _, name := range []string{"customerName", "CustomerName", "CustNo"} {
		doc := model.Document{Diagrams: []model.Diagram{{
			Name:   "물리ERD",
			Tables: []model.Table{{Name: "T", Columns: []model.Column{{Name: name, Type: "varchar"}}}},
		}}}
		cfg := config.Config{}
		cfg.Validation.NamingConvention = true

		if findings := Optional(doc, cfg); !hasRule(findings, "naming_convention") {
			t.Fatalf("%q는 여전히 잡혀야 한다, got %+v", name, findings)
		}
	}
}

// TestOptional_MultiWordANSITypesAreNotFlagged는 Phase 2b가 연 자리를 막는다.
// drawio 파서가 오른쪽 기준이 되기 전에는 타입에서 첫 낱말만 읽혀
// `character varying(255)`가 `character`로 잘렸고, 이 검사에는 잘린 값이
// 닿았다. 이제 원문이 그대로 오므로 «표준인데 표준이 아니라고 말하는»
// 오경보가 생길 수 있다 — 역공학한 PostgreSQL DB에서는 거의 모든 컬럼이다.
func TestOptional_MultiWordANSITypesAreNotFlagged(t *testing.T) {
	types := []string{
		"character varying(255)",
		"character(10)",
		"timestamp with time zone",
		"timestamp without time zone",
		"time with time zone",
		"double precision",
		"bit varying(8)",
		"numeric (10,2)", // 괄호 앞 공백까지 허용해야 한다
	}
	var cols []model.Column
	for i, ty := range types {
		cols = append(cols, model.Column{Name: "c", Type: ty})
		_ = i
	}
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "물리ERD", Tables: []model.Table{{Name: "T", Columns: cols}},
	}}}
	cfg := config.Config{}
	cfg.Validation.ANSISQLTypes = true

	if findings := Optional(doc, cfg); hasRule(findings, "non_ansi_type") {
		t.Errorf("표준 타입을 표준이 아니라고 했다: %+v", findings)
	}
}

// 그렇다고 아무거나 통과시키면 안 된다. MySQL의 int unsigned는 실제로
// ANSI가 아니다 — 검사가 여전히 잡아야 한다.
func TestOptional_NonStandardMultiWordTypeIsStillFlagged(t *testing.T) {
	doc := model.Document{Diagrams: []model.Diagram{{
		Name: "물리ERD", Tables: []model.Table{{Name: "T", Columns: []model.Column{
			{Name: "qty", Type: "int unsigned"},
		}}},
	}}}
	cfg := config.Config{}
	cfg.Validation.ANSISQLTypes = true

	if findings := Optional(doc, cfg); !hasRule(findings, "non_ansi_type") {
		t.Errorf("int unsigned는 ANSI가 아니다: %+v", findings)
	}
}
