package pipeline

import "time"

// debouncer는 같은 파일에 대해 짧은 간격으로 겹쳐 들어오는 중복 이벤트를
// 걸러낸다. 단일 저장이 Windows에서 같은 파일에 대해 이벤트를 두 번
// 발생시키는 것이 실측으로 확인됐다(os.WriteFile은 WRITE+WRITE, 만들면서
// 쓰는 저장은 CREATE+WRITE).
//
// 시각은 처리가 «끝난» 뒤에 mark로 찍는다. 시작 시점에 찍으면 처리가
// window보다 오래 걸리는 순간(settle 재시도가 붙은 뒤로는 흔하다) 같은
// 저장의 뒤따르는 이벤트가 창을 벗어나 그대로 통과한다.
type debouncer struct {
	window time.Duration
	last   map[string]time.Time
}

func newDebouncer(window time.Duration) *debouncer {
	return &debouncer{window: window, last: make(map[string]time.Time)}
}

// allow는 지금 이 이벤트를 처리해야 하는지 답한다.
func (d *debouncer) allow(name string, now time.Time) bool {
	t, seen := d.last[name]
	return !seen || now.Sub(t) >= d.window
}

// mark는 처리가 끝났음을 기록하고, 창을 지나 쓸모없어진 항목을 치운다.
//
// 치우는 이유: Watch는 오래 떠 있는 프로세스라, 파일이 계속 새로 생기는
// 폴더를 감시하면 이름마다 항목이 쌓여 메모리가 단조증가한다. 창을 지난
// 항목은 allow가 어차피 통과시키므로 지워도 동작이 바뀌지 않는다.
func (d *debouncer) mark(name string, now time.Time) {
	for k, t := range d.last {
		if now.Sub(t) >= d.window {
			delete(d.last, k)
		}
	}
	d.last[name] = now
}

// size는 테스트가 항목 수를 확인하기 위한 것이다.
func (d *debouncer) size() int { return len(d.last) }
