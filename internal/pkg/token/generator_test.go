package token

import "testing"

func Test_RandomGenerator(t *testing.T) {
	generator := NewGenerator()

	uniqueTokens := make(map[string]int)

	for i := 0; i < 1000; i++ {
		token := generator.Generate()
		count := uniqueTokens[token]
		uniqueTokens[token] = count + 1
		if count > 0 {
			t.Fatalf("generated tokens are not unique, token %s was generated multiple times", token)
		}
	}
}
