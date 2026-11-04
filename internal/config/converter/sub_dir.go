package converter

const defaultSubDirValue bool = true

// SubDirConverter는 option.watch.subDir 값을 바꾼다. 값을 적지 않았으면 하위 폴더도 감시한다.
type SubDirConverter struct {
	subDir *bool
}

func NewSubDirConverter(subDir *bool) *SubDirConverter {
	return &SubDirConverter{subDir: subDir}
}

func (c SubDirConverter) Convert() (bool, error) {
	if c.subDir == nil {
		return defaultSubDirValue, nil
	}
	return *c.subDir, nil
}
