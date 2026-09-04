package main

import "testing"

func TestHashIsDeterministic(t *testing.T) {
	first := hash("token")
	second := hash("token")
	if first != second {
		t.Fatal("hash must be deterministic")
	}
	if hash("token") == hash("other") {
		t.Fatal("hash collision in test input")
	}
}

func TestRandomTokenIsUnique(t *testing.T) {
	first := randomToken(24)
	second := randomToken(24)
	if first == second {
		t.Fatal("random tokens must differ")
	}
}
