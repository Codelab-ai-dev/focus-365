package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/google/uuid"
)

// Kinds de acción persistidos en ai_messages.action_kind.
const (
	actionCheckin    = "checkin"
	actionMovimiento = "movimiento"
	actionHabito     = "habito"
	actionMeta       = "meta"
)

// toolNameToKind mapea el nombre de la function de Groq al kind persistido.
var toolNameToKind = map[string]string{
	"registrar_checkin":    actionCheckin,
	"registrar_movimiento": actionMovimiento,
	"marcar_habito":        actionHabito,
	"actualizar_meta":      actionMeta,
}

type checkinPayload struct {
	Mood       int    `json:"mood"`
	Energy     int    `json:"energy"`
	Discipline int    `json:"discipline"`
	Note       string `json:"note,omitempty"`
}

type movimientoPayload struct {
	Type           string `json:"type"`
	AmountCentavos int64  `json:"amount_centavos"`
	Category       string `json:"category"`
	Remark         string `json:"remark,omitempty"`
}

type habitoPayload struct {
	HabitID string `json:"habit_id"`
}

type metaPayload struct {
	GoalID   string `json:"goal_id"`
	Progress int    `json:"progress"`
}

func rango(v, min, max int, campo string) error {
	if v < min || v > max {
		return fmt.Errorf("%s fuera de rango (%d-%d)", campo, min, max)
	}
	return nil
}

// parseActionPayload valida los argumentos del modelo para el kind y devuelve
// el payload normalizado (re-serializado) listo para persistir. Las mismas
// reglas que validan los handlers HTTP de cada módulo.
func parseActionPayload(kind, args string) (json.RawMessage, error) {
	dec := func(v any) error {
		d := json.NewDecoder(strings.NewReader(args))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	switch kind {
	case actionCheckin:
		var p checkinPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		for _, c := range []struct {
			v int
			n string
		}{{p.Mood, "mood"}, {p.Energy, "energy"}, {p.Discipline, "discipline"}} {
			if err := rango(c.v, 1, 10, c.n); err != nil {
				return nil, err
			}
		}
		return json.Marshal(p)
	case actionMovimiento:
		var p movimientoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if p.Type != "income" && p.Type != "expense" {
			return nil, fmt.Errorf("type debe ser income o expense")
		}
		if p.AmountCentavos < 1 {
			return nil, fmt.Errorf("amount_centavos debe ser positivo")
		}
		if strings.TrimSpace(p.Category) == "" {
			return nil, fmt.Errorf("falta category")
		}
		return json.Marshal(p)
	case actionHabito:
		var p habitoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if _, err := uuid.Parse(p.HabitID); err != nil {
			return nil, fmt.Errorf("habit_id inválido")
		}
		return json.Marshal(p)
	case actionMeta:
		var p metaPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		if _, err := uuid.Parse(p.GoalID); err != nil {
			return nil, fmt.Errorf("goal_id inválido")
		}
		if err := rango(p.Progress, 0, 100, "progress"); err != nil {
			return nil, err
		}
		return json.Marshal(p)
	}
	return nil, fmt.Errorf("acción desconocida: %s", kind)
}

// actionSummary genera el texto de la burbuja cuando el modelo no dio texto.
func actionSummary(kind string, payload json.RawMessage) string {
	switch kind {
	case actionCheckin:
		var p checkinPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo registrar tu check-in de hoy: ánimo %d, energía %d, disciplina %d.", p.Mood, p.Energy, p.Discipline)
	case actionMovimiento:
		var p movimientoPayload
		_ = json.Unmarshal(payload, &p)
		verbo := "un gasto"
		if p.Type == "income" {
			verbo = "un ingreso"
		}
		return fmt.Sprintf("Propongo registrar %s de $%.2f en %s.", verbo, float64(p.AmountCentavos)/100, p.Category)
	case actionHabito:
		return "Propongo marcar el hábito como hecho hoy."
	case actionMeta:
		var p metaPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo actualizar el progreso de la meta a %d%%.", p.Progress)
	}
	return "Propongo una acción."
}

// Errores del ciclo de vida de una acción. El handler los traduce a HTTP.
var (
	ErrActionNotFound = errors.New("acción no encontrada")
	ErrActionConflict = errors.New("la acción ya fue resuelta")
	ErrActionInvalid  = errors.New("acción inválida")
)

// Interfaces estrechas sobre los servicios de dominio (testeables con fakes).
type checkinUpserter interface {
	Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error)
}

type txCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error)
}

type habitChecker interface {
	SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error)
}

