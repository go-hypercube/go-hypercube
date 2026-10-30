package reflectutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNil(t *testing.T) {
	var pointer *int
	var slice []int
	var mapping map[string]int
	var channel chan int
	var function func()
	var empty any
	number := 0
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", want: true},
		{name: "nil interface", value: empty, want: true},
		{name: "nil pointer", value: pointer, want: true},
		{name: "nil slice", value: slice, want: true},
		{name: "nil map", value: mapping, want: true},
		{name: "nil channel", value: channel, want: true},
		{name: "nil function", value: function, want: true},
		{name: "pointer", value: &number},
		{name: "empty slice", value: []int{}},
		{name: "empty map", value: map[string]int{}},
		{name: "channel", value: make(chan int)},
		{name: "function", value: func() {}},
		{name: "zero integer", value: 0},
		{name: "false", value: false},
		{name: "empty string", value: ""},
		{name: "struct", value: struct{}{}},
		{name: "array", value: [1]int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsNil(tc.value))
		})
	}
}
