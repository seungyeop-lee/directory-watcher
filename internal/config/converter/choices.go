package converter

import "strings"

// choice는 설정 파일에 적는 값 하나와 그 값이 뜻하는 도메인 값이다.
type choice[T any] struct {
	name  string
	value T
}

// choices는 설정 키 하나가 받는 값의 목록이다. 오류 메시지는 값을 이 순서대로 안내한다.
type choices[T any] []choice[T]

func (c choices[T]) lookup(name string) (T, bool) {
	for _, choice := range c {
		if choice.name == name {
			return choice.value, true
		}
	}
	var zero T
	return zero, false
}

func (c choices[T]) names() []string {
	names := make([]string, len(c))
	for i, choice := range c {
		names[i] = choice.name
	}
	return names
}

// list는 오류 메시지에 쓰도록 값을 "last, lastPerPath, or each"처럼 이어 돌려준다.
func (c choices[T]) list() string {
	names := c.names()
	if len(names) <= 2 {
		return strings.Join(names, " or ")
	}
	return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
}
