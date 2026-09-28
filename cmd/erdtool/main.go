package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"erdtool/internal/buildinfo"
	"erdtool/internal/config"
	"erdtool/internal/convert"
	"erdtool/internal/dialect"
	"erdtool/internal/pipeline"
)

// 버전 문자열은 internal/buildinfo에 있다. 산출물 머리에 «무엇으로 뽑았는지»를
// 적으려면 리포터도 그 값을 봐야 하는데, cmd는 internal 패키지들이 볼 수 없는
// 위쪽이라 여기 두면 닿지 않는다.

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: erdtool <generate|convert|build|reverse|annotate|watch|version> ...")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version":
		runVersion(os.Args[2:])
	case "generate":
		runGenerate(os.Args[2:])
	case "convert":
		runConvert(os.Args[2:])
	case "build":
		runBuild(os.Args[2:])
	case "reverse":
		runReverse(os.Args[2:])
	case "annotate":
		runAnnotate(os.Args[2:])
	case "watch":
		runWatch(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// runVersion은 버전을 찍는다. **플래그도 위치 인자도 받지 않으므로 무엇이든
// 오면 거부한다** — 「모르는 플래그는 어느 커맨드든 거부한다」가 이 커맨드에서만
// 예외가 되면 그 문장이 거짓이 된다. 2026-09-22 이전에는 실제로 그랬다:
// `erdtool version --bogus`가 모르는 플래그를 조용히 무시하고 종료 코드 0으로
// 끝났다.
//
// 빈 cmdFlags로 파싱하는 것이 요점이다. 검사를 따로 짜면 문구가 다른 여섯과
// 갈리고, 나중에 한쪽만 고쳐진다.
func runVersion(args []string) {
	if _, err := (cmdFlags{}).parse(args); err != nil {
		die(err)
	}
	fmt.Println("erdtool " + buildinfo.Version)
}

// strFlag는 값을 하나 받는 플래그다. noun은 값을 빼먹었을 때의 문구에
// 들어간다("--out requires a directory argument") — 같은 --out이라도
// build는 파일을, convert는 폴더를 받으므로 서브커맨드마다 다르다.
type strFlag struct {
	dst  *string
	noun string
}

// cmdFlags는 서브커맨드 하나의 플래그 정의다.
//
// 왜 이 타입이 있나: 예전에는 여섯 서브커맨드가 각자 같은 모양의
// for-switch를 갖고 있었다. 플래그를 하나 더하거나 문구를 고칠 때 여섯
// 곳을 함께 고쳐야 했고, 실제로 그 복제 때문에 runGenerate와 runWatch가
// 나란히 default: 갈래를 빠뜨려 «모르는 플래그를 삼키고 exit 0»을 냈다
// (2026-09-01에 닫았다). 그 루프는 os.Exit을 안고 있어 단위 테스트도
// 불가능했다 — 정확히 조용히 틀렸던 그 코드가 그물 밖에 있었던 셈이다.
//
// 그래서 여기서는 os.Exit 대신 error를 돌려준다. 찍고 죽는 것은 호출자
// 몫이고, 파싱 자체는 테스트가 볼 수 있다.
type cmdFlags struct {
	strs  map[string]strFlag
	bools map[string]*bool
	// positional이 true면 플래그가 아닌 인자를 모아 돌려준다(build/reverse).
	// false면 그것도 «모르는 플래그»로 거부한다 — generate/watch/convert/
	// annotate는 대상 경로를 args[0]에서 이미 떼어 갔으므로 남는 위치
	// 인자가 없어야 맞다.
	positional bool
}

// parse는 args를 왼쪽부터 읽는다. 첫 번째 잘못에서 멈추는 것이 중요하다 —
// 모아서 나중에 보고하면 사용자가 고칠 곳과 화면에 뜬 이름이 어긋난다.
func (f cmdFlags) parse(args []string) ([]string, error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if sf, ok := f.strs[a]; ok {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("%s requires a %s argument", a, sf.noun)
			}
			i++
			*sf.dst = args[i]
			continue
		}
		if b, ok := f.bools[a]; ok {
			*b = true
			continue
		}
		if !f.positional || strings.HasPrefix(a, "--") {
			return nil, fmt.Errorf("unknown flag: %s", a)
		}
		rest = append(rest, a)
	}
	return rest, nil
}

