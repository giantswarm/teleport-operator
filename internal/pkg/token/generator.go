package token

import "github.com/google/uuid"

type Generator interface {
	Generate() string
}

type RandomGenerator struct {
}

func NewGenerator() Generator {
	return &RandomGenerator{}
}

func (g *RandomGenerator) Generate() string {
	return uuid.NewString()
}
