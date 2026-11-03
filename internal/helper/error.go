package helper

type EmptyCmdError error

type emptyCmdError struct{}

var _ EmptyCmdError = &emptyCmdError{}

func NewEmptyCmdError() EmptyCmdError {
	return &emptyCmdError{}
}

func (e *emptyCmdError) Error() string {
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
