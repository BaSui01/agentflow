package lsp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (s *LSPServer) getDocumentSnapshot(uri string) (managedDocument, bool) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return managedDocument{}, false
	}

	s.docMu.RLock()
	doc, ok := s.documents[uri]
	s.docMu.RUnlock()
	if !ok || doc == nil {
		return managedDocument{}, false
	}

	return *doc, true
}

func applyTextChange(text string, change TextDocumentContentChangeEvent) (string, error) {
	if change.Range == nil {
		return change.Text, nil
	}

	start, err := positionToOffset(text, change.Range.Start)
	if err != nil {
		return "", err
	}
	end, err := positionToOffset(text, change.Range.End)
	if err != nil {
		return "", err
	}

	if start < 0 || end < start || end > len(text) {
		return "", fmt.Errorf("invalid content change range")
	}

	return text[:start] + change.Text + text[end:], nil
}

func positionToOffset(text string, position Position) (int, error) {
	if position.Line < 0 || position.Character < 0 {
		return 0, fmt.Errorf("invalid position %v", position)
	}

	lines := strings.Split(text, "\n")
	if position.Line >= len(lines) {
		return 0, fmt.Errorf("line out of range: %d", position.Line)
	}

	offset := 0
	for i := 0; i < position.Line; i++ {
		offset += len(lines[i]) + 1
	}

	lineRunes := []rune(lines[position.Line])
	if position.Character > len(lineRunes) {
		return 0, fmt.Errorf("character out of range: %d", position.Character)
	}

	offset += len(string(lineRunes[:position.Character]))
	return offset, nil
}

func identifierPrefixAtPosition(text string, position Position) (string, error) {
	line, ok := lineAt(text, position.Line)
	if !ok {
		return "", nil
	}

	runes := []rune(line)
	if position.Character < 0 {
		return "", fmt.Errorf("character out of range: %d", position.Character)
	}
	if position.Character > len(runes) {
		position.Character = len(runes)
	}

	start := position.Character
	for start > 0 && isIdentifierPart(runes[start-1]) {
		start--
	}

	return string(runes[start:position.Character]), nil
}

func wordAtPosition(text string, position Position) (string, Range, bool, error) {
	line, ok := lineAt(text, position.Line)
	if !ok {
		return "", Range{}, false, nil
	}

	runes := []rune(line)
	if position.Character < 0 || position.Character > len(runes) {
		return "", Range{}, false, fmt.Errorf("character out of range: %d", position.Character)
	}

	probe := position.Character
	if probe == len(runes) && probe > 0 {
		probe--
	}
	if probe < len(runes) && !isIdentifierPart(runes[probe]) && probe > 0 && isIdentifierPart(runes[probe-1]) {
		probe--
	}

	if probe < 0 || probe >= len(runes) || !isIdentifierPart(runes[probe]) {
		return "", Range{}, false, nil
	}

	start := probe
	for start > 0 && isIdentifierPart(runes[start-1]) {
		start--
	}

	end := probe + 1
	for end < len(runes) && isIdentifierPart(runes[end]) {
		end++
	}

	word := string(runes[start:end])
	wordRange := Range{
		Start: Position{Line: position.Line, Character: start},
		End:   Position{Line: position.Line, Character: end},
	}

	return word, wordRange, true, nil
}

func findDefinitionRange(text, word string) (Range, bool) {
	if word == "" {
		return Range{}, false
	}

	lines := strings.Split(text, "\n")
	for lineNumber, line := range lines {
		trimmed := strings.TrimSpace(line)

		if matchesDeclaration(trimmed, word) {
			start, end, ok := tokenRangeInLine(line, word)
			if !ok {
				continue
			}
			return Range{
				Start: Position{Line: lineNumber, Character: start},
				End:   Position{Line: lineNumber, Character: end},
			}, true
		}
	}

	ranges := findWordRanges(text, word)
	if len(ranges) == 0 {
		return Range{}, false
	}

	return ranges[0], true
}

func matchesDeclaration(trimmedLine string, word string) bool {
	if trimmedLine == "" {
		return false
	}

	if match := funcDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}
	if match := typeDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}
	if match := varDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}

	return false
}

