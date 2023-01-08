package yaml

const defaultWatchSubDirValue bool = true

type WatchSubDirConverter struct {
	watchSubDir *bool
}

func NewWatchSubDirConverter(watchSubDir *bool) *WatchSubDirConverter {
	return &WatchSubDirConverter{
		watchSubDir: watchSubDir,
	}
}

func (c *WatchSubDirConverter) Convert() bool {
	if c.watchSubDir == nil {
		return defaultWatchSubDirValue
	} else {
		return *c.watchSubDir
	}
}
