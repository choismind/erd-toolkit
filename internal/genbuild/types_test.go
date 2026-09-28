package genbuild

import "testing"

func TestValidCardinality_SixCodes(t *testing.T) {
	want := []string{"ERone", "ERmandOne", "ERzeroToOne", "ERzeroToMany", "ERoneToMany", "ERmany"}
	if len(ValidCardinality) != len(want) {
		t.Fatalf("코드 개수: got %d, want %d", len(ValidCardinality), len(want))
	}
	for _, c := range want {
		if !ValidCardinality[c] {
			t.Errorf("%q가 유효 카디널리티 코드에 없다", c)
		}
	}
}

func TestValidCardinality_RejectsUnknown(t *testing.T) {
	if ValidCardinality["ERbogus"] {
		t.Fatal("정의되지 않은 코드가 true로 나오면 안 된다")
	}
}
