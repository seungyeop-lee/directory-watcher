package runner

import "sync"

type runners struct {
	initCmd Cmd
	endCmd  Cmd
	runners []*runner
}

func NewRunners(sets CommandSets) runners {
	result := runners{}
	result.initCmd = sets.InitCmd
	result.endCmd = sets.EndCmd
	for _, set := range sets.Sets {
		result.runners = append(result.runners, NewRunner(set))
	}
	return result
}

func (rs runners) Do() {
	rs.initCmd.Run("")

	for _, r := range rs.runners {
		go r.Do()
	}
}

func (rs runners) Stop() {
	setsWg := sync.WaitGroup{}
	for _, r := range rs.runners {
		setsWg.Add(1)
		go r.Stop(&setsWg)
	}
	setsWg.Wait()

	rs.endCmd.Run("")
}
