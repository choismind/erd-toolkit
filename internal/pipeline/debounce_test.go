package pipeline

import (
	"fmt"
	"testing"
	"time"
)

func TestDebouncer_SuppressesRepeatWithinWindow(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)
	now := time.Unix(0, 0)

	if !d.allow("a.drawio", now) {
		t.Fatal("처음 보는 파일은 통과해야 한다")
	}
	d.mark("a.drawio", now)

	if d.allow("a.drawio", now.Add(50*time.Millisecond)) {
		t.Fatal("창 안쪽의 중복 이벤트는 걸러져야 한다")
	}
	if !d.allow("a.drawio", now.Add(150*time.Millisecond)) {
		t.Fatal("창을 지난 뒤의 저장은 다시 통과해야 한다")
	}
}

func TestDebouncer_DoesNotGrowForever(t *testing.T) {
	// Watch는 오래 떠 있는 프로세스다. 파일이 계속 새로 생기는 폴더를
	// 감시하면 이름마다 항목이 쌓이는데, 제거 로직이 없으면 메모리가
	// 단조증가한다. 창을 지난 항목은 allow()에서 어차피 통과시키므로
	// 남겨둘 이유가 없다 — 지워도 동작이 바뀌지 않는다.
	d := newDebouncer(100 * time.Millisecond)
	now := time.Unix(0, 0)

	for i := 0; i < 10000; i++ {
		name := fmt.Sprintf("f%d.drawio", i)
		now = now.Add(10 * time.Millisecond)
		d.allow(name, now)
		d.mark(name, now)
	}

	// 10ms 간격이므로 창(100ms) 안에 들어오는 것은 10여 개뿐이다.
	if got := d.size(); got > 100 {
		t.Fatalf("창을 지난 항목이 계속 쌓인다: %d개 남음", got)
	}
}
