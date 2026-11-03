// releasenotes는 CHANGELOG.md에서 `## [<version>] - <YYYY-MM-DD>` 절의 본문을 뽑아 stdout에 출력한다.
// 본문은 제목 다음 줄부터 다음 `## ` 제목 앞까지이고, 앞뒤 빈 줄은 뺀다.
// 절이 없거나, 본문이 비었거나, 제목이 날짜로 끝나지 않으면 오류를 stderr에 출력하고 종료 코드 1로 끝난다.
//
// 저장소 루트에서 `go run ./scripts/releasenotes <version>`으로 실행한다. version에는 tag의 v를 뺀 값(예: 3.0.0)을 준다.
// tag workflow가 이 출력을 goreleaser의 --release-notes로 넘긴다.
package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

var headingPattern = regexp.MustCompile(`^## \[([^\]]+)\](?: - (.*))?$`)

func main() { os.Exit(run("CHANGELOG.md", os.Args[1:], os.Stdout, os.Stderr)) }

func run(changelogPath string, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, "usage: releasenotes <version>")
		return 2
	}
	changelog, err := os.ReadFile(changelogPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	notes, err := releaseNotes(string(changelog), args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, notes)
	return 0
}

func releaseNotes(changelog, version string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n")
	start := -1
	var date string
	for i, line := range lines {
		if m := headingPattern.FindStringSubmatch(line); m != nil && m[1] == version {
			start, date = i, strings.TrimSpace(m[2])
			break
		}
	}
	if start == -1 {
		return "", fmt.Errorf("CHANGELOG.md has no ## [%s] section", version)
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	notes := strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
	if notes == "" {
		return "", fmt.Errorf("the CHANGELOG.md ## [%s] section is empty", version)
	}

	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return "", fmt.Errorf("the CHANGELOG.md ## [%s] heading must end with \" - YYYY-MM-DD\", but it ends with %q", version, date)
	}
	return notes, nil
}
