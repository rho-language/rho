package analysis

import "unicode"

func Lex(source string) []Token {
	keywords := map[string]bool{"module": true, "fn": true, "struct": true, "type": true, "let": true, "if": true, "else": true, "while": true, "return": true, "break": true, "import": true, "include": true, "feature": true}
	types := map[string]bool{"u8": true, "u16": true, "u32": true, "u64": true, "i32": true, "f64": true, "bool": true, "string": true, "void": true}
	var tokens []Token
	line, column := 0, 0
	for index := 0; index < len(source); {
		character := rune(source[index])
		if character == '\n' {
			line++
			column = 0
			index++
			continue
		}
		if unicode.IsSpace(character) {
			column++
			index++
			continue
		}
		if character == '/' && index+1 < len(source) && source[index+1] == '/' {
			for index < len(source) && source[index] != '\n' {
				index++
				column++
			}
			continue
		}
		startLine, startColumn := line, column
		if unicode.IsLetter(character) || character == '_' {
			start := index
			for index < len(source) && (unicode.IsLetter(rune(source[index])) || unicode.IsDigit(rune(source[index])) || source[index] == '_') {
				index++
				column++
			}
			text := source[start:index]
			kind := "identifier"
			if keywords[text] {
				kind = "keyword"
			} else if types[text] {
				kind = "type"
			}
			tokens = append(tokens, Token{Kind: kind, Text: text, Line: startLine, Column: startColumn})
			continue
		}
		if unicode.IsDigit(character) {
			start := index
			for index < len(source) && (unicode.IsDigit(rune(source[index])) || source[index] == '.') {
				index++
				column++
			}
			tokens = append(tokens, Token{Kind: "literal", Text: source[start:index], Line: startLine, Column: startColumn})
			continue
		}
		if character == '"' {
			start := index
			index++
			column++
			for index < len(source) && source[index] != '"' {
				if source[index] == '\n' {
					line++
					column = 0
				} else {
					column++
				}
				index++
			}
			if index < len(source) {
				index++
				column++
			}
			tokens = append(tokens, Token{Kind: "literal", Text: source[start:index], Line: startLine, Column: startColumn})
			continue
		}
		kind := "operator"
		text := string(character)
		index++
		column++
		tokens = append(tokens, Token{Kind: kind, Text: text, Line: startLine, Column: startColumn})
	}
	return tokens
}
