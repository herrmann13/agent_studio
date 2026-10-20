package tui

import "testing"

func TestBoundedIndex(t *testing.T) {
	tests := []struct {
		name   string
		index  int
		length int
		want   int
	}{
		{name: "empty", index: 4, length: 0, want: 0},
		{name: "first", index: 0, length: 3, want: 0},
		{name: "wrap forward", index: 3, length: 3, want: 0},
		{name: "wrap backward", index: -1, length: 3, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := boundedIndex(test.index, test.length); got != test.want {
				t.Errorf("boundedIndex(%d, %d) = %d, want %d", test.index, test.length, got, test.want)
			}
		})
	}
}

func TestInvocationModeIndex(t *testing.T) {
	if got := invocationModeIndex("disabled"); got != 3 {
		t.Errorf("invocationModeIndex(disabled) = %d, want 3", got)
	}
	if got := invocationModeIndex("unknown"); got != 0 {
		t.Errorf("invocationModeIndex(unknown) = %d, want 0", got)
	}
}
