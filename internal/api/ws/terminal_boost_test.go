package ws

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", []string{"/bin/true"}},
		{"ls", []string{"ls"}},
		{"ls -la", []string{"ls", "-la"}},
		{"  ls   -la  ", []string{"ls", "-la"}},
		{"echo 'hello world'", []string{"echo", "hello world"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{"git\tclone\turl", []string{"git", "clone", "url"}},
		{"echo hello\nworld", []string{"echo", "hello", "world"}},
		{"echo hello\rworld", []string{"echo", "hello", "world"}},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := splitCommand(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitCommand(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
