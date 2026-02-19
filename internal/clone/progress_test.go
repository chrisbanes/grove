package clone

import "testing"

func TestParseCPVerboseLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "valid cp verbose line",
			line: "/tmp/src/a.txt -> /tmp/dst/a.txt",
			want: true,
		},
		{
			name: "missing arrow",
			line: "/tmp/src/a.txt /tmp/dst/a.txt",
			want: false,
		},
		{
			name: "empty line",
			line: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCPVerboseLine(tt.line)
			if got != tt.want {
				t.Fatalf("parseCPVerboseLine(%q): want %v, got %v", tt.line, tt.want, got)
			}
		})
	}
}

func TestMapClonePercent(t *testing.T) {
	tests := []struct {
		name   string
		copied int
		total  int
		min    int
		max    int
		want   int
	}{
		{
			name:   "halfway",
			copied: 50,
			total:  100,
			min:    5,
			max:    95,
			want:   50,
		},
		{
			name:   "clamp above max",
			copied: 200,
			total:  100,
			min:    5,
			max:    95,
			want:   95,
		},
		{
			name:   "no total returns min",
			copied: 10,
			total:  0,
			min:    5,
			max:    95,
			want:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapClonePercent(tt.copied, tt.total, tt.min, tt.max)
			if got != tt.want {
				t.Fatalf(
					"mapClonePercent(%d, %d, %d, %d): want %d, got %d",
					tt.copied, tt.total, tt.min, tt.max, tt.want, got,
				)
			}
		})
	}
}
