package runner

import "sync"

type runners []*runner

func NewRunners(sets CommandSets) runners {
	result := runners{}
	for _, set := range sets {
		result = append(result, NewRunner(set))
	}
	return result
}

func (rs runners) Do() {
	for _, r := range rs {
		go r.Do()
	}
}

func (rs runners) Stop(wg *sync.WaitGroup) {
	for _, r := range rs {
		wg.Add(1)
		go r.Stop(wg)
	}
}
