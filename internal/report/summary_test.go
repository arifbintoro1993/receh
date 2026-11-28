package report

import (
	"reflect"
	"testing"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		want  Summary
	}{
		{name: "empty", want: Summary{}},
		{
			name:  "single",
			items: []Item{{Amount: 25_000, Category: "Food"}},
			want:  Summary{Total: 25_000, Count: 1, Categories: []CategoryTotal{{Name: "Food", Total: 25_000, Count: 1}}},
		},
		{
			name: "groups and sorts by total desc",
			items: []Item{
				{Amount: 25_000, Category: "Food"},
				{Amount: 1_200_000, Category: "Bills"},
				{Amount: 45_000, Category: "Food"},
				{Amount: 32_500, Category: "Transport"},
			},
			want: Summary{Total: 1_302_500, Count: 4, Categories: []CategoryTotal{
				{Name: "Bills", Total: 1_200_000, Count: 1},
				{Name: "Food", Total: 70_000, Count: 2},
				{Name: "Transport", Total: 32_500, Count: 1},
			}},
		},
		{
			name: "ties sorted by name",
			items: []Item{
				{Amount: 10_000, Category: "Transport"},
				{Amount: 10_000, Category: "Food"},
			},
			want: Summary{Total: 20_000, Count: 2, Categories: []CategoryTotal{
				{Name: "Food", Total: 10_000, Count: 1},
				{Name: "Transport", Total: 10_000, Count: 1},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summarize(tt.items); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
