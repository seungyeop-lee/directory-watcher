package helper

var EmptyCmdError = &emptyCmdError{}

type emptyCmdError struct {
}

func (e *emptyCmdError) Error() string {
	return ""
}
