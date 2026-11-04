package domain

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ExcludePatterns는 감시 대상 option.exclude에 적은 경로 패턴 목록이다. 각 패턴은 **를 지원하는 glob이고,
// !로 시작하면 앞선 패턴으로 빠진 경로를 다시 감시한다.
type ExcludePatterns []string

// ParseExcludePatterns는 patterns의 문법을 검사해 ExcludePatterns로 돌려준다.
func ParseExcludePatterns(patterns []string) (ExcludePatterns, error) {
	for i, pattern := range patterns {
		if !doublestar.ValidatePattern(strings.TrimPrefix(pattern, "!")) {
			return nil, fmt.Errorf("exclude[%d]: invalid pattern %q", i, pattern)
		}
	}
	return ExcludePatterns(patterns), nil
}

// Excludes는 감시 대상 path 기준 상대 경로 rel이 빠지는지 판단한다. rel은 /로 구분한다.
// 빠진 폴더는 감시 목록에 넣지 않아 그 안의 항목을 !로 다시 살릴 수 없으므로, 상위 폴더가 하나라도
// 빠졌으면 rel도 빠진다.
func (p ExcludePatterns) Excludes(rel string) bool {
	if len(p) == 0 {
		return false
	}
	for i := range len(rel) {
		if rel[i] == '/' && p.lastMatchExcludes(rel[:i]) {
			return true
		}
	}
	return p.lastMatchExcludes(rel)
}

// lastMatchExcludes는 rel에 마지막으로 맞은 패턴이 빼는 패턴인지 돌려준다. 맞은 패턴이 없으면 빼지 않는다.
func (p ExcludePatterns) lastMatchExcludes(rel string) bool {
	excluded := false
	for _, pattern := range p {
		glob, negated := strings.CutPrefix(pattern, "!")
		if doublestar.MatchUnvalidated(glob, rel) {
			excluded = !negated
		}
	}
	return excluded
}
