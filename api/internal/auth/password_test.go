package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	plain := "s3cret!"

	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword devolvió error: %v", err)
	}
	if hash == plain {
		t.Fatal("el hash no debe ser igual al texto plano")
	}
	if !VerifyPassword(hash, plain) {
		t.Fatal("VerifyPassword debe devolver true para la contraseña correcta")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("VerifyPassword debe devolver false para una contraseña incorrecta")
	}
}
