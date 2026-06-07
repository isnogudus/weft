// Package idalloc computes the next free POSIX uid/gid number within a range.
//
// The function is pure: it takes the set of numbers already in use and the
// configured [Min,Max] range and returns the smallest free number. The caller
// (the directory implementation) is responsible for reading the in-use set from
// the directory and for serialising allocation with a lock, since ldapd has no
// atomic counters and a concurrent scan+add would race.
package idalloc

import "errors"

// ErrRangeExhausted means no free number is available in [Min,Max].
var ErrRangeExhausted = errors.New("idalloc: range exhausted")

// Range is an inclusive [Min,Max] allocation window.
type Range struct {
	Min int
	Max int
}

// Valid reports whether the range is well-formed.
func (r Range) Valid() bool { return r.Min >= 0 && r.Max >= r.Min }

// NextFree returns the smallest number in r that is not present in used.
// Numbers in used that fall outside r are ignored. Returns ErrRangeExhausted
// when every number in the range is taken.
func NextFree(r Range, used []int) (int, error) {
	if !r.Valid() {
		return 0, ErrRangeExhausted
	}
	taken := make(map[int]struct{}, len(used))
	for _, n := range used {
		if n >= r.Min && n <= r.Max {
			taken[n] = struct{}{}
		}
	}
	for n := r.Min; n <= r.Max; n++ {
		if _, ok := taken[n]; !ok {
			return n, nil
		}
	}
	return 0, ErrRangeExhausted
}