// die는 에러를 찍고 종료한다. 이 층의 문구가 영어인 것은 의도된 관례다
// (결과·통계 줄만 한국어).
func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// resolveConfig는 이번 실행에 쓸 설정을 정한다. --config를 명시했으면 그것만
// 쓰고 자동탐색은 아예 하지 않는다 — 옆에 놓인 erdtool.yaml이 조용히 끼어들면
// "왜 내가 준 설정이 안 먹지"가 된다. 명시가 없을 때만 대상 폴더에서
// erdtool.yaml을 찾는다. 설정이 없는 것은 에러가 아니다.
//
// 두 번째 반환값은 «실제로 쓰인 설정 파일 경로»다. 자동으로 읽힌 설정이
// 화면에 안 보이면 사용자는 산출물이 왜 달라졌는지 알 수 없다.
func resolveConfig(explicit, dir string) (config.Config, string, error) {
	path := explicit
	if path == "" {
		found, ok := config.Discover(dir)
		if !ok {
			return config.Config{}, "", nil
		}
		path = found
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, "", err
	}
	return cfg, path, nil
}

func runGenerate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: erdtool generate <file-or-dir> [--config path] [--dialect name] [--relations] [--sql] [--recursive]")
		os.Exit(1)
	}
	target := args[0]
	var configPath, wantDialect string
	var wantRelations, wantSQL, wantRecursive bool
	if _, err := (cmdFlags{
		strs: map[string]strFlag{
			"--config":  {&configPath, "path"},
			"--dialect": {&wantDialect, "name"},
		},
		bools: map[string]*bool{
			"--relations": &wantRelations,
			"--sql":       &wantSQL,
			"--recursive": &wantRecursive,
		},
	}).parse(args[1:]); err != nil {
		die(err)
	}

	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot access %s: %v\n", target, err)
		os.Exit(1)
	}

	// 자동탐색은 «대상이 있는 폴더»에서 한다 — 파일을 지정했으면 그 파일이
	// 있는 폴더다.
	lookIn := target
	if !info.IsDir() {
		lookIn = filepath.Dir(target)
	}
	cfg, usedConfig, err := resolveConfig(configPath, lookIn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}
	if usedConfig != "" && configPath == "" {
		// 자동으로 읽힌 설정은 반드시 알린다. 명시한 경우는 사용자가 이미
		// 알고 있으므로 굳이 찍지 않는다.
		fmt.Printf("using config %s\n", usedConfig)
	}

	// CLI 플래그는 그 실행 1회에 한해 설정 파일 값을 덮어쓴다(원장: 설정 파일과
	// CLI). 플래그가 켜져 있으면 무조건 켠다 — 끄는 플래그는 없다(옵션 산출물은
	// 기본이 꺼짐이므로 "켜는" 방향만 필요).
	if wantRelations {
		cfg.Outputs.RelationDoc = true
	}
	if wantSQL {
		cfg.Outputs.SQLDDL = true
	}
	if wantRecursive {
		cfg.Recursive = true
	}
	// --dialect는 «켜는» 플래그가 아니라 값이라, 준 경우에만 덮어쓴다.
	// 여기서 미리 검사해 두면 폴더 처리에서 파일마다 같은 에러를 반복하지
	// 않는다.
	if wantDialect != "" {
		if _, err := dialect.Parse(wantDialect); err != nil {
			die(err)
		}
		cfg.Dialect = wantDialect
	}

	if info.IsDir() {
		results, err := pipeline.GenerateFolder(target, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
			os.Exit(1)
		}
		if printFolderResults(os.Stdout, os.Stderr, results) {
			// 폴더 안의 파일 하나라도 실패했으면 그 사실이 종료 코드에
			// 남아야 한다 — watch와 같은 규칙이다. 화면의 [SKIP] 한 줄은
			// 스크립트/CI에서 아무도 못 본다.
			os.Exit(1)
		}
		return
	}

	result, err := pipeline.Generate(target, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] %s -> %s (%d tables)\n", target, result.OutputDir, result.TableCount)
}

