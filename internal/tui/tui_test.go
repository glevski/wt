package tui

import (
	"reflect"
	"testing"
)

func TestParseKeys(t *testing.T) {
	cases := []struct {
		in   []byte
		want []key
	}{
		{[]byte{27, '[', 'A'}, []key{keyUp}},
		{[]byte{27, '[', 'B'}, []key{keyDown}},
		{[]byte{27, 'O', 'B'}, []key{keyDown}}, // application cursor mode
		{[]byte{'k'}, []key{keyUp}},
		{[]byte{'j'}, []key{keyDown}},
		{[]byte{'\r'}, []key{keyEnter}},
		{[]byte{'\n'}, []key{keyEnter}},
		{[]byte{27}, []key{keyCancel}},
		{[]byte{'q'}, []key{keyCancel}},
		{[]byte{3}, []key{keyCancel}},
		{[]byte{27, '[', 'C'}, nil}, // right arrow — ignored
		{[]byte{'x'}, nil},
		{nil, nil},
		// several keys in one read: down + enter must both come through
		{[]byte("\x1b[B\r"), []key{keyDown, keyEnter}},
		{[]byte("\x1b[B\x1b[B\x1b[A\r"), []key{keyDown, keyDown, keyUp, keyEnter}},
		{[]byte("jjk\r"), []key{keyDown, keyDown, keyUp, keyEnter}},
	}
	for _, tc := range cases {
		if got := parseKeys(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseKeys(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
