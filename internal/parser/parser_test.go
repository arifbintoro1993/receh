package parser

import (
	"errors"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 28, 9, 30, 0, 0, jkt)
	noon := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, jkt) }

	tests := []struct {
		name    string
		in      string
		want    Entry
		wantErr error
	}{
		// FR-01 basic shape
		{name: "note then amount", in: "kopi 25k", want: Entry{Amount: 25_000, Note: "kopi"}},
		{name: "thousands dot", in: "gojek 32.500", want: Entry{Amount: 32_500, Note: "gojek"}},
		{name: "with tag", in: "listrik 1,2jt #bills", want: Entry{Amount: 1_200_000, Note: "listrik", Tag: "bills"}},
		{name: "multi-word note keeps case", in: "Makan Siang 45rb", want: Entry{Amount: 45_000, Note: "Makan Siang"}},
		{name: "amount only", in: "25k", want: Entry{Amount: 25_000}},

		// FR-02 amount formats (Phase 1 acceptance list)
		{name: "k", in: "x 25k", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "rb", in: "x 25rb", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "ribu attached", in: "x 25ribu", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "ribu separate", in: "x 25 ribu", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "plain thousands", in: "x 25.000", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "comma decimal k", in: "x 25,5k", want: Entry{Amount: 25_500, Note: "x"}},
		{name: "comma decimal jt", in: "x 1,2jt", want: Entry{Amount: 1_200_000, Note: "x"}},
		{name: "dot decimal juta separate", in: "x 1.2 juta", want: Entry{Amount: 1_200_000, Note: "x"}},
		{name: "dot decimal k", in: "x 2.5k", want: Entry{Amount: 2_500, Note: "x"}},
		{name: "two-digit fraction", in: "x 1,25jt", want: Entry{Amount: 1_250_000, Note: "x"}},
		{name: "multi-group thousands", in: "x 1.250.000", want: Entry{Amount: 1_250_000, Note: "x"}},
		{name: "thousands with zero decimals", in: "x 12.500,00", want: Entry{Amount: 12_500, Note: "x"}},
		{name: "plain digits", in: "x 15000", want: Entry{Amount: 15_000, Note: "x"}},
		{name: "suffix case-insensitive", in: "x 25K", want: Entry{Amount: 25_000, Note: "x"}},
		{name: "JUTA upper separate", in: "x 2 JUTA", want: Entry{Amount: 2_000_000, Note: "x"}},
		{name: "max amount", in: "x 1000jt", want: Entry{Amount: 1_000_000_000, Note: "x"}},

		// FR-03 position and precedence
		{name: "amount first", in: "25k kopi", want: Entry{Amount: 25_000, Note: "kopi"}},
		{name: "amount first separate suffix", in: "25 ribu kopi susu", want: Entry{Amount: 25_000, Note: "kopi susu"}},
		{name: "last suffixed wins", in: "beli 2 kopi 50k", want: Entry{Amount: 50_000, Note: "beli 2 kopi"}},
		{name: "suffixed beats later plain", in: "kopi 25k 2", want: Entry{Amount: 25_000, Note: "kopi 2"}},
		{name: "last of two suffixed", in: "10k kopi 25k", want: Entry{Amount: 25_000, Note: "10k kopi"}},
		{name: "no suffix last number wins", in: "nasi padang 2 25000", want: Entry{Amount: 25_000, Note: "nasi padang 2"}},

		// FR-04 dates
		{name: "kemarin", in: "kopi 25k kemarin", want: Entry{Amount: 25_000, Note: "kopi", SpentAt: noon(2026, 11, 27)}},
		{name: "yesterday any case", in: "Yesterday kopi 25k", want: Entry{Amount: 25_000, Note: "kopi", SpentAt: noon(2026, 11, 27)}},
		{name: "explicit date", in: "kopi 25k @2026-11-20", want: Entry{Amount: 25_000, Note: "kopi", SpentAt: noon(2026, 11, 20)}},
		{name: "date and tag", in: "@2026-11-20 listrik 1,2jt #bills", want: Entry{Amount: 1_200_000, Note: "listrik", Tag: "bills", SpentAt: noon(2026, 11, 20)}},
		{name: "invalid date", in: "kopi 25k @2026-13-40", wantErr: ErrInvalidDate},
		{name: "at-mention is note", in: "traktir @budi 50k", want: Entry{Amount: 50_000, Note: "traktir @budi"}},

		// FR-06 unparseable
		{name: "empty", in: "", wantErr: ErrNoAmount},
		{name: "no number", in: "halo bot", wantErr: ErrNoAmount},
		{name: "zero", in: "kopi 0k", wantErr: ErrNoAmount},
		{name: "over max", in: "kopi 1001jt", wantErr: ErrNoAmount},
		{name: "fractional rupiah", in: "kopi 25,5", wantErr: ErrNoAmount},
		{name: "unknown suffix", in: "kopi 2pcs", wantErr: ErrNoAmount},
		{name: "bad grouping", in: "kopi 25.00", wantErr: ErrNoAmount},
		{name: "huge digits no overflow", in: "kopi 99999999999999999999999jt", wantErr: ErrNoAmount},
		{name: "trailing comma", in: "kopi 25,k", wantErr: ErrNoAmount},
		{name: "first group over 3 digits", in: "kopi 1234.567", wantErr: ErrNoAmount},
		{name: "two commas", in: "kopi 1,2,3k", wantErr: ErrNoAmount},
		{name: "fraction too long", in: "kopi 1,2345678jt", wantErr: ErrNoAmount},
		{name: "tag only", in: "#food", wantErr: ErrNoAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Parse(%q) err = %v, want %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			want := tt.want
			if want.SpentAt.IsZero() {
				want.SpentAt = now
			}
			if got.Amount != want.Amount || got.Note != want.Note || got.Tag != want.Tag || !got.SpentAt.Equal(want.SpentAt) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, want)
			}
		})
	}
}
