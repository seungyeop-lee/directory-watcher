package converter

const defaultNoWait bool = false

type NoWaitConverter struct {
	noWait *bool
}

func NewNoWaitConverter(noWait *bool) *NoWaitConverter {
	return &NoWaitConverter{
		noWait: noWait,
	}
}

func (c *NoWaitConverter) Convert() bool {
	if c.noWait == nil {
		return defaultNoWait
	} else {
		return *c.noWait
	}
}
