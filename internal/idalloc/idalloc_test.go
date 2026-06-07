package idalloc

import (
	"errors"
	"testing"
)

func TestNextFree(t *testing.T) {
	tests := []struct {
		name string
		r    Range
		used []int
		want int
		err  error
	}{
		{"empty range returns min", Range{10000, 59999}, nil, 10000, nil},
		{"smallest free, gap at start", Range{10000, 59999}, []int{10001, 10002}, 10000, nil},
		{"smallest free, fills gap", Range{10000, 59999}, []int{10000, 10001, 10003}, 10002, nil},
		{"contiguous from min", Range{10000, 59999}, []int{10000, 10001, 10002}, 10003, nil},
		{"ignores out-of-range used", Range{10000, 10002}, []int{5, 9999, 60000}, 10000, nil},
		{"exhausted", Range{10000, 10002}, []int{10000, 10001, 10002}, 0, ErrRangeExhausted},
		{"single slot free", Range{10000, 10000}, nil, 10000, nil},
		{"invalid range", Range{50, 10}, nil, 0, ErrRangeExhausted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextFree(tt.r, tt.used)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if err == nil && got != tt.want {
				t.Fatalf("NextFree = %d, want %d", got, tt.want)
			}
		})
	}
}
