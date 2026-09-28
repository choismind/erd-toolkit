package dbreverse

import (
	"testing"

	"erdtool/internal/genbuild"
)

// TestDecideCardinality_FourCases는 원장 "카디널리티" 절의 표를 그대로
// 고정한다. 표의 세 줄은 draw.io가 직접 제공하는 프리셋과 같다.
func TestDecideCardinality_FourCases(t *testing.T) {
	cases := []struct {
		notNull, unique bool
		wantParent      string
		wantChild       string
		presetName      string
	}{
		{true, false, "ERmandOne", "ERzeroToMany", "1 Mandatory to Many Optional"},
		{false, false, "ERzeroToOne", "ERzeroToMany", "1 Optional to Many Optional"},
		{true, true, "ERmandOne", "ERzeroToOne", "1 Mandatory to 1 Optional"},
		{false, true, "ERzeroToOne", "ERzeroToOne", "(프리셋 없음)"},
	}
	for _, c := range cases {
		got := DecideCardinality(c.notNull, c.unique)
		if got.Parent != c.wantParent || got.Child != c.wantChild {
			t.Errorf("DecideCardinality(notNull=%v, unique=%v) = {%q, %q}; want {%q, %q} (draw.io %q)",
				c.notNull, c.unique, got.Parent, got.Child, c.wantParent, c.wantChild, c.presetName)
		}
	}
}

// TestDecideCardinality_NeverClaimsMandatoryChildren이 이 파일의 핵심이다.
// ERoneToMany는 draw.io가 "Many Mandatory"라 부르는 것, 즉 「부모에게 자식이
// 반드시 하나 이상 있다」는 주장이다. 표준 SQL에는 그것을 선언할 문법이
// 없으므로 스키마만 읽는 역공학은 그 코드를 쓸 수 없다.
func TestDecideCardinality_NeverClaimsMandatoryChildren(t *testing.T) {
	for _, notNull := range []bool{true, false} {
		for _, unique := range []bool{true, false} {
			got := DecideCardinality(notNull, unique)
			if got.Child == "ERoneToMany" || got.Child == "ERmany" {
				t.Errorf("DecideCardinality(%v,%v).Child = %q; DB가 보장하지 않는 «자식 필수»를 주장한다",
					notNull, unique, got.Child)
			}
		}
	}
}

// TestDecideCardinality_UsesOnlyValidCodes는 genbuild가 받아들이는 어휘를
// 벗어나지 않음을 고정한다.
func TestDecideCardinality_UsesOnlyValidCodes(t *testing.T) {
	for _, notNull := range []bool{true, false} {
		for _, unique := range []bool{true, false} {
			got := DecideCardinality(notNull, unique)
			for _, code := range []string{got.Parent, got.Child} {
				if !genbuild.ValidCardinality[code] {
					t.Errorf("%q는 genbuild.ValidCardinality에 없다", code)
				}
			}
		}
	}
}
