package annotate

import (
	"reflect"
	"testing"

	"erdtool/internal/validate"
)

func TestBuildPlansMarksOnlyAnchoredWarnings(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "테이블 \"주문\"에 PK가 없음", DiagramID: "pg1", CellID: "t1"},
		// info는 마크하지 않는다. 오늘 info를 내는 규칙은 없지만 Severity에
		// 자리가 있으므로 그 갈래를 여기서 고정해 둔다.
		{Rule: "info_sample", Severity: validate.SeverityInfo,
			Message: "안내 한 줄", DiagramID: "pg1"},
		{Rule: "duplicate_table_name", Severity: validate.SeverityWarning,
			Message: "동일 이름 테이블 \"고객\"", DiagramID: "", CellID: ""},
	}
	plans := BuildPlans(fs, []string{"pg1"})
	if len(plans) != 1 {
		t.Fatalf("plan %d개; 1개여야 한다", len(plans))
	}
	p := plans[0]

	if len(p.Marks) != 1 {
		t.Fatalf("마크 %d개; 1개여야 한다 (info와 앵커 없는 건 마크 안 한다)", len(p.Marks))
	}
	want := []string{"테이블 \"주문\"에 PK가 없음"}
	if !reflect.DeepEqual(p.Marks["t1"].Messages, want) {
		t.Errorf("t1의 메시지=%v; %v여야 한다", p.Marks["t1"].Messages, want)
	}
	// 요약에는 셋 다 들어간다.
	if len(p.Summary) != 3 {
		t.Errorf("요약 %d줄; 3줄이어야 한다: %v", len(p.Summary), p.Summary)
	}
}

func TestBuildPlansSkipsPageWithNoFindings(t *testing.T) {
	plans := BuildPlans(nil, []string{"pg1", "pg2"})
	if len(plans) != 0 {
		t.Errorf("plan %d개; 0개여야 한다 — 진단 없는 페이지엔 아무것도 안 만든다", len(plans))
	}
}

// 한 셀에 진단이 여럿이면 순서가 실행마다 같아야 한다.
func TestBuildPlansKeepsMessageOrder(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "non_ansi_type", Severity: validate.SeverityWarning,
			Message: "첫째", DiagramID: "pg1", CellID: "c1"},
		{Rule: "naming_convention", Severity: validate.SeverityWarning,
			Message: "둘째", DiagramID: "pg1", CellID: "c1"},
	}
	p := BuildPlans(fs, []string{"pg1"})[0]
	if !reflect.DeepEqual(p.Marks["c1"].Messages, []string{"첫째", "둘째"}) {
		t.Fatalf("순서가 다르다: %v", p.Marks["c1"].Messages)
	}
}

// 페이지 순서는 order 슬라이스를 그대로 따라야 한다 — findings의 등장
// 순서나 내부 맵의 순회 순서를 따르면 같은 입력에도 실행마다 다른 바이트가
// 나온다. 여기가 이 파일에서 실제로 맵이 끼어드는 유일한 자리다.
//
// (예전에는 위 메시지 순서 테스트가 같은 단언을 20번 반복했다. 메시지
// 순서는 findings 슬라이스를 그대로 append하는 것이라 맵이 전혀 안
// 끼어들어, 그 루프는 몇 번을 돌려도 언제나 같은 답을 냈다 — 무작위성을
// 잡는 것처럼 보였지만 실제로는 첫 회차의 단언 하나를 19번 다시 확인한
// 것뿐이다. 이 저장소가 반복해서 데인 «코드를 지워도 통과하는 단언»과
// 같은 계열이라 루프를 걷어내고, 진짜 맵이 끼어드는 자리를 여기서 본다.)
func TestBuildPlansKeepsPageOrder(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "다", DiagramID: "pg3", CellID: "c1"},
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "가", DiagramID: "pg1", CellID: "c1"},
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "나", DiagramID: "pg2", CellID: "c1"},
	}
	order := []string{"pg1", "pg2", "pg3"}

	// 맵 순회 순서는 실행마다 다르므로 한 번 통과한 것이 우연일 수 있다.
	for i := 0; i < 50; i++ {
		plans := BuildPlans(fs, order)
		got := make([]string, 0, len(plans))
		for _, p := range plans {
			got = append(got, p.DiagramID)
		}
		if !reflect.DeepEqual(got, order) {
			t.Fatalf("%d회차 페이지 순서가 order와 다르다: %v", i, got)
		}
	}
}

// order에 없는 DiagramID를 가진 finding은 조용히 버려져야 한다 — order는
// 파싱된 문서에서 오므로, 이 경우가 실제로 생기면 호출자 쪽 버그지만
// 여기서 패닉하면 안 된다(방어적).
func TestBuildPlansDropsUnknownDiagramID(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "알 수 없는 페이지의 진단", DiagramID: "ghost", CellID: "t1"},
	}
	plans := BuildPlans(fs, []string{"pg1"})
	if len(plans) != 0 {
		t.Fatalf("plan %d개; 0개여야 한다 — order에 없는 페이지는 버려야 한다: %v", len(plans), plans)
	}
}

