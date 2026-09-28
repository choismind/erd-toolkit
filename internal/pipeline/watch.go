package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"erdtool/internal/config"
)

// debounceWindow는 같은 저장에서 겹쳐 들어오는 이벤트를 걸러내는 창이다.
// 자세한 근거는 debounce.go의 debouncer 주석 참고.
const debounceWindow = 100 * time.Millisecond

// settleWindow/settleInterval은 "아직 다 써지지 않은 파일"을 기다려주는
// 한도다. fsnotify는 쓰던 쪽이 파일을 놓기 전에 이벤트를 준다 — 실제
// 바이너리로 확인한 결과 Windows에서는 공유 위반(open ...: The process
// cannot access the file because it is being used by another process),
// 그 밖에서는 잘린 내용이 읽혀 "parse mxfile: EOF"가 났다. 둘 다 멀쩡한
// 저장에서 나온 것이다.
//
// 에러 종류로 "일시적인 실패"와 "진짜 깨진 파일"을 가릴 수는 없다 — 반쯤
// 써진 파일도 깨진 파일과 똑같이 파싱 에러를 낸다. 그래서 종류를 안 따지고
// 성공할 때까지 짧게 되풀이하고, 한도를 넘으면 마지막 에러를 그대로
// 보고한다. 대가는 «진짜로» 깨진 파일의 보고가 settleWindow만큼 늦어지는
// 것인데, 저장할 때마다 가짜 실패를 띄우는 쪽이 훨씬 나쁘다(I5가 실패를
// 화면에 띄우게 만든 이상, 그 실패는 믿을 수 있어야 한다).
const (
	settleWindow   = 1500 * time.Millisecond
	settleInterval = 150 * time.Millisecond
)

// Watch는 dir을 감시하다 .drawio 쓰기 이벤트가 발생하면 같은 Generate
// 로직을 반복 호출한다("자동 트리거" — watch는 generate의 반복 호출 래퍼일
// 뿐이라는 스펙 원칙). 각 처리 결과를 events로 보내며, stop이 닫히면
// 종료한다.
//
// 실패도 반드시 events로 나간다(I5). 예전에는 `if err == nil`로 성공만
// 흘려보내서, 사용자가 저장한 파일이 깨져 있어도 화면에 아무 것도 안 뜨고
// 산출물만 조용히 그대로였다 — 왜 안 되는지 알 방법이 없었다. watcher
// 자체의 에러도 같은 방식으로 SourceFile 없이 실어 보낸다.
//
// events는 Watch가 소유한다: 반환 직전에 반드시 close하므로 호출자는
// `for r := range events`로 안전하게 소비하고 그 루프의 종료로 watch가
// 끝났음을 안다(M9 잔여 — close를 안 해서 Ctrl+C 뒤에도 호출자의 range가
// 영원히 대기했다).
//
// 전송은 언제나 stop과 함께 select한다. events가 unbuffered이거나 소비자가
// 느리면 전송에서 막히는데, 그 상태로는 stop을 영영 못 봐서 종료 요청이
// 먹히지 않는다(M9 잔여).
func Watch(dir string, cfg config.Config, events chan<- FileResult, stop <-chan struct{}) error {
	defer close(events)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// cfg.Recursive면 하위 폴더도 전부 등록한다 — fsnotify는 폴더 하나만
	// 보므로 재귀 감시는 직접 걸어야 한다. generate가 recursive를 따르는데
	// watch만 무시하면 같은 설정으로 돌린 두 명령의 대상이 조용히 달라진다.
	if err := addWatchTree(watcher, dir, cfg.Recursive); err != nil {
		return err
	}

	// send는 결과 하나를 흘려보낸다. stop이 닫혔으면 false를 돌려주어
	// 호출부가 즉시 루프를 빠져나가게 한다.
	send := func(fr FileResult) bool {
		select {
		case events <- fr:
			return true
		case <-stop:
			return false
		}
	}

	deb := newDebouncer(debounceWindow)

	// handle은 파일 하나를 처리해 결과를 흘려보낸다. 계속 돌아도 되면
	// true, stop이 닫혀 루프를 빠져나가야 하면 false를 돌려준다.
	handle := func(name string) bool {
		if !deb.allow(name, time.Now()) {
			return true
		}
		result, genErr := generateSettled(name, cfg, stop)
		deb.mark(name, time.Now())
		return send(FileResult{SourceFile: name, Result: result, Err: genErr})
	}

	for {
		select {
		case <-stop:
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// 폴더 판정이 확장자 필터보다 «먼저» 와야 한다. 폴더 이름에는
			// .drawio가 없으니, 확장자로 먼저 거르면 새 폴더를 등록할
			// 기회 자체가 없다(실제 바이너리에서 감시 시작 후 만든 폴더가
			// 조용히 빠졌다).
			if cfg.Recursive && event.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(event.Name); err == nil && st.IsDir() {
					if !handleNewDir(watcher, event.Name, handle) {
						return nil
					}
					continue
				}
			}
			// 확장자 비교는 대소문자를 구분하지 않는다 — GenerateFolder와
			// 같은 규칙이다(M12). 감시 중에만 .DRAWIO 파일이 무시되면 폴더
			// 일괄 생성 결과와 watch 결과가 서로 달라진다.
			if !strings.EqualFold(filepath.Ext(event.Name), ".drawio") {
				continue
			}
			if !handle(filepath.Clean(event.Name)) {
				return nil
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if !send(FileResult{Err: err}) {
				return nil
			}
		}
	}
}

