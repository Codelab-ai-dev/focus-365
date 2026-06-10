package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword genera un hash bcrypt de la contraseña en texto plano.
func HashPassword(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword comprueba si la contraseña en texto plano coincide con el hash.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