// 여러 페이지가 있을 때, 페이지별 진단은 그 페이지의 요약에만 들어가고
// DiagramID가 빈 진단은 모든 페이지의 요약에 들어가야 한다.
func TestBuildPlansMultiPageSummaryScoping(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "pg1 전용 진단", DiagramID: "pg1", CellID: "t1"},
		{Rule: "duplicate_table_name", Severity: validate.SeverityWarning,
			Message: "모든 페이지 공통 진단", DiagramID: "", CellID: ""},
	}
	plans := BuildPlans(fs, []string{"pg1", "pg2"})
	if len(plans) != 2 {
		t.Fatalf("plan %d개; 2개여야 한다: %v", len(plans), plans)
	}

	byID := map[string]PagePlan{}
	for _, p := range plans {
		byID[p.DiagramID] = p
	}

	pg1 := byID["pg1"]
	wantPg1 := []string{"pg1 전용 진단", "모든 페이지 공통 진단"}
	if !reflect.DeepEqual(pg1.Summary, wantPg1) {
		t.Errorf("pg1 요약=%v; %v여야 한다", pg1.Summary, wantPg1)
	}

	pg2 := byID["pg2"]
	wantPg2 := []string{"모든 페이지 공통 진단"}
	if !reflect.DeepEqual(pg2.Summary, wantPg2) {
		t.Errorf("pg2 요약=%v; %v여야 한다", pg2.Summary, wantPg2)
	}
}

// BuildPlans는 findings 인자를 변경하면 안 된다 — 호출자가 같은 슬라이스를
// 다른 용도로 재사용할 수 있다.
func TestBuildPlansDoesNotMutateInput(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "진단", DiagramID: "pg1", CellID: "t1"},
	}
	before := make([]validate.Finding, len(fs))
	copy(before, fs)

	_ = BuildPlans(fs, []string{"pg1"})

	if !reflect.DeepEqual(fs, before) {
		t.Errorf("findings가 변경됐다: %v; 원래는 %v여야 한다", fs, before)
	}
}

// id 없는 <diagram>의 진단은 어느 페이지에도 새어 나가면 안 된다.
//
// validate는 그런 페이지의 finding에도 DiagramID를 d.ID(=빈 문자열)로 적는다.
// 「DiagramID가 비었다」만 보고 «페이지를 가로지르는 사실»로 읽으면, 쓸 수도
// 없는 페이지의 진단이 남은 모든 페이지의 요약 박스에 실려 사용자에게 보인다.
// 진짜로 페이지를 가로지르는 진단(duplicate_table_name)은 DiagramName도
// 비어 있다 — 페이지 이름을 Message 안에만 담기 때문이다. 그것이 둘을
// 가르는 신호다.
func TestBuildPlansDropsFindingFromIDLessPage(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: `테이블 "주문"에 PK가 없음`, DiagramName: "첫 페이지", DiagramID: "", CellID: "t1"},
		{Rule: "missing_primary_key", Severity: validate.SeverityWarning,
			Message: "pg2 전용 진단", DiagramName: "둘째 페이지", DiagramID: "pg2", CellID: "t9"},
	}
	plans := BuildPlans(fs, []string{"pg2"})
	if len(plans) != 1 {
		t.Fatalf("plan %d개; 1개여야 한다: %v", len(plans), plans)
	}
	want := []string{"pg2 전용 진단"}
	if !reflect.DeepEqual(plans[0].Summary, want) {
		t.Errorf("pg2 요약=%v; %v여야 한다 — id 없는 페이지의 진단이 새어 나왔다",
			plans[0].Summary, want)
	}
	if _, marked := plans[0].Marks["t1"]; marked {
		t.Error("id 없는 페이지의 셀 t1이 pg2에서 마크됐다")
	}
}

// 대상 밖 도형은 심각도가 «안내»인데도 마크한다. 마크 대상을 심각도만으로
// 고르면(«경고만 마크한다») 회색 테두리가 통째로 사라지는데, 리포트는
// 그대로라 아무 테스트도 안 깨진다 — 2026-09-06에 소유자가 정한 «읽지 않은
// 도형은 회색으로 칠한다»가 조용히 없어지는 자리라 여기서 못박는다.
func TestBuildPlans대상밖은안내여도마크한다(t *testing.T) {
	fs := []validate.Finding{
		{Rule: "shape_violation", Severity: validate.SeverityInfo,
			Message: "ellipse 도형은 읽지 않았다", OutOfScope: true,
			DiagramID: "pg1", CellID: "e1"},
	}
	p := BuildPlans(fs, []string{"pg1"})[0]

	m, ok := p.Marks["e1"]
	if !ok {
		t.Fatalf("e1이 마크되지 않았다 — 대상 밖 도형은 회색 테두리를 받아야 한다: %+v", p.Marks)
	}
	if !m.OutOfScope {
		t.Error("OutOfScope=false; 회색이 아니라 빨강으로 칠해진다")
	}
}
