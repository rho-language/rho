package analysis

import "testing"

func TestLexPreservesPositionsAndKinds(t *testing.T) {
	tokens := Lex("module demo\nfn main() void")
	if len(tokens) < 5 {
		t.Fatalf("expected tokens, got %#v", tokens)
	}
	if tokens[0].Kind != "keyword" || tokens[0].Text != "module" || tokens[0].Line != 0 || tokens[0].Column != 0 {
		t.Fatalf("unexpected module token: %#v", tokens[0])
	}
	if tokens[1].Kind != "identifier" || tokens[1].Text != "demo" || tokens[1].Line != 0 || tokens[1].Column != 7 {
		t.Fatalf("unexpected name token: %#v", tokens[1])
	}
	if tokens[2].Text != "fn" || tokens[2].Line != 1 || tokens[2].Column != 0 {
		t.Fatalf("unexpected function token: %#v", tokens[2])
	}
}

func TestLexClassifiesLiteralsAndOperators(t *testing.T) {
	tokens := Lex("let value = 42 + 0.5")
	foundLiteral, foundOperator := false, false
	for _, token := range tokens {
		if token.Kind == "literal" {
			foundLiteral = true
		}
		if token.Kind == "operator" {
			foundOperator = true
		}
	}
	if !foundLiteral || !foundOperator {
		t.Fatalf("expected literal and operator tokens: %#v", tokens)
	}
}
