package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type IDGenerator interface {
	New(string) string
}

type RandomIDs struct{}

func (RandomIDs) New(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("secure random unavailable: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}

type TokenGenerator interface {
	NewToken() (string, error)
}

type RandomTokens struct{}

func (RandomTokens) NewToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
