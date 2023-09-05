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

func FilterError(err error) error {
	if err == nil {
		return nil
	}

	switch err.(type) {
	case EmptyCmdError:
		return nil
	default:
		return err
	}
}
