package command

// emptyCmdError는 빈 명령을 빌드할 때 나는 오류다. filterError는 이 오류만 걸러 명령 목록에서 그 명령을 건너뛴다.
// error처럼 Error 메서드만 요구하는 인터페이스로 정의하면 모든 오류가 이 타입으로 판정되므로 구체 타입으로 둔다.
type emptyCmdError struct{}

func newEmptyCmdError() error {
	return emptyCmdError{}
}

func (emptyCmdError) Error() string {
	return ""
}

func filterError[T any](cmds []T, err error) ([]T, error) {
	if err == nil {
		return cmds, nil
	}

	switch err.(type) {
	case emptyCmdError:
		return cmds, nil
	default:
		return nil, err
	}
}
