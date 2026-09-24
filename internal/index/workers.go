package index

import (
	"sync"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Keep source reads serialized: Source does not promise concurrent Read calls.
// Each worker holds at most one parsed unit while its network writes run.
type singleUnitSource struct {
	trace.Source
	unit trace.Unit
	mu   *sync.Mutex
}

func (s singleUnitSource) Units() ([]trace.Unit, error) { return []trace.Unit{s.unit}, nil }
func (s singleUnitSource) Read(u trace.Unit) ([]trace.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Source.Read(u)
}

func runReadSideWorkers(src trace.Source, cl *layer.Client, st *State, opt Options, units []trace.Unit) (*Report, error) {
	type result struct {
		index  int
		report *Report
		state  *State
		err    error
	}
	jobs := make(chan int)
	results := make(chan result)
	var sourceMu, repoMu sync.Mutex
	repoURL := opt.RepoURL
	workerOpt := opt
	workerOpt.Workers = 1
	workerOpt.Progress = nil
	workerOpt.RepoURL = func(dir string) string {
		repoMu.Lock()
		defer repoMu.Unlock()
		return repoURL(dir)
	}
	// Snapshot signatures before workers start. Only the collector changes st.
	signatures := make([]string, len(units))
	for i, u := range units {
		signatures[i] = st.Units[u.Key]
	}
	var wg sync.WaitGroup
	for range min(opt.Workers, len(units)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				u := units[i]
				state := &State{Units: map[string]string{u.Key: signatures[i]}}
				report, err := Run(singleUnitSource{src, u, &sourceMu}, cl, state, workerOpt)
				results <- result{i, report, state, err}
			}
		}()
	}
	go func() {
		for i := range units {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	ordered := make([]result, len(units))
	done := 0
	for r := range results {
		ordered[r.index] = r
		done++
		if opt.Progress != nil {
			opt.Progress(done, len(units), units[r.index].Key)
		}
	}
	rep := &Report{}
	for i, r := range ordered {
		if r.err != nil {
			return rep, r.err
		}
		one := r.report
		rep.UnitsSeen += one.UnitsSeen
		rep.UnitsIndexed += one.UnitsIndexed
		rep.UnitsSkipped += one.UnitsSkipped
		rep.Turns += one.Turns
		rep.Chunks += one.Chunks
		rep.Blocks += one.Blocks
		rep.Sessions += one.Sessions
		rep.RowsUpserted += one.RowsUpserted
		rep.BlockRowsUpserted += one.BlockRowsUpserted
		rep.SessionRowsUpserted += one.SessionRowsUpserted
		rep.EmbeddingTokens += one.EmbeddingTokens
		rep.Errors = append(rep.Errors, one.Errors...)
		if one.UnitsIndexed > 0 && !opt.DryRun && units[i].Signature != "" {
			st.Units[units[i].Key] = r.state.Units[units[i].Key]
		}
	}
	return rep, nil
}
