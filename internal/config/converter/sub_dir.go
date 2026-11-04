package converter

const defaultSubDirValue bool = true

type SubDirConverter struct {
	subDir *bool
}

func NewSubDirConverter(subDir *bool) *SubDirConverter {
	return &SubDirConverter{
		subDir: subDir,
	}
}

func (c *SubDirConverter) Convert() bool {
	if c.subDir == nil {
		return defaultSubDirValue
	} else {
		return *c.subDir
	}
}
