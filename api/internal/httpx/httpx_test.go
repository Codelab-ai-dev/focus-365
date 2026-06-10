package httpx

import "testing"

type sample struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=6"`
	Mood     int    `validate:"required,min=1,max=10"`
}

func TestValidationMessage(t *testing.T) {
	cases := []struct {
		name string
		in   sample
		want string
	}{
		{"email inválido", sample{Email: "bad", Password: "abcdef", Mood: 5}, "El email no tiene un formato válido"},
		{"password corta", sample{Email: "a@b.com", Password: "123", Mood: 5}, "La contraseña debe tener al menos 6 caracteres"},
		{"mood fuera de rango", sample{Email: "a@b.com", Password: "abcdef", Mood: 11}, "El ánimo debe ser como máximo 10"},
		{"mood faltante", sample{Email: "a@b.com", Password: "abcdef", Mood: 0}, "Falta el ánimo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validate.Struct(c.in)
			if err == nil {
				t.Fatal("se esperaba un error de validación")
			}
			if got := ValidationMessage(err); got != c.want {
				t.Errorf("ValidationMessage = %q, want %q", got, c.want)
			}
		})
	}
}
