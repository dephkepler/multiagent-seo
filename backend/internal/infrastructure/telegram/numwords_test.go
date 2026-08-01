package telegram

import "testing"

func TestUkrainianNumberWords(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "нуль"},
		{1, "одна"},
		{2, "дві"},
		{5, "п'ять"},
		{10, "десять"},
		{15, "п'ятнадцять"},
		{20, "двадцять"},
		{21, "двадцять одна"},
		{100, "сто"},
		{800, "вісімсот"},
		{1000, "одна тисяча"},
		{1500, "одна тисяча п'ятсот"},
		{2000, "дві тисячі"},
		{5000, "п'ять тисяч"},
		{11000, "одинадцять тисяч"},
		{12500, "дванадцять тисяч п'ятсот"},
	}
	for _, c := range cases {
		if got := ukrainianNumberWords(c.n); got != c.want {
			t.Errorf("ukrainianNumberWords(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
