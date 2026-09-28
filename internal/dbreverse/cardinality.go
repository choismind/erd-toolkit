package dbreverse

// Cardinality는 관계선 양 끝의 draw.io 화살표 코드다.
// Parent는 Source(부모, 참조당하는 쪽), Child는 Target(자식, FK를 가진 쪽)에
// 붙는다. 이 방향은 Phase 2a부터의 관례다.
type Cardinality struct {
	Parent string
	Child  string
}

// DecideCardinality는 FK 컬럼 집합의 성질에서 양 끝 카디널리티를 정한다.
// 이 Phase에서 «추론»이라 부를 만한 것은 이 함수 하나뿐이다.
//
//	fkNotNull: FK를 이루는 컬럼이 전부 NOT NULL인가
//	fkUnique:  FK 컬럼 집합이 유일한가(단일 컬럼 UNIQUE/PK, 또는 복합 FK가 곧 PK)
//
// 부모 쪽 끝은 «자식이 부모를 몇 개 보는가»를 말한다. FK가 NOT NULL이면
// 정확히 하나(ERmandOne, draw.io 이름 "1 Mandatory"), NULL이 허용되면
// 0 또는 1(ERzeroToOne, "0 to 1")이다. 둘 다 DB가 실제로 강제하는 사실이다.
//
// 자식 쪽 끝은 «부모가 자식을 몇 개 갖는가»를 말한다. FK 컬럼이 유일하면
// 최대 하나(ERzeroToOne), 아니면 0개 이상(ERzeroToMany, "Many Optional")이다.
//
// **ERoneToMany/ERmany는 쓰지 않는다.** draw.io가 그것들에 붙인 이름이
// "Many Mandatory"/"Many" — 즉 「부모에게 자식이 반드시 하나 이상 있다」는
// 주장인데, 표준 SQL에는 그 사실을 선언할 문법이 없다. FK도 NOT NULL도
// UNIQUE도 CHECK도 그것을 강제하지 못한다(실측: NOT NULL FK를 걸고도
// 자식이 0건인 부모가 정상 존재한다 — 원장 "카디널리티" 절).
// 데이터를 세어 판단하지도 않는다 — 지금 데이터에 없다는 것이 앞으로도
// 없어야 한다는 뜻이 아니다.
//
// 이 규칙은 원장에서 확정됐다(2026-08-27). 다만 되돌리는 비용을 낮추려고
// 이 파일 하나에 가둬 뒀다 — 바꾸기로 하면 여기만 고치면 전체가 뒤집힌다.
func DecideCardinality(fkNotNull, fkUnique bool) Cardinality {
	c := Cardinality{Parent: "ERzeroToOne", Child: "ERzeroToMany"}
	if fkNotNull {
		c.Parent = "ERmandOne"
	}
	if fkUnique {
		c.Child = "ERzeroToOne"
	}
	return c
}
