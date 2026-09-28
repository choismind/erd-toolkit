package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"erdtool/internal/config"
)

func TestWatch_RegeneratesOnWrite(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "watched.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", target)

	stop := make(chan struct{})
	events := make(chan FileResult, 4)
	go Watch(tmp, config.Config{}, events, stop)

	time.Sleep(200 * time.Millisecond) // watcher가 등록될 시간을 준다

	copyFile(t, "../drawio/testdata/relationship_physical.drawio", target)

	select {
	case r := <-events:
		if r.Err != nil {
			t.Fatalf("expected a successful regenerate, got error: %v", r.Err)
		}
		if r.SourceFile != target {
			t.Fatalf("expected regenerate event for %s, got %s", target, r.SourceFile)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watch to regenerate on file write")
	}
	close(stop)
}

func TestWatch_ReportsGenerateErrors(t *testing.T) {
	// I5: 저장한 파일이 깨져 있으면 Generate가 에러를 내는데 Watch가
	// `if err == nil` 로 조용히 버려서 화면에 아무 것도 안 떴다 — 사용자는
	// 산출물이 안 바뀌는 것만 보고 이유를 알 길이 없다.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "broken.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", target)

	stop := make(chan struct{})
	defer close(stop)
	events := make(chan FileResult, 4)
	go Watch(tmp, config.Config{}, events, stop)

	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(target, []byte("not even xml"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case r := <-events:
		if r.Err == nil {
			t.Fatalf("expected an error event for a broken file, got success: %+v", r)
		}
		if r.SourceFile != target {
			t.Fatalf("expected the error to name the file %s, got %q", target, r.SourceFile)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out: a broken file produced no event at all")
	}
}

func TestWatch_ClosesEventsChannelOnStop(t *testing.T) {
	// M9 잔여: Watch가 반환하면서 events를 close하지 않아 호출자의
	// `for r := range events`가 영원히 대기했다 — Ctrl+C 후에도 프로세스가
	// 스스로 끝나지 못한다.
	tmp := t.TempDir()
	stop := make(chan struct{})
	events := make(chan FileResult, 4)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range events { //nolint // 채널이 닫히면 끝나야 한다
		}
	}()

	go Watch(tmp, config.Config{}, events, stop)
	time.Sleep(200 * time.Millisecond)
	close(stop)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out: events channel was never closed, so range never ended")
	}
}

func TestWatch_StopsEvenIfNobodyReadsEvents(t *testing.T) {
	// M9 잔여: events가 unbuffered면 소비자가 없을 때 Watch가 전송에서
	// 막혀 stop을 영영 못 본다. 전송도 stop과 함께 select해야 한다.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "watched.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", target)

	stop := make(chan struct{})
	events := make(chan FileResult) // 아무도 읽지 않는 unbuffered 채널
	returned := make(chan error, 1)
	go func() { returned <- Watch(tmp, config.Config{}, events, stop) }()

	time.Sleep(200 * time.Millisecond)
	copyFile(t, "../drawio/testdata/relationship_physical.drawio", target)
	time.Sleep(300 * time.Millisecond) // Watch가 전송에서 막히도록 둔다
	close(stop)

	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out: Watch blocked on send and never saw stop")
	}
}

