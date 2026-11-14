//go:build ignore

// What an application reads from a form and writes to a page: a value without
// the white space around it, a URL's escapes, one field of a form body, and a
// decimal held as thousandths.
//
// This file is not compiled into the generator. Everything after the marker line
// is written into a generated Go program that calls one of rho_str_trim,
// rho_str_url_encode, rho_str_url_decode, rho_form_field or rho_milli_*. It is
// the Go half of csrc/text.c, and the parity tests hold the two to the same
// bytes: a decimal is read with a dot or a comma, digits past the third decimal
// are dropped, and a text that is not a number reads as 0.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// --- emitted below ---

// rhoStrTrim is s without the spaces, tabs and line breaks around it.
func rhoStrTrim(s string) string { return strings.Trim(s, " \t\r\n") }

// rhoStrUrlEncode escapes everything but the unreserved characters, a space
// included, so the result is safe in a path, a query and a form alike.
func rhoStrUrlEncode(s string) string {
	const digits = "0123456789ABCDEF"
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			out.WriteByte(c)
		} else {
			out.WriteByte('%')
			out.WriteByte(digits[c>>4])
			out.WriteByte(digits[c&15])
		}
	}
	return out.String()
}

func rhoTextHex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// rhoStrUrlDecode reads a '+' as a space, as a form sends it, and keeps a '%'
// that is not followed by two hex digits as it is.
func rhoStrUrlDecode(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '+' {
			out.WriteByte(' ')
		} else if c == '%' && i+2 < len(s) && rhoTextHex(s[i+1]) >= 0 && rhoTextHex(s[i+2]) >= 0 {
			out.WriteByte(byte(rhoTextHex(s[i+1])*16 + rhoTextHex(s[i+2])))
			i += 2
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}

// rhoFormField is the first value of a field in an x-www-form-urlencoded body,
// decoded, or "" when the body has none.
func rhoFormField(body string, name string) string {
	for _, pair := range strings.Split(body, "&") {
		key, value, _ := strings.Cut(pair, "=")
		if rhoStrUrlDecode(key) == name {
			return rhoStrUrlDecode(value)
		}
	}
	return ""
}

func rhoTextMilli(text string) (int64, bool) {
	s := strings.Trim(text, " \t\r\n")
	negative := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		negative = s[0] == '-'
		s = s[1:]
	}
	var whole, fraction int64
	wholeDigits, fractionDigits := 0, 0
	separator := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c == '.' || c == ',') && !separator:
			separator = true
		case c < '0' || c > '9':
			return 0, false
		case separator:
			if fractionDigits < 3 {
				fraction = fraction*10 + int64(c-'0')
			}
			fractionDigits++
		default:
			if wholeDigits >= 15 {
				return 0, false
			}
			whole = whole*10 + int64(c-'0')
			wholeDigits++
		}
	}
	if wholeDigits == 0 && fractionDigits == 0 {
		return 0, false
	}
	for d := fractionDigits; d < 3; d++ {
		fraction *= 10
	}
	milli := whole*1000 + fraction
	if negative {
		milli = -milli
	}
	return milli, true
}

// rhoMilliValid is whether a text is a decimal rhoMilliParse reads.
func rhoMilliValid(text string) bool {
	_, ok := rhoTextMilli(text)
	return ok
}

// rhoMilliParse reads a decimal as thousandths, or 0 when it is not one.
func rhoMilliParse(text string) int64 {
	milli, _ := rhoTextMilli(text)
	return milli
}

// rhoMilliFormat writes thousandths as a decimal with no trailing zeros.
func rhoMilliFormat(milli int64) string {
	value := uint64(milli)
	if milli < 0 {
		value = uint64(0) - uint64(milli)
	}
	out := strconv.FormatUint(value/1000, 10)
	if fraction := value % 1000; fraction != 0 {
		digits := strconv.FormatUint(fraction+1000, 10)[1:]
		out += "." + strings.TrimRight(digits, "0")
	}
	if milli < 0 {
		out = "-" + out
	}
	return out
}

