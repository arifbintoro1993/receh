// Package report turns expenses into report numbers. It is pure: no I/O and
// no clock, so text replies and PDFs computed from the same rows always agree.
package report

import (
	"cmp"
	"slices"
)

// Item is the part of an expense a summary needs.
type Item struct {
	Amount   int64
	Category string
}

type CategoryTotal struct {
	Name  string
	Total int64
	Count int
}

type Summary struct {
	Total      int64
	Count      int
	Categories []CategoryTotal // largest total first, ties by name
}

// Summarize totals items overall and per category. O(k + c log c) for k
// items in c categories.
func Summarize(items []Item) Summary {
	index := map[string]int{}
	var s Summary
	for _, it := range items {
		s.Total += it.Amount
		s.Count++
		i, ok := index[it.Category]
		if !ok {
			i = len(s.Categories)
			index[it.Category] = i
			s.Categories = append(s.Categories, CategoryTotal{Name: it.Category})
		}
		s.Categories[i].Total += it.Amount
		s.Categories[i].Count++
	}
	slices.SortFunc(s.Categories, func(a, b CategoryTotal) int {
		if c := cmp.Compare(b.Total, a.Total); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return s
}