func parseDocumentSymbols(text string) []DocumentSymbol {
	lines := strings.Split(text, "\n")
	symbols := make([]DocumentSymbol, 0)

	for lineNumber, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		name := ""
		kind := SymbolVariable

		switch {
		case funcDeclPattern.MatchString(trimmed):
			name = funcDeclPattern.FindStringSubmatch(trimmed)[1]
			kind = SymbolFunction
		case typeDeclPattern.MatchString(trimmed):
			name = typeDeclPattern.FindStringSubmatch(trimmed)[1]
			kind = SymbolClass
		case varDeclPattern.MatchString(trimmed):
			name = varDeclPattern.FindStringSubmatch(trimmed)[1]
			if strings.HasPrefix(trimmed, "const ") {
				kind = SymbolConstant
			} else {
				kind = SymbolVariable
			}
		default:
			continue
		}

		start, end, found := tokenRangeInLine(line, name)
		if !found {
			start, end = 0, utf8.RuneCountInString(line)
		}

		fullRange := Range{
			Start: Position{Line: lineNumber, Character: 0},
			End:   Position{Line: lineNumber, Character: utf8.RuneCountInString(line)},
		}

		symbols = append(symbols, DocumentSymbol{
			Name:           name,
			Kind:           kind,
			Range:          fullRange,
			SelectionRange: Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: end}},
		})
	}

	return symbols
}

func findWordRanges(text, word string) []Range {
	if word == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	ranges := make([]Range, 0)

	for lineNumber, line := range lines {
		runes := []rune(line)
		for idx := 0; idx < len(runes); {
			if !isIdentifierStart(runes[idx]) {
				idx++
				continue
			}

			end := idx + 1
			for end < len(runes) && isIdentifierPart(runes[end]) {
				end++
			}

			if string(runes[idx:end]) == word {
				ranges = append(ranges, Range{
					Start: Position{Line: lineNumber, Character: idx},
					End:   Position{Line: lineNumber, Character: end},
				})
			}

			idx = end
		}
	}

	return ranges
}

func tokenRangeInLine(line, word string) (int, int, bool) {
	runes := []rune(line)
	for idx := 0; idx < len(runes); {
		if !isIdentifierStart(runes[idx]) {
			idx++
			continue
		}

		end := idx + 1
		for end < len(runes) && isIdentifierPart(runes[end]) {
			end++
		}

		if string(runes[idx:end]) == word {
			return idx, end, true
		}

		idx = end
	}

	return 0, 0, false
}

func collectIdentifiers(text string) []string {
	unique := make(map[string]struct{})
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		runes := []rune(line)
		for idx := 0; idx < len(runes); {
			if !isIdentifierStart(runes[idx]) {
				idx++
				continue
			}

			end := idx + 1
			for end < len(runes) && isIdentifierPart(runes[end]) {
				end++
			}

			identifier := string(runes[idx:end])
			unique[identifier] = struct{}{}
			idx = end
		}
	}

	identifiers := make([]string, 0, len(unique))
	for identifier := range unique {
		identifiers = append(identifiers, identifier)
	}

	sort.Strings(identifiers)
	return identifiers
}

func lineAt(text string, lineNumber int) (string, bool) {
	if lineNumber < 0 {
		return "", false
	}

	lines := strings.Split(text, "\n")
	if lineNumber >= len(lines) {
		return "", false
	}

	return lines[lineNumber], true
}

func rangesEqual(a, b Range) bool {
	return a.Start.Line == b.Start.Line &&
		a.Start.Character == b.Start.Character &&
		a.End.Line == b.End.Line &&
		a.End.Character == b.End.Character
}

func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || unicode.IsDigit(r)
}

func defaultCompletions() []string {
	return []string{
		"append",
		"break",
		"const",
		"continue",
		"ctx",
		"else",
		"err",
		"for",
		"func",
		"if",
		"len",
		"make",
		"new",
		"nil",
		"Print",
		"Printf",
		"Println",
		"range",
		"return",
		"switch",
		"type",
		"var",
	}
}

func isKeyword(word string) bool {
	switch word {
	case "append", "break", "const", "continue", "else", "for", "func", "if", "len", "make", "new", "nil", "range", "return", "switch", "type", "var":
		return true
	default:
		return false
	}
}

// LSPMessage LSP 消息
type LSPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *LSPError       `json:"error,omitempty"`
}

// LSPError LSP 错误
type LSPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