func runWatch(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: erdtool watch <dir> [--config path] [--recursive]")
		os.Exit(1)
	}
	dir := args[0]
	var configPath string
	var wantRecursive bool
	if _, err := (cmdFlags{
		strs:  map[string]strFlag{"--config": {&configPath, "path"}},
		bools: map[string]*bool{"--recursive": &wantRecursive},
	}).parse(args[1:]); err != nil {
		die(err)
	}

	cfg, usedConfig, err := resolveConfig(configPath, dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}
	if usedConfig != "" && configPath == "" {
		fmt.Printf("using config %s\n", usedConfig)
	}
	if wantRecursive {
		cfg.Recursive = true
	}

	// Ctrl+C(및 SIGTERM)를 받으면 stop을 닫아 Watch가 스스로 빠져나오게
	// 한다. Watch는 반환하면서 events를 close하므로 아래 range가 끝나고
	// 함수가 정상 반환한다 — 프로세스를 그 자리에서 죽이지 않기 때문에
	// 마지막으로 쓰던 산출물이 중간에 잘리지 않는다.
	events := make(chan pipeline.FileResult, 16)
	stop := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Fprintln(os.Stderr, "\nstopping...")
		close(stop)
	}()

	watchErr := make(chan error, 1)
	go func() { watchErr <- pipeline.Watch(dir, cfg, events, stop) }()

	scope := "for .drawio changes"
	if cfg.Recursive {
		scope = "and its subfolders for .drawio changes"
	}
	fmt.Printf("watching %s %s (Ctrl+C to stop)\n", dir, scope)
	failed := false
	for r := range events {
		switch {
		case r.Err != nil && r.SourceFile == "":
			// 감시자 자체의 에러(파일 하나에 귀속되지 않음).
			fmt.Fprintf(os.Stderr, "[WATCH ERROR] %v\n", r.Err)
			failed = true
		case r.Err != nil:
			// 저장한 파일이 잘못됐다는 걸 반드시 보여준다. 감시는
			// 계속한다 — 사용자가 고쳐서 다시 저장하면 그때 성공한다.
			fmt.Fprintf(os.Stderr, "[FAILED] %s: %v\n", r.SourceFile, r.Err)
			failed = true
		default:
			fmt.Printf("[REGENERATED] %s -> %s (%d tables)\n",
				r.SourceFile, r.Result.OutputDir, r.Result.TableCount)
		}
	}
	if err := <-watchErr; err != nil {
		fmt.Fprintf(os.Stderr, "watch failed: %v\n", err)
		os.Exit(1)
	}
	if failed {
		// 감시 중 한 번이라도 실패한 파일이 있었으면 그 사실이 종료 코드에
		// 남아야 한다 — 스크립트/CI에서 watch를 돌린 경우를 위해서다.
		os.Exit(1)
	}
}

// printFolderResults는 파일별 결과를 찍고, 하나라도 실패했는지 돌려준다.
// 종료 코드 판단을 호출부에 남겨두기 위해 출력과 분리했다.
func printFolderResults(out, errOut io.Writer, results []pipeline.FileResult) bool {
	failed := false
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(errOut, "[SKIP] %s: %v\n", r.SourceFile, r.Err)
			failed = true
			continue
		}
		fmt.Fprintf(out, "[OK] %s -> %s (%d tables)\n", r.SourceFile, r.Result.OutputDir, r.Result.TableCount)
	}
	return failed
}

