package shell

import "strings"

// segment is one simple command: its words and the files its output is
// redirected to.
type segment struct {
	words, redirects []string
}

// What the next word read by split is.
const (
	nextWord   = iota
	nextTarget // output redirect target
	nextSkip   // input file, here-string, or fd duplication like 2>&1
	nextDelim  // here-document delimiter
)

// split tokenizes cmd into simple commands, separated by ; & && || | ( )
// and newlines. It understands quotes, comments, redirects and here-docs well
// enough for Writes; it is not a full shell parser. Backslash escapes only
// quotes and spaces, so Windows paths like C:\Users survive.
func split(cmd string) []segment {
	var (
		segs     []segment
		cur      segment
		w        strings.Builder
		inWord   bool
		next     = nextWord
		heredocs []string
	)
	flush := func() {
		if !inWord {
			return
		}
		s := w.String()
		w.Reset()
		inWord = false
		switch next {
		case nextTarget:
			if !discard(s) {
				cur.redirects = append(cur.redirects, s)
			}
		case nextDelim:
			heredocs = append(heredocs, s)
		case nextWord:
			cur.words = append(cur.words, s)
		}
		next = nextWord
	}
	end := func() {
		flush()
		if len(cur.words)+len(cur.redirects) > 0 {
			segs = append(segs, cur)
		}
		cur, next = segment{}, nextWord
	}

	rs := []rune(cmd)
	at := func(i int, c rune) bool { return i < len(rs) && rs[i] == c }
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			w.WriteString(string(rs[i+1 : min(j, len(rs))]))
			inWord, i = true, j
		case c == '"':
			for i++; i < len(rs) && rs[i] != '"'; i++ {
				if rs[i] == '\\' && at(i+1, '"') {
					i++
				}
				w.WriteRune(rs[i])
			}
			inWord = true
		case c == '\\' && at(i+1, '\n'):
			i++ // line continuation
		case c == '\\' && (at(i+1, ' ') || at(i+1, '"') || at(i+1, '\'')):
			w.WriteRune(rs[i+1])
			inWord, i = true, i+1
		case c == '#' && !inWord:
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
			i-- // let the newline end the segment
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		case c == '\n':
			end()
			if len(heredocs) > 0 {
				i = skipHeredocs(rs, i+1, heredocs) - 1
				heredocs = nil
			}
		case c == ';' || c == '(' || c == ')':
			end()
		case c == '|':
			end()
			if at(i+1, '|') {
				i++
			}
		case c == '&' && at(i+1, '&'):
			end()
			i++
		case c == '&' && at(i+1, '>'): // &> file
			flush()
			i++
			if at(i+1, '>') {
				i++
			}
			next = nextTarget
		case c == '&' && !inWord:
			end() // background
		case c == '>':
			if inWord && isDigits(w.String()) { // 2> file: drop the fd number
				w.Reset()
				inWord = false
			} else {
				flush()
			}
			if at(i+1, '>') || at(i+1, '|') {
				i++
			}
			next = nextTarget
			if at(i+1, '&') {
				i++
				next = nextSkip
			}
		case c == '<':
			flush()
			switch {
			case at(i+1, '<') && at(i+2, '<'):
				i += 2
				next = nextSkip
			case at(i+1, '<'):
				i++
				if at(i+1, '-') {
					i++
				}
				next = nextDelim
			default:
				next = nextSkip
			}
		default:
			w.WriteRune(c)
			inWord = true
		}
	}
	end()
	return segs
}

// skipHeredocs returns the index just past the here-document bodies that
// start at i, one per delimiter.
func skipHeredocs(rs []rune, i int, delims []string) int {
	for _, d := range delims {
		for i < len(rs) {
			j := i
			for j < len(rs) && rs[j] != '\n' {
				j++
			}
			line := strings.TrimRight(strings.TrimLeft(string(rs[i:j]), "\t"), "\r")
			i = j + 1
			if line == d {
				break
			}
		}
	}
	return min(i, len(rs))
}

func discard(target string) bool {
	switch strings.ToLower(target) {
	case "/dev/null", "/dev/stdout", "/dev/stderr", "nul", "$null":
		return true
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
