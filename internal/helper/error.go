package helper

import "os/exec"

type EmptyCmdError error

type emptyCmdError struct{}

var _ EmptyCmdError = &emptyCmdError{}

func NewEmptyCmdError() EmptyCmdError {
	return &emptyCmdError{}
}

func (e *emptyCmdError) Error() string {
	return ""
}

func FilterError(cmds []*exec.Cmd, err error) ([]*exec.Cmd, error) {
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