// resolveDictionary는 이번 실행에 쓸 사전을 정한다. --dictionary를 명시했으면
// 그것만 쓰고 자동탐색은 아예 하지 않는다(설정 파일과 같은 규칙).
//
// 셋 다 없으면 에러다. 사전 없이 돌리면 전부 미매칭이라 원본 복사본이
// 나오는데, 그걸 조용히 성공으로 보고하면 안 된다.
//
// 두 번째 반환값이 true면 «자동으로 찾은» 것이다 — 그 경우에만 화면에
// 알린다. 설정이 소리 없이 적용되면 산출물이 왜 달라졌는지 알 수 없다.
func resolveDictionary(explicit string, cfg config.Config, dir string) (string, bool, error) {
	if explicit != "" {
		return explicit, false, nil
	}
	if cfg.Dictionary != "" {
		return cfg.Dictionary, false, nil
	}
	if found, ok := config.DiscoverDictionary(dir); ok {
		return found, true, nil
	}
	return "", false, fmt.Errorf("no dictionary: pass --dictionary <path>, set 'dictionary:' in the config, or put 표준용어사전.xlsx next to the target")
}

// printDryRun은 --dry-run 표를 찍는다. 파일은 쓰지 않는다.
func printDryRun(w io.Writer, stats convert.Stats) {
	for _, d := range stats.Details {
		fmt.Fprintf(w, "  %-6s %s -> %s  [%s]", d.Kind, d.Logical, d.Physical, d.Route)
		var missing []string
		for _, p := range d.Parts {
			if !p.Matched {
				missing = append(missing, p.Source)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(w, "  미매칭: %s", strings.Join(missing, ", "))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "요약: %d건 중 %d건 변환", stats.Total, stats.Converted)
	if len(stats.Unmatched) > 0 {
		fmt.Fprintf(w, ", 미매칭 낱말 %d개(%s)", len(stats.Unmatched), strings.Join(stats.Unmatched, ", "))
	}
	fmt.Fprintln(w)
}

// printConvertResults는 convert 파일별 결과를 찍고, 하나라도 실패했는지
// 돌려준다. printFolderResults와 결은 같지만 그 함수를 그대로 쓰지 않는다 —
// convert에는 outDir도 테이블 개수도 없어서 재사용하면 "(0 tables)" 같은
// 거짓 문구가 나온다. 대신 Stats(N건 중 M건 변환, 미매칭 낱말 개수)를 찍는다.
//
// 단일 파일 경로(runConvert)도 결과 하나짜리 슬라이스로 이 함수를 불러
// 같은 문구를 낸다 — 폴더와 단일 파일이 같은 정보를 다르게 찍으면
// 사용자가 두 형식을 배워야 한다. [OVERWRITTEN]도 같은 이유로 두
// 경로에 함께 적용된다.
//
// target은 이 배치가 무엇을 겨냥했는지(파일 또는 폴더 경로)다. results가
// 빈 경우에만 쓴다 — ConvertFolder는 이미 변환된 산출물(*.physical.drawio)을
// 건너뛰므로, 같은 폴더에 두 번째로 돌리면 results가 통째로 빈 채
// 돌아온다. 그때 아무것도 안 찍고 조용히 끝나면 스크립트에게는 성공과
// 구별되지 않는다 — 실패가 아니므로(failed=false) 종료 코드는 0을 유지한
// 채 한 줄만 알린다.
func printConvertResults(out, errOut io.Writer, target string, results []pipeline.ConvertFileResult) bool {
	if len(results) == 0 {
		fmt.Fprintf(out, "%s: nothing to convert (already converted, or no .drawio files found)\n", target)
		return false
	}
	failed := false
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(errOut, "[SKIP] %s: %v\n", r.SourceFile, r.Err)
			failed = true
			continue
		}
		// [CONVERTED]와 [OVERWRITTEN]을 가른다. 원장 결정 5는 물리 ERD를
		// «사람이 검수·수정한 다음» 리포트를 만드는 흐름을 전제하는데,
		// convert를 다시 돌리면 그 손댄 파일이 대체된다. logicalName이
		// 지켜주는 것은 이름뿐이고 레이아웃·추가한 컬럼·메모는 아니다.
		// 재실행 자체를 막지는 않는다(멱등성이 설계 목표다) — 대신 매
		// 실행마다 어느 쪽이 벌어졌는지 말한다.
		word := "CONVERTED"
		if r.Overwritten {
			word = "OVERWRITTEN"
		}
		fmt.Fprintf(out, "[%s] %s (%d건 중 %d건 변환", word, r.OutputFile, r.Stats.Total, r.Stats.Converted)
		if len(r.Stats.Unmatched) > 0 {
			// "건"은 이름(Total/Converted) 단위다. Unmatched는 «낱말» 목록이지
			// «이름» 개수가 아니므로(internal/convert/plan.go의 Stats 주석)
			// 명사 없는 "%d개"만 쓰면 앞의 "건"을 상속해 "N건 미매칭"으로
			// 읽힌다 — printDryRun의 "미매칭 낱말 %d개"와 같은 단위로 맞춘다.
			fmt.Fprintf(out, ", 미매칭 낱말 %d개", len(r.Stats.Unmatched))
		}
		fmt.Fprintln(out, ")")
	}
	return failed
}