// generateSettled는 Generate가 성공하거나 settleWindow를 넘길 때까지
// 되풀이한다. stop이 닫히면 즉시 그때까지의 결과를 돌려준다 — 종료
// 요청을 최대 settleWindow만큼 붙잡고 있지 않기 위해서다.
func generateSettled(name string, cfg config.Config, stop <-chan struct{}) (Result, error) {
	deadline := time.Now().Add(settleWindow)
	for {
		result, err := Generate(name, cfg)
		if err == nil || !time.Now().Before(deadline) {
			return result, err
		}
		select {
		case <-time.After(settleInterval):
		case <-stop:
			return result, err
		}
	}
}

// addWatchTree는 dir을 감시 대상에 넣고, recursive면 하위 폴더도 전부 넣는다.
// 산출물 폴더는 건너뛴다 — 재생성할 때마다 그 안의 쓰기 이벤트가 쏟아져
// 들어와 아무 의미 없이 루프를 돌린다.
func addWatchTree(w *fsnotify.Watcher, dir string, recursive bool) error {
	if err := w.Add(dir); err != nil {
		return err
	}
	if !recursive {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil || !e.IsDir() || path == dir {
			// 읽을 수 없는 폴더 하나가 나머지 감시를 막지 않는다.
			return nil //nolint // err는 의도적으로 삼킨다
		}
		if strings.HasSuffix(e.Name(), reportDirSuffix) {
			return fs.SkipDir
		}
		return w.Add(path)
	})
}

// handleNewDir는 감시 중에 나타난 폴더를 감시 대상에 넣고, 그 안에 «이미»
// 들어 있는 .drawio를 처리한다.
//
// 안쪽 파일까지 읽는 이유: 폴더를 통째로 복사하거나 옮겨 넣으면 폴더 하나에
// 대한 이벤트만 오고 안쪽 파일 각각에 대한 이벤트는 오지 않는다. 등록만
// 하고 끝내면 그 파일들은 다시 저장하기 전까지 영영 처리되지 않는다.
// 뒤늦게 파일 이벤트가 따라 들어오는 경우는 debouncer가 걸러낸다.
func handleNewDir(w *fsnotify.Watcher, dir string, handle func(string) bool) bool {
	// 감시 등록에 실패하더라도 이미 들어 있는 파일은 처리해준다 — 한쪽
	// 실패로 둘 다 놓치는 것이 더 나쁘다.
	_ = addWatchTree(w, dir, true)

	paths, err := findDrawioFiles(dir, true)
	if err != nil {
		return true
	}
	for _, path := range paths {
		if !handle(path) {
			return false
		}
	}
	return true
}
