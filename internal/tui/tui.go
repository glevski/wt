// Package tui implements the one piece of terminal UI wt needs: an arrow-key
// list picker. It renders on /dev/tty and reads keys from it directly, so
// stdout stays clean for the wt() shell wrapper to capture.
package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	// ErrCanceled is returned when the user backs out (Esc, q or Ctrl-C).
	ErrCanceled = errors.New("canceled")
	// ErrNoTTY is returned when there is no terminal to interact with.
	ErrNoTTY = errors.New("no interactive terminal")
)

// Pick renders title and items on the terminal and returns the index chosen
// with the arrow keys (j/k work too) and Enter.
func Pick(title string, items []string) (int, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, ErrNoTTY
	}
	defer tty.Close()
	restore, err := makeRaw(tty.Fd())
	if err != nil {
		return 0, ErrNoTTY
	}
	defer restore()

	fmt.Fprint(tty, "\x1b[?25l")       // hide cursor
	defer fmt.Fprint(tty, "\x1b[?25h") // and show it again

	selected := 0
	render(tty, title, items, selected, true)
	defer clear(tty, len(items)+1)

	buf := make([]byte, 64)
	for {
		n, err := tty.Read(buf)
		if err != nil {
			return 0, ErrCanceled
		}
		// One read can carry several keys (pasted or piped input), so the
		// whole chunk is parsed, not just its first key.
		for _, k := range parseKeys(buf[:n]) {
			switch k {
			case keyUp:
				selected = (selected + len(items) - 1) % len(items)
			case keyDown:
				selected = (selected + 1) % len(items)
			case keyEnter:
				return selected, nil
			case keyCancel:
				return 0, ErrCanceled
			}
		}
		render(tty, title, items, selected, false)
	}
}

type key int

const (
	keyUp key = iota
	keyDown
	keyEnter
	keyCancel
)

func parseKeys(b []byte) []key {
	var keys []key
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case 3, 'q': // Ctrl-C, q
			keys = append(keys, keyCancel)
		case '\r', '\n':
			keys = append(keys, keyEnter)
		case 'k':
			keys = append(keys, keyUp)
		case 'j':
			keys = append(keys, keyDown)
		case 27:
			// ESC [ A/B are the arrows (ESC O in application-cursor mode);
			// a lone trailing ESC is the Esc key; any other sequence is
			// skipped one byte at a time.
			if i+2 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				switch b[i+2] {
				case 'A':
					keys = append(keys, keyUp)
				case 'B':
					keys = append(keys, keyDown)
				}
				i += 2
			} else if i == len(b)-1 {
				keys = append(keys, keyCancel)
			}
		}
	}
	return keys
}

// render draws the block; on re-renders it first moves the cursor back up
// over the previous frame. Raw mode needs explicit \r\n line endings.
func render(tty *os.File, title string, items []string, selected int, first bool) {
	var b strings.Builder
	if !first {
		fmt.Fprintf(&b, "\x1b[%dA", len(items)+1)
	}
	b.WriteString("\r\x1b[2K\x1b[1m" + title + "\x1b[0m\r\n")
	for i, item := range items {
		b.WriteString("\r\x1b[2K")
		if i == selected {
			b.WriteString("\x1b[36;1m❯ " + item + "\x1b[0m")
		} else {
			b.WriteString("  " + item)
		}
		b.WriteString("\r\n")
	}
	tty.WriteString(b.String())
}

func clear(tty *os.File, lines int) {
	fmt.Fprintf(tty, "\x1b[%dA\x1b[J", lines)
}
