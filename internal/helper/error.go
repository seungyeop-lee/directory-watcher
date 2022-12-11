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
