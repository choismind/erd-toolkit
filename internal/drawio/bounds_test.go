package drawio

import (
	"reflect"
	"testing"
)

func TestTopLevelBounds(t *testing.T) {
	cells := []RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &RawGeometry{X: 80, Y: 40, Width: 200, Height: 100}},
		{ID: "t2", Parent: "1", Geometry: &RawGeometry{X: 400, Y: 200, Width: 160, Height: 60}},
		// 행은 t1의 자식이며 좌표가 «상대»다. 섞으면 안 된다.
		{ID: "r1", Parent: "t1", Geometry: &RawGeometry{Y: 30, Width: 200, Height: 30}},
	}
	got, ok := TopLevelBounds(cells)
	if !ok {
		t.Fatal("ok=false; true여야 한다")
	}
	want := RawGeometry{X: 80, Y: 40, Width: 480, Height: 220} // 560-80, 260-40
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bounds=%+v; %+v여야 한다", got, want)
	}
}

func TestTopLevelBoundsEmpty(t *testing.T) {
	cells := []RawCell{{ID: "0"}, {ID: "1", Parent: "0"}}
	if _, ok := TopLevelBounds(cells); ok {
		t.Error("기하를 가진 최상위 셀이 없으면 ok=false여야 한다")
	}
}

// draw.io는 원점 왼쪽·위로도 얼마든지 그릴 수 있다. 최소/최대를 0으로
// 초기화하는 흔한 실수를 하면 음수 영역이 통째로 잘려 bounds가 원점에
// 붙는데, annotate는 그 bounds 오른쪽에 요약 박스를 놓으므로 박스가
// 그림 위에 겹쳐 놓인다. 첫 셀로 초기화하는 지금 구현이 옳다는 근거다.
func TestTopLevelBoundsWithNegativeCoordinates(t *testing.T) {
	cells := []RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &RawGeometry{X: -300, Y: -200, Width: 100, Height: 50}},
		{ID: "t2", Parent: "1", Geometry: &RawGeometry{X: -40, Y: -20, Width: 60, Height: 30}},
	}
	got, ok := TopLevelBounds(cells)
	if !ok {
		t.Fatal("ok=false; true여야 한다")
	}
	want := RawGeometry{X: -300, Y: -200, Width: 320, Height: 210} // 20-(-300), 10-(-200)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bounds=%+v; %+v여야 한다 — 음수 영역이 잘렸다", got, want)
	}
}

// 음수 하나만 있어도 그 셀이 최소를 잡아야 한다.
func TestTopLevelBoundsMixesNegativeAndPositive(t *testing.T) {
	cells := []RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "t1", Parent: "1", Geometry: &RawGeometry{X: 100, Y: 100, Width: 50, Height: 50}},
		{ID: "t2", Parent: "1", Geometry: &RawGeometry{X: -80, Y: 20, Width: 40, Height: 40}},
	}
	got, ok := TopLevelBounds(cells)
	if !ok {
		t.Fatal("ok=false; true여야 한다")
	}
	want := RawGeometry{X: -80, Y: 20, Width: 230, Height: 130} // 150-(-80), 150-20
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bounds=%+v; %+v여야 한다", got, want)
	}
}

// 자기 자신을 부모로 가리키는 셀(퇴화 입력, 손편집이나 파일 병합으로만
// 만들어진다)은 최상위로 세야 한다. 「부모가 기하를 가진 셀이면 상대
// 좌표다」는 판정을 곧이곧대로 적용하면 자기 자신이 그 조건을 만족해
// 이 셀이 통째로 bounds에서 빠지고, 요약 박스가 그 위에 겹쳐 놓인다.
// 좌표가 «자기 자신에 대해 상대»일 수는 없다.
func TestTopLevelBoundsCountsSelfParentedCell(t *testing.T) {
	cells := []RawCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
		{ID: "solo", Parent: "solo", Geometry: &RawGeometry{X: 10, Y: 20, Width: 100, Height: 50}},
	}
	got, ok := TopLevelBounds(cells)
	if !ok {
		t.Fatal("ok=false; 자기순환 셀도 최상위로 세야 한다")
	}
	want := RawGeometry{X: 10, Y: 20, Width: 100, Height: 50}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bounds=%+v; %+v여야 한다", got, want)
	}
}
