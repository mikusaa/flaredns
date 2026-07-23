package security

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was stored in plaintext")
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password was rejected")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("invalid password was accepted")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("expected a minimum length error")
	}
}
