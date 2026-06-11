// Package finance implementa el dominio de finanzas: transacciones scoped por
// usuario y la lógica del ciclo de pago (mes financiero que arranca el día de
// pago, el último día hábil del mes).
package finance

import "time"

// dateLayout es el formato de fecha que viaja por la API (YYYY-MM-DD).
const dateLayout = "2006-01-02"

// monthLayout es el formato de un ciclo (YYYY-MM).
const monthLayout = "2006-01"

// payday devuelve el último día hábil del mes dado. Si el último día del mes
// cae en sábado retrocede un día (al viernes); si cae en domingo, dos.
// Solo ajusta fines de semana; no contempla feriados.
func payday(year int, month time.Month) time.Time {
	// Primer día del mes siguiente menos un día = último día del mes.
	last := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1)
	switch last.Weekday() {
	case time.Saturday:
		last = last.AddDate(0, 0, -1)
	case time.Sunday:
		last = last.AddDate(0, 0, -2)
	}
	return last
}

// financialMonth devuelve (año, mes) del mes financiero al que pertenece date:
// si date es en o después del día de pago de su mes, pertenece al mes siguiente.
func financialMonth(date time.Time) (int, time.Month) {
	y, m := date.Year(), date.Month()
	if !date.Before(payday(y, m)) { // date >= payday
		next := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		return next.Year(), next.Month()
	}
	return y, m
}

// Cycle devuelve el primer día del mes financiero de date (la columna cycle).
func Cycle(date time.Time) time.Time {
	y, m := financialMonth(date)
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// ParseCycle interpreta un ciclo "YYYY-MM" como el primer día de ese mes.
func ParseCycle(s string) (time.Time, error) {
	return time.Parse(monthLayout, s)
}

// FormatCycle serializa un ciclo como "YYYY-MM".
func FormatCycle(t time.Time) string {
	return t.Format(monthLayout)
}