type goalPatcher interface {
	Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error)
}

// actionExecutor traduce una acción confirmada a la llamada del servicio de
// dominio correspondiente. Re-valida el payload (defensa en profundidad: ya se
// validó al proponer, pero el dato vivió en la DB entre medio).
type actionExecutor struct {
	checkin checkinUpserter
	finance txCreator
	habits  habitChecker
	goals   goalPatcher
}

// NewActionExecutor arma el ejecutor con los servicios reales (wiring en server.go).
func NewActionExecutor(c checkinUpserter, f txCreator, h habitChecker, g goalPatcher) *actionExecutor {
	return &actionExecutor{checkin: c, finance: f, habits: h, goals: g}
}

func (e *actionExecutor) execute(ctx context.Context, userID uuid.UUID, kind string, payload []byte, today time.Time) error {
	normalized, err := parseActionPayload(kind, string(payload))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrActionInvalid, err)
	}
	switch kind {
	case actionCheckin:
		var p checkinPayload
		_ = json.Unmarshal(normalized, &p)
		_, err := e.checkin.Upsert(ctx, userID, checkin.Input{
			Date: today, Mood: p.Mood, Energy: p.Energy, Discipline: p.Discipline, Note: p.Note,
		})
		return err
	case actionMovimiento:
		var p movimientoPayload
		_ = json.Unmarshal(normalized, &p)
		_, err := e.finance.Create(ctx, userID, finance.Input{
			Type: p.Type, Amount: p.AmountCentavos, OccurredOn: today, Category: p.Category, Remark: p.Remark,
		})
		return err
	case actionHabito:
		var p habitoPayload
		_ = json.Unmarshal(normalized, &p)
		h, err := e.habits.SetCheck(ctx, userID, uuid.MustParse(p.HabitID), today, true, today)
		if err != nil {
			return err
		}
		if h == nil {
			return fmt.Errorf("%w: hábito no encontrado", ErrActionInvalid)
		}
		return nil
	case actionMeta:
		var p metaPayload
		_ = json.Unmarshal(normalized, &p)
		prog := int32(p.Progress)
		g, err := e.goals.Patch(ctx, userID, uuid.MustParse(p.GoalID), goals.GoalPatch{Progress: &prog}, today)
		if err != nil {
			return err
		}
		if g == nil {
			return fmt.Errorf("%w: meta no encontrada", ErrActionInvalid)
		}
		return nil
	}
	return fmt.Errorf("%w: kind %s", ErrActionInvalid, kind)
}

// buildChatTools define las 4 functions que se ofrecen al modelo.
func buildChatTools() []Tool {
	return []Tool{
		{
			Name:        "registrar_checkin",
			Description: "Registra o actualiza el check-in de HOY del usuario. Úsala solo si el usuario pide explícitamente registrar su check-in y dio los tres valores (1-10).",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"mood":{"type":"integer","minimum":1,"maximum":10,"description":"ánimo 1-10"},
				"energy":{"type":"integer","minimum":1,"maximum":10,"description":"energía 1-10"},
				"discipline":{"type":"integer","minimum":1,"maximum":10,"description":"disciplina 1-10"},
				"note":{"type":"string","description":"nota opcional"}},
				"required":["mood","energy","discipline"]}`),
		},
		{
			Name:        "registrar_movimiento",
			Description: "Registra un movimiento financiero de HOY. type es income (ingreso) o expense (gasto). El monto va en CENTAVOS (ej: $250.00 → 25000). Úsala solo si el usuario dio monto y categoría.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"type":{"type":"string","enum":["income","expense"]},
				"amount_centavos":{"type":"integer","minimum":1},
				"category":{"type":"string"},
				"remark":{"type":"string"}},
				"required":["type","amount_centavos","category"]}`),
		},
		{
			Name:        "marcar_habito",
			Description: "Marca un hábito como hecho HOY. habit_id debe ser el id exacto de la lista habits del contexto; si el usuario nombra un hábito que no está, no uses la función y dilo.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"habit_id":{"type":"string","description":"UUID del hábito, de la lista habits del contexto"}},
				"required":["habit_id"]}`),
		},
		{
			Name:        "actualizar_meta",
			Description: "Actualiza el progreso (0-100) de una meta activa. goal_id debe ser el id exacto de la lista goals del contexto.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"goal_id":{"type":"string","description":"UUID de la meta, de la lista goals del contexto"},
				"progress":{"type":"integer","minimum":0,"maximum":100}},
				"required":["goal_id","progress"]}`),
		},
	}
}
