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

type txTipo struct {
	Type string `validate:"required,oneof=income expense transfer"`
}

type txMonto struct {
	Amount int64 `validate:"required,min=1"`
}

func TestValidationMessageFinanzas(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"tipo inválido", txTipo{Type: "bogus"}, "El tipo no es válido"},
		{"monto faltante", txMonto{Amount: 0}, "Falta el monto"},
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

// Estructuras con un único campo inválido para aislar cada rama de
// ValidationMessage sin depender del orden de los errores.
type numericMin struct {
	Energy int `validate:"min=2"`
}

type stringMax struct {
	Name string `validate:"max=3"`
}

type unknownTag struct {
	Code string `validate:"alpha"`
}

func TestValidationMessageBranches(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"min numérico", numericMin{Energy: 1}, "La energía debe ser al menos 2"},
		{"max de string", stringMax{Name: "abcd"}, "El nombre debe tener como máximo 3 caracteres"},
		{"tag no manejado", unknownTag{Code: "123"}, "Code no es válido"},
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
