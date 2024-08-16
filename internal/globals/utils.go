package globals

import (
	"strings"
	"unicode"
)

// capitalizeWords converts a string to title case (capitalizes the first letter of each word)
func CapitalizeWords(input string) string {
	words := strings.Split(input, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = string(unicode.ToUpper(rune(word[0]))) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "-")
}
