package term

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func (b *TermBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	return len(p), nil
}

// Raw returns every byte written. Screen returns the final visible lines.
func (b *TermBuffer) Raw() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func (b *TermBuffer) Screen() string { return renderScreen(b.Raw()) }

// renderScreen replays text on an empty terminal: \r goes to the start of the row, \n to the start of the next one,
// the erase sequences clear a row, the end of it or the end of the screen, the cursor moves up, colour sequences are
// dropped, and any other rune overwrites the cell under the cursor.
func renderScreen(text string) string {
	s := &screenState{rows: [][]rune{nil}}
	for i := 0; i < len(text); {
		if text[i] == escape {
			if n := s.sequence(text[i:]); n > 0 {
				i += n
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		s.put(r)
		i += size
	}
	return s.String()
}

// screenState is a terminal with unlimited width and height: the rows so far and the cursor.
type screenState struct {
	rows [][]rune
	cur  int // the cursor's row
	col  int
}

func (s *screenState) row() int { return s.cur }

func (s *screenState) put(r rune) {
	switch r {
	case '\r':
		s.col = 0
	case '\n':
		s.cur++
		if s.cur == len(s.rows) {
			s.rows = append(s.rows, nil)
		}
		s.col = 0
	default:
		s.write(r)
	}
}

func (s *screenState) write(r rune) {
	line := s.rows[s.row()]
	for len(line) < s.col {
		line = append(line, ' ')
	}
	if s.col < len(line) {
		line[s.col] = r
	} else {
		line = append(line, r)
	}
	s.rows[s.row()] = line
	s.col++
}

// sequence handles a control sequence that starts at the head of text and returns how many bytes it used, or 0 when
// text does not start with one it knows: that includes a sequence cut off at the end, which stays as text.
func (s *screenState) sequence(text string) int {
	if len(text) < 3 || text[1] != '[' {
		return 0
	}
	end := 2
	for end < len(text) && (text[end] >= '0' && text[end] <= '9' || text[end] == ';') {
		end++
	}
	if end == len(text) {
		return 0
	}
	switch params := text[2:end]; text[end] {
	case 'm':
		return end + 1
	case 'K':
		return s.erase(params, end+1)
	case 'A':
		return s.up(params, end+1)
	case 'J':
		return s.eraseBelow(params, end+1)
	}
	return 0
}

// up moves the cursor up by the parameter (1 when empty), stopping at the top row, and returns used.
func (s *screenState) up(params string, used int) int {
	n := 1
	if params != "" {
		parsed, err := strconv.Atoi(params)
		if err != nil {
			return 0
		}
		n = max(parsed, 1)
	}
	s.cur = max(s.cur-n, 0)
	return used
}

// eraseBelow clears from the cursor to the end of the screen and returns used, or 0 for a parameter it does not know.
func (s *screenState) eraseBelow(params string, used int) int {
	if params != "" && params != "0" {
		return 0
	}
	if s.col < len(s.rows[s.cur]) {
		s.rows[s.cur] = s.rows[s.cur][:s.col]
	}
	s.rows = s.rows[:s.cur+1]
	return used
}

// erase applies an erase-in-line sequence with the given parameter and returns used, or 0 for one it does not know.
func (s *screenState) erase(params string, used int) int {
	row := s.row()
	switch params {
	case "", "0":
		if s.col < len(s.rows[row]) {
			s.rows[row] = s.rows[row][:s.col]
		}
	case "2":
		s.rows[row] = nil
	default:
		return 0
	}
	return used
}

func (s *screenState) String() string {
	lines := make([]string, len(s.rows))
	for i, row := range s.rows {
		lines[i] = string(row)
	}
	return strings.Join(lines, "\n")
}