// rhoMilliRound keeps places decimals, the half away from zero; three places
// or more leaves the value as it is.
func rhoMilliRound(milli int64, places uint32) int64 {
	if places >= 3 {
		return milli
	}
	step := int64(10)
	if places == 0 {
		step = 1000
	} else if places == 1 {
		step = 100
	}
	magnitude := milli
	if milli < 0 {
		magnitude = -milli
	}
	rounded := (magnitude + step/2) / step * step
	if milli < 0 {
		return -rounded
	}
	return rounded
}

// rhoI64ToStr writes a whole number of any sign.
func rhoI64ToStr(value int64) string { return strconv.FormatInt(value, 10) }

// rhoI64FromStr reads a whole number with an optional sign, or 0.
func rhoI64FromStr(text string) int64 {
	s := strings.Trim(text, " \t\r\n")
	digits := strings.TrimLeft(s, "+-")
	if len(s)-len(digits) > 1 || digits == "" || len(digits) > 18 {
		return 0
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0
		}
	}
	value, _ := strconv.ParseInt(digits, 10, 64)
	if strings.HasPrefix(s, "-") {
		return -value
	}
	return value
}

func rhoTextCase(s string, upper bool) string {
	b := []byte(s)
	for i, c := range b {
		if upper && c >= 'a' && c <= 'z' {
			b[i] = c - 32
		} else if !upper && c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// rhoStrLower and rhoStrUpper change ASCII letters only.
func rhoStrLower(s string) string { return rhoTextCase(s, false) }
func rhoStrUpper(s string) string { return rhoTextCase(s, true) }

// rhoStrField is the field at index (from 0) of s split on sep, or "".
func rhoStrField(s string, sep string, index uint32) string {
	if sep == "" {
		return ""
	}
	parts := strings.Split(s, sep)
	if int(index) >= len(parts) {
		return ""
	}
	return parts[index]
}

func rhoJSONValueAt(doc, key string) (string, bool) {
	needle := "\"" + key + "\""
	from := 0
	for {
		at := strings.Index(doc[from:], needle)
		if at < 0 {
			return "", false
		}
		rest := strings.TrimLeft(doc[from+at+len(needle):], " \t\r\n")
		if strings.HasPrefix(rest, ":") {
			return strings.TrimLeft(rest[1:], " \t\r\n"), true
		}
		from += at + 1
	}
}

// rhoJsonStrField is the first string value of key anywhere in a document, or "".
func rhoJsonStrField(doc string, key string) string {
	rest, ok := rhoJSONValueAt(doc, key)
	if !ok || !strings.HasPrefix(rest, "\"") {
		return ""
	}
	var out strings.Builder
	for i := 1; i < len(rest) && rest[i] != '"'; i++ {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			i++
			switch rest[i] {
			case 'n':
				c = '\n'
			case 't':
				c = '\t'
			case 'r':
				c = '\r'
			default:
				c = rest[i]
			}
		}
		out.WriteByte(c)
	}
	return out.String()
}

// rhoJsonNumField is the first numeric value of key, as written, or "".
func rhoJsonNumField(doc string, key string) string {
	rest, ok := rhoJSONValueAt(doc, key)
	if !ok {
		return ""
	}
	n := 0
	for n < len(rest) && strings.IndexByte("+-.eE0123456789", rest[n]) >= 0 {
		n++
	}
	return rest[:n]
}

// rhoAppWorkspace is where the application keeps its data: what its supervisor
// says, else a folder of its own under the user's home.
func rhoAppWorkspace() string {
	for _, name := range []string{"RHO_APP_WORKSPACE", "ROBOHUB_APP_WORKSPACE"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	app := os.Getenv("RHO_APP_NAME")
	if app == "" {
		app = filepath.Base(os.Args[0])
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".rho", "apps", app)
}
