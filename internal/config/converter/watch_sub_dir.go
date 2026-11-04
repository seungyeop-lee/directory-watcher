package converter

const defaultWatchSubDirValue bool = true

type WatchSubDirConverter struct {
	subDir *bool
}

func NewWatchSubDirConverter(subDir *bool) *WatchSubDirConverter {
	return &WatchSubDirConverter{
		subDir: subDir,
	}
}

func (c *WatchSubDirConverter) Convert() bool {
	if c.subDir == nil {
		return defaultWatchSubDirValue
	} else {
		return *c.subDir
	}
}