// checkConvertFlags는 서로 어긋나는 플래그 조합을 막는다.
//
// 조용히 무시하지 않는 이유: 사용자가 친 것을 말없이 버리는 것이 이
// 프로젝트가 막으려는 실패(«조용히 틀린 산출물을 성공으로 보고»)의
// 사용자 대면 형태다.
//   - `--recursive`를 파일에 걸면 아무 일도 일어나지 않는데, 사용자는
//     하위 폴더까지 읽었고 변환할 것이 없었다고 믿는다.
//   - `--out`을 `--dry-run`과 함께 치면 파일이 아예 안 나온다. 사용자는
//     지정한 폴더에서 산출물을 찾아 헤맨다.
//
// 문구는 기존 폴더+`--dry-run` 거절과 같은 결로 맞춘다.
func checkConvertFlags(isDir, wantRecursive, wantDryRun bool, outDir string) error {
	if isDir && wantDryRun {
		return fmt.Errorf("--dry-run is only supported for a single file")
	}
	if !isDir && wantRecursive {
		return fmt.Errorf("--recursive is only supported for a folder target")
	}
	if wantDryRun && outDir != "" {
		return fmt.Errorf("--out is not supported with --dry-run (--dry-run writes no file)")
	}
	return nil
}

func runConvert(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: erdtool convert <file-or-dir> [--out dir] [--dictionary path] [--config path] [--recursive] [--dry-run]")
		os.Exit(1)
	}
	target := args[0]
	var configPath, dictPath, outDir string
	var wantRecursive, wantDryRun bool
	if _, err := (cmdFlags{
		strs: map[string]strFlag{
			"--config":     {&configPath, "path"},
			"--dictionary": {&dictPath, "path"},
			"--out":        {&outDir, "directory"},
		},
		bools: map[string]*bool{"--recursive": &wantRecursive, "--dry-run": &wantDryRun},
	}).parse(args[1:]); err != nil {
		die(err)
	}

	st, err := os.Stat(target)
	if err != nil {
		// runGenerate와 같은 관용구다 — "cannot access %s"가 없으면
		// 플래그 값을 대상 경로로 잘못 받았을 때(예: 대상을 빼먹어 target이
		// "--dry-run" 자체가 됨) 이 에러만 보고는 무엇이 잘못됐는지 알 수 없다.
		fmt.Fprintf(os.Stderr, "cannot access %s: %v\n", target, err)
		os.Exit(1)
	}
	if err := checkConvertFlags(st.IsDir(), wantRecursive, wantDryRun, outDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dir := target
	if !st.IsDir() {
		dir = filepath.Dir(target)
	}

	cfg, cfgPath, err := resolveConfig(configPath, dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}
	if configPath == "" && cfgPath != "" {
		fmt.Printf("using config %s\n", cfgPath)
	}
	if wantRecursive {
		cfg.Recursive = true
	}

	resolvedDict, auto, err := resolveDictionary(dictPath, cfg, dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dictionary resolution failed: %v\n", err)
		os.Exit(1)
	}
	if auto {
		fmt.Printf("using dictionary %s\n", resolvedDict)
	}

	if st.IsDir() {
		results, err := pipeline.ConvertFolder(target, cfg, resolvedDict, outDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "convert failed: %v\n", err)
			os.Exit(1)
		}
		if printConvertResults(os.Stdout, os.Stderr, target, results) {
			// 폴더 안의 파일 하나라도 실패했으면 그 사실이 종료 코드에
			// 남아야 한다 — generate와 같은 규칙이다.
			os.Exit(1)
		}
		return
	}

	if wantDryRun {
		// pipeline.ConvertStats가 실제 실행과 «같은» 읽기·변환 코드를 탄다.
		// 예전에는 여기서 os.ReadFile + glossary.Load + convert.File을 직접
		// 다시 조립하고, 에러 문구를 pipeline 쪽과 손으로 맞춰 놓은 주석을
		// 달아 뒀다 — 한쪽만 고치면 dry-run과 일반 경로가 같은 실패를 다른
		// 정보량으로 보고하게 되는 구조였다.
		stats, err := pipeline.ConvertStats(target, resolvedDict)
		if err != nil {
			die(err)
		}
		printDryRun(os.Stdout, stats)
		return
	}

	// pipeline.Convert는 cfg를 받지 않는다 — 파일 하나짜리 변환에는
	// cfg.Recursive 같은 폴더 스캔 옵션이 쓸모가 없다. 사전과 outDir만
	// 넘긴다.
	stats, out, overwritten, err := pipeline.Convert(target, resolvedDict, outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "convert failed: %v\n", err)
		os.Exit(1)
	}
	printConvertResults(os.Stdout, os.Stderr, target, []pipeline.ConvertFileResult{
		{SourceFile: target, OutputFile: out, Stats: stats, Overwritten: overwritten},
	})
}

