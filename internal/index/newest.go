package index

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/hev/kit/internal/trace"
)

type sourceUnit struct {
	unit     trace.Unit
	src      trace.Source
	state    *State
	modified int64
}

// RunNewest captures units in activity order across native sources. Each unit
// retains its native reader and its archive- and source-scoped signatures.
func RunNewest(sources []trace.Source, cl Archive, st *State, opt Options) (*Report, error) {
	if opt.MigrateArchive || opt.Workers > 1 || opt.Select != nil {
		return nil, errors.New("newest-first capture requires a sequential scan without migration or unit filters")
	}
	unlock, err := lockArchive(cl)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var pending bool
	var all []sourceUnit
	var failures []error
	states := make(map[string]*State)
	for _, src := range sources {
		if opt.Context != nil && opt.Context.Err() != nil {
			break
		}
		root, _ := sourceScope(src)
		state := st
		var needsMigration bool
		if root != "" && !opt.DryRun {
			if err := adoptCleanArchive(cl, root); err != nil {
				failures = append(failures, err)
				continue
			}
			var err error
			needsMigration, err = archiveMigrationPending(cl, root)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			pending = pending || needsMigration
			if _, err := os.Stat(root); os.IsNotExist(err) {
				continue
			}
			state = states[root]
			if state == nil {
				state = scopedState(st, cl, root)
				states[root] = state
			}
		}
		units, err := src.Units()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if needsMigration {
			inheritLegacySignatures(units, state, st)
		}
		for _, u := range units {
			if opt.Context != nil && opt.Context.Err() != nil {
				break
			}
			modified := u.Modified
			if modified == 0 {
				if info, err := os.Stat(u.Key); err == nil {
					modified = info.ModTime().UnixMilli()
				}
			}
			all = append(all, sourceUnit{u, src, state, modified})
		}
	}
	slices.SortStableFunc(all, func(a, b sourceUnit) int {
		switch {
		case a.modified > b.modified:
			return -1
		case a.modified < b.modified:
			return 1
		default:
			return strings.Compare(a.unit.Key, b.unit.Key)
		}
	})
	units := make([]trace.Unit, 0, len(all))
	for _, entry := range all {
		units = append(units, entry.unit)
	}
	rep, err := runUnits(nil, cl, nil, opt, units, func(i int) (trace.Source, *State) {
		return all[i].src, all[i].state
	})
	for root, state := range states {
		mergeScopedState(st, state, cl, root)
	}
	if rep != nil {
		rep.MigrationPending = pending
	}
	if len(failures) > 0 {
		err = errors.Join(err, fmt.Errorf("could not enumerate or prepare %d transcript sources", len(failures)))
	}
	return rep, err
}
