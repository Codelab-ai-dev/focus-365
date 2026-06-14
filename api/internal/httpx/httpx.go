// Package httpx reúne helpers HTTP compartidos por los módulos de dominio:
// escritura de JSON/errores y decodificación + validación de requests con
// mensajes claros en español.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// WriteJSON serializa v como JSON con el status dado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr responde con el formato estándar {"error": "..."}.
func WriteErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeAndValidate decodifica el body JSON en dst y lo valida. Si algo falla
// responde 400 con un mensaje claro y devuelve false.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		WriteErr(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	if err := validate.Struct(dst); err != nil {
		WriteErr(w, http.StatusBadRequest, ValidationMessage(err))
		return false
	}
	return true
}

// ValidationMessage traduce el primer error de validación a un mensaje claro
// en español, indicando qué campo falló y por qué.
func ValidationMessage(err error) string {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) || len(verrs) == 0 {
		return "datos inválidos"
	}
	fe := verrs[0]
	label := fieldLabel(fe.Field())
	switch fe.Tag() {
	case "required":
		return "Falta " + label
	case "email":
		return "El email no tiene un formato válido"
	case "min":
		if isNumeric(fe.Kind()) {
			return capitalize(label) + " debe ser al menos " + fe.Param()
		}
		return capitalize(label) + " debe tener al menos " + fe.Param() + " caracteres"
	case "max":
		if isNumeric(fe.Kind()) {
			return capitalize(label) + " debe ser como máximo " + fe.Param()
		}
		return capitalize(label) + " debe tener como máximo " + fe.Param() + " caracteres"
	default:
		return capitalize(label) + " no es válido"
	}
}

func fieldLabel(field string) string {
	switch field {
	case "Email":
		return "el email"
	case "Password":
		return "la contraseña"
	case "Name":
		return "el nombre"
	case "Date":
		return "la fecha"
	case "Mood":
		return "el ánimo"
	case "Energy":
		return "la energía"
	case "Type":
		return "el tipo"
	case "Amount":
		return "el monto"
	case "OccurredOn":
		return "la fecha"
	case "Category":
		return "la categoría"
	case "Remark":
		return "la nota"
	case "TargetDays":
		return "la meta de días"
	case "Day":
		return "el día"
	case "Done":
		return "el estado"
	case "Exercise":
		return "el ejercicio"
	case "Sets":
		return "las series"
	case "Reps":
		return "las repeticiones"
	case "WeightGrams":
		return "el peso"
	case "Title":
		return "el título"
	case "Dimension":
		return "la dimensión"
	case "Deadline":
		return "la fecha límite"
	case "Progress":
		return "el progreso"
	case "Status":
		return "el estado"
	default:
		return field
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