func runBuild(args []string) {
	var outPath string
	rest, err := (cmdFlags{
		strs:       map[string]strFlag{"--out": {&outPath, "path"}},
		positional: true,
	}).parse(args)
	if err != nil {
		die(err)
	}
	if len(rest) > 1 {
		fmt.Fprintln(os.Stderr, "build takes exactly one input file")
		os.Exit(1)
	}
	var inputPath string
	if len(rest) == 1 {
		inputPath = rest[0]
	}
	if inputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: erdtool build <설계서.xlsx|csv> [--out FILE]")
		os.Exit(1)
	}

	res, err := pipeline.Build(inputPath, outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}
	status := "[BUILT]"
	if res.Overwritten {
		status = "[OVERWRITTEN]"
	}
	fmt.Printf("%s %s (%d개 페이지, %d개 테이블)\n", status, res.OutputPath, res.Pages, res.Tables)
}

// runReverse는 DB에 접속해 .drawio를 새로 쓴다. 기존 세 서브커맨드와 같은
// 모양이다 — 위치 인자 하나 + 플래그. 플래그 파싱 문구는 영어로 두고
// 결과·통계 줄만 한국어로 낸다(기존 runBuild/runConvert와 같은 결).
func runReverse(args []string) {
	var outPath, connPath, target string
	rest, err := (cmdFlags{
		strs: map[string]strFlag{
			"--out":         {&outPath, "path"},
			"--connections": {&connPath, "path"},
		},
		positional: true,
	}).parse(args)
	if err != nil {
		die(err)
	}
	if len(rest) > 1 {
		fmt.Fprintln(os.Stderr, "reverse takes exactly one target")
		os.Exit(1)
	}
	if len(rest) == 1 {
		target = rest[0]
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: erdtool reverse <접속이름|DSN|파일경로> [--out FILE] [--connections FILE]")
		os.Exit(1)
	}

	res, err := pipeline.Reverse(target, outPath, connPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reverse failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(formatReverseResult(res))
}

// formatReverseResult는 결과 줄을 만든다. 출력과 갈라 둬야 문구를 테스트가
// 짚을 수 있다 — printConvertResults와 같은 이유다.
func formatReverseResult(res pipeline.ReverseResult) string {
	status := "[REVERSED]"
	if res.Overwritten {
		status = "[OVERWRITTEN]"
	}
	out := fmt.Sprintf("%s %s -> %s (%d개 페이지, %d개 테이블, 관계 %d건)\n",
		status, res.Target, res.OutputPath, res.Pages, res.Tables, res.Relations)
	if len(res.SkippedCross) > 0 {
		out += fmt.Sprintf("  * 페이지를 가로지르는 관계 %d건은 그리지 못했다: %s\n",
			len(res.SkippedCross), strings.Join(res.SkippedCross, ", "))
	}
	return out
}

// checkAnnotateFlags는 서로 어긋나는 조합을 막는다. checkConvertFlags와
// 같은 결이되 --out이 없고, --dry-run과 --clean의 조합은 허용한다 —
// «지울 것을 찍기만 한다»는 뜻이 분명하기 때문이다.
func checkAnnotateFlags(isDir, wantRecursive, wantDryRun bool) error {
	if isDir && wantDryRun {
		return fmt.Errorf("--dry-run is only supported for a single file")
	}
	if !isDir && wantRecursive {
		return fmt.Errorf("--recursive is only supported for a folder target")
	}
	return nil
}

func runAnnotate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: erdtool annotate <file-or-dir> [--config path] [--recursive] [--dry-run] [--clean]")
		os.Exit(1)
	}
	target := args[0]
	var configPath string
	var wantRecursive, wantDryRun, wantClean bool
	if _, err := (cmdFlags{
		strs: map[string]strFlag{"--config": {&configPath, "path"}},
		bools: map[string]*bool{
			"--recursive": &wantRecursive,
			"--dry-run":   &wantDryRun,
			"--clean":     &wantClean,
		},
	}).parse(args[1:]); err != nil {
		die(err)
	}

	st, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot access %s: %v\n", target, err)
		os.Exit(1)
	}
	if err := checkAnnotateFlags(st.IsDir(), wantRecursive, wantDryRun); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dir := target
	if !st.IsDir() {
		dir = filepath.Dir(target)
	}
	cfg, cfgPath, err := resolveConfig(configPath, dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}
	if configPath == "" && cfgPath != "" {
		fmt.Printf("using config %s\n", cfgPath)
	}
	if wantRecursive {
		cfg.Recursive = true
	}

	var results []pipeline.AnnotateResult
	if st.IsDir() {
		results, err = pipeline.AnnotateFolder(target, cfg, wantClean)
		if err != nil {
			fmt.Fprintf(os.Stderr, "annotate failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		r, err := pipeline.Annotate(target, cfg, wantClean, wantDryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "annotate failed: %v\n", err)
			os.Exit(1)
		}
		results = []pipeline.AnnotateResult{r}
	}

	failed := 0
	for _, r := range results {
		fmt.Println(annotateResultLine(r, wantClean, wantDryRun))
		if r.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

// annotateResultLine은 결과 하나를 화면에 낼 한 줄로 바꾼다. 출력에서
// 갈라 둬야 문구를 테스트가 조합별로 직접 짚을 수 있다 —
// formatReverseResult와 같은 이유다.
//
// 분기 순서가 뜻을 가른다. 첫째, --dry-run은 파일을 실제로 안 건드렸다는
// 사실을 r.Changed·clean과 무관하게 먼저 말해야 한다. clean && r.Changed를
// 앞에 두면 --clean --dry-run 조합에서 "표식을 지웠다"고 말해 버리는데,
// 실제로는 파일이 그대로다(pipeline.Annotate가 dryRun이면 쓰기 자체를
// 건너뛴다) — 도구가 한 일과 다른 말을 하는 것은 이 저장소가 잡으려는
// 바로 그 실패 형태(«조용히 틀린 산출물을 성공으로 보고»)의 반대쪽
// 얼굴이다: 아무 일도 안 했는데 했다고 말하는 것.
//
// 둘째, clean && !r.Changed는 일반 !r.Changed보다 먼저 갈라야 한다.
// internal/pipeline/annotate.go의 Annotate는 clean일 때 annotate.Clean만
// 부르고 ir.Assemble/validate.Required/Optional을 아예 안 돈다 — 그래서
// clean 경로의 r.Findings는 검증을 안 했으니 항상 0이다. 이걸 일반
// !r.Changed 갈래로 흘려보내면 "(진단 0건)"이라고 찍는데, 사용자는
// "이 도면에 고칠 게 없다"고 읽는다. 실제로는 지울 표식이 없었을
// 뿐 검증 자체를 하지 않았다 — 안 본 것을 봤다고 말하는 것도 같은
// 종류의 거짓말이다.
//
// 셋째, clean 갈래는 Marked만이 아니라 Pages도 찍는다. 앵커 없는
// 진단(duplicate_table_name, 파싱 안 된 행의
// shape_violation)만 나온 페이지는 마크된 셀이 하나도 없이 요약 박스만
// 받는다 — 그런 파일에 --clean을 돌리면 Marked=0인데 파일은 실제로
// 바뀐다(박스가 사라진다). Marked만 찍으면 «표식 0개 제거»라고 말하면서
// 사용자 파일의 sha256을 바꾸는 셈이고, 그건 이 저장소가 가장 경계하는
// 실패(도구가 한 일과 다른 말을 한다)다. annotate.Result.Pages가 이미
// 그 수를 들고 있으니 그대로 찍는다.
func annotateResultLine(r pipeline.AnnotateResult, clean, dryRun bool) string {
	line := annotateStatusLine(r, clean, dryRun)
	// 경고는 결과 줄 «밑»에 붙인다. 줄 안에 섞으면 [ANNOTATED] 한 줄을
	// 눈으로 읽는 사용자가 그냥 지나치고, 폴더 일괄 처리에서는 파일마다
	// 한 줄씩 나오므로 더더욱 묻힌다. 앞의 두 칸은 «이 파일에 딸린 말»
	// 이라는 표다.
	for _, w := range r.Warnings {
		line += "\n  [주의] " + w
	}
	return line
}

// annotateStatusLine은 결과 한 줄의 «상태» 부분만 만든다. 경고를 붙이는
// 일과 갈래를 고르는 일을 갈라 둬야 각각을 따로 짚을 수 있다.
func annotateStatusLine(r pipeline.AnnotateResult, clean, dryRun bool) string {
	switch {
	case r.Err != nil:
		return fmt.Sprintf("[FAILED] %s: %v", r.SourceFile, r.Err)
	case clean && !r.Changed:
		return fmt.Sprintf("[UNCHANGED] %s (지울 표식 없음)", r.SourceFile)
	case !r.Changed:
		// 침묵하지 않는다. «아무 일도 없었다»도 결과다. dry-run 여부와
		// 무관하다 — 바꿀 것 자체가 없으면 --dry-run이 미리 보여줄 것도
		// 없다.
		return fmt.Sprintf("[UNCHANGED] %s (진단 %d건)", r.SourceFile, r.Findings)
	case clean && dryRun:
		return fmt.Sprintf("[DRY-RUN] %s: 표식 %d개, 요약 박스 %d개 제거 예정 (파일은 안 바꿈)",
			r.SourceFile, r.Marked, r.Pages)
	case clean:
		return fmt.Sprintf("[CLEANED] %s (표식 %d개, 요약 박스 %d개 제거)",
			r.SourceFile, r.Marked, r.Pages)
	case dryRun:
		return fmt.Sprintf("[DRY-RUN] %s: 셀 %d개 마크, 페이지 %d개 요약 (진단 %d건, 파일은 안 바꿈)",
			r.SourceFile, r.Marked, r.Pages, r.Findings)
	default:
		return fmt.Sprintf("[ANNOTATED] %s: 셀 %d개 마크, 페이지 %d개 요약 (진단 %d건)",
			r.SourceFile, r.Marked, r.Pages, r.Findings)
	}
}
