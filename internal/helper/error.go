package helper

// EmptyCmdError는 빈 명령을 빌드할 때 나는 오류다. FilterError는 이 오류만 걸러 명령 목록에서 그 명령을 건너뛴다.
// error처럼 Error 메서드만 요구하는 인터페이스로 정의하면 모든 오류가 이 타입으로 판정되므로 구체 타입으로 둔다.
type EmptyCmdError struct{}

func NewEmptyCmdError() error {
	return EmptyCmdError{}
}

func (EmptyCmdError) Error() string {
	return ""
}

func FilterError[T any](cmds []T, err error) ([]T, error) {
	if err == nil {
		return cmds, nil
	}

	switch err.(type) {
	case EmptyCmdError:
		return cmds, nil
	default:
		return nil, err
	}
}