func TestWatch_WaitsOutAHalfWrittenFile(t *testing.T) {
	// 실측(실제 바이너리로 확인): 멀쩡한 파일을 폴더에 저장했는데 첫
	// 이벤트에서 Generate가 실패했다 — fsnotify는 쓰던 쪽이 아직 파일을
	// 잡고 있는 시점에 이벤트를 주기 때문이다. Windows에서는 공유 위반
	// ("used by another process"), 그 밖에서는 잘린 내용을 읽어 파싱
	// 에러가 난다. I5로 실패가 화면에 뜨게 된 지금, 이걸 그대로 보고하면
	// 평범한 저장 한 번마다 가짜 [FAILED]가 뜬다.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "slow.drawio")
	golden, err := os.ReadFile("../drawio/testdata/relationship_physical.drawio")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{}, events, stop)
	// 감시를 «확실히» 끝내고 나서 t.TempDir 정리가 돌게 한다. 그냥
	// close(stop)만 하면 Watch가 아직 산출물을 쓰는 중일 수 있어
	// RemoveAll이 "directory is not empty"로 깨진다. events가 닫히는
	// 것이 Watch가 반환했다는 신호다.
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(200 * time.Millisecond)

	// 느린 저장을 흉내낸다: 파일을 만들어 비워둔 채 이벤트를 띄우고,
	// 진짜 내용은 잠시 뒤에야 쓴다.
	f, err := os.Create(target)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := f.Write(golden); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	select {
	case r := <-events:
		if r.Err != nil {
			t.Fatalf("아직 다 써지지 않은 파일에 가짜 실패를 보고했다: %v", r.Err)
		}
		if r.Result.TableCount == 0 {
			t.Fatalf("성공은 했는데 테이블이 하나도 없다: %+v", r.Result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: 저장이 끝났는데도 아무 이벤트가 없다")
	}
}

func TestWatch_OneSaveProducesOneEvent(t *testing.T) {
	// 실측: 저장 한 번에 [REGENERATED]가 두 줄씩 찍혔다. Windows는 저장
	// 한 번에 WRITE 이벤트를 두 번 이상 준다(데이터 쓰기 + 핸들 close).
	// 디바운스 시각을 처리 «시작» 시점에 찍으면, 처리가 debounceWindow보다
	// 오래 걸리는 순간(= settle 재시도가 붙은 지금은 늘 그렇다) 뒤따르던
	// 중복 이벤트가 창을 벗어나 그대로 통과한다. 시각은 처리가 «끝난» 뒤에
	// 찍어야 한다.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "once.drawio")

	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{}, events, stop)
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(200 * time.Millisecond)

	// Copy-Item이나 draw.io의 저장과 같은 모양으로 만든다: 파일을 만들고
	// (CREATE) 곧바로 내용을 쓴 뒤 닫는다(WRITE). 이벤트 두 개가 몇 ms
	// 간격으로 들어온다 — os.WriteFile 한 방은 이벤트가 하나뿐이라 이
	// 상황을 재현하지 못한다.
	golden, err := os.ReadFile("../drawio/testdata/relationship_physical.drawio")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	f, err := os.Create(target)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 내용을 조금 늦게 쓴다 — 그래야 첫 이벤트의 처리가 settle 재시도를
	// 거치며 debounceWindow보다 오래 걸리고, 뒤이은 WRITE 이벤트가 창을
	// 벗어난다. 실제 바이너리에서 Copy-Item으로 저장했을 때가 정확히 이
	// 모양이었다(빈 파일 CREATE → 내용 → close).
	time.Sleep(300 * time.Millisecond)
	if _, err := f.Write(golden); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	select {
	case r := <-events:
		if r.Err != nil {
			t.Fatalf("expected a successful regenerate, got error: %v", r.Err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: 저장했는데 이벤트가 없다")
	}

	select {
	case r := <-events:
		t.Fatalf("저장 한 번에 이벤트가 두 번 나왔다: %+v", r)
	case <-time.After(1500 * time.Millisecond):
	}
}

func TestWatch_RecursiveWatchesSubdirs(t *testing.T) {
	// I9: generate가 recursive를 따르는데 watch만 무시하면, 같은
	// erdtool.yaml로 돌렸을 때 두 명령의 대상이 조용히 달라진다.
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{Recursive: true}, events, stop)
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(300 * time.Millisecond)
	target := filepath.Join(sub, "nested.drawio")
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", target)

	select {
	case r := <-events:
		if r.Err != nil {
			t.Fatalf("하위 폴더 저장이 실패로 보고됐다: %v", r.Err)
		}
		if r.SourceFile != target {
			t.Fatalf("다른 파일 이벤트다: %s", r.SourceFile)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: 하위 폴더 저장이 감지되지 않았다")
	}
}

func TestWatch_NonRecursiveIgnoresSubdirs(t *testing.T) {
	// 기본은 비재귀다(사용자 결정). 하위 폴더 저장은 아무 일도 일으키지
	// 않아야 한다.
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{}, events, stop)
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(300 * time.Millisecond)
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", filepath.Join(sub, "nested.drawio"))

	select {
	case r := <-events:
		t.Fatalf("비재귀인데 하위 폴더 이벤트가 나왔다: %+v", r)
	case <-time.After(2 * time.Second):
	}
}

func TestWatch_RecursivePicksUpNewSubdir(t *testing.T) {
	// 실측(실제 바이너리): 감시를 시작한 «뒤에» 만든 폴더는 감시에서 조용히
	// 빠졌다. 확장자 필터가 폴더 이벤트를 먼저 걸러내서 새 폴더를 등록할
	// 기회 자체가 없었다.
	tmp := t.TempDir()
	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{Recursive: true}, events, stop)
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(300 * time.Millisecond)
	fresh := filepath.Join(tmp, "brandnew")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // 새 폴더가 등록될 시간을 준다
	target := filepath.Join(fresh, "later.drawio")
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", target)

	select {
	case r := <-events:
		if r.Err != nil || r.SourceFile != target {
			t.Fatalf("새 폴더의 저장이 제대로 잡히지 않았다: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: 감시 시작 후 만든 폴더가 감시되지 않았다")
	}
}

func TestWatch_RecursivePicksUpMovedInFolder(t *testing.T) {
	// 폴더를 통째로 «붙여넣는» 경우 — 파일이 이미 안에 들어있는 채로
	// 폴더 하나가 나타난다. 안쪽 파일 각각에 대한 이벤트는 오지 않으므로,
	// 새 폴더를 등록만 하고 끝내면 그 파일들은 영영 처리되지 않는다.
	tmp := t.TempDir()
	staging := t.TempDir()
	ready := filepath.Join(staging, "ready")
	if err := os.MkdirAll(ready, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", filepath.Join(ready, "inside.drawio"))

	stop := make(chan struct{})
	events := make(chan FileResult, 8)
	go Watch(tmp, config.Config{Recursive: true}, events, stop)
	t.Cleanup(func() {
		close(stop)
		for range events { //nolint // 채널이 닫힐 때까지 비운다
		}
	})

	time.Sleep(300 * time.Millisecond)
	moved := filepath.Join(tmp, "ready")
	if err := os.Rename(ready, moved); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-events:
		want := filepath.Join(moved, "inside.drawio")
		if r.Err != nil || r.SourceFile != want {
			t.Fatalf("통째로 들어온 폴더 안의 파일이 처리되지 않았다: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: 폴더째 들어온 .drawio가 처리되지 않았다")
	}
}
