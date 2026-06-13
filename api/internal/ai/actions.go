package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
	"github.com/google/uuid"
)

// Kinds de acción persistidos en ai_actions.kind.
const (
	actionCheckin       = "checkin"
	actionMovimiento    = "movimiento"
	actionHabito        = "habito"
	actionMeta          = "meta"
	actionHabitoNuevo   = "habito_nuevo"
	actionMetaNueva     = "meta_nueva"
	actionEntrenamiento = "entrenamiento"
)

// toolNameToKind mapea el nombre de la function de Groq al kind persistido.
var toolNameToKind = map[string]string{
	"registrar_checkin":       actionCheckin,
	"registrar_movimiento":    actionMovimiento,
	"marcar_habito":           actionHabito,
	"actualizar_meta":         actionMeta,
	"crear_habito":            actionHabitoNuevo,
	"crear_meta":              actionMetaNueva,
	"registrar_entrenamiento": actionEntrenamiento,
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
	OccurredOn     string `json:"occurred_on,omitempty"` // YYYY-MM-DD; "" = hoy
}

type habitoPayload struct {
	HabitID string `json:"habit_id"`
}

type metaPayload struct {
	GoalID   string `json:"goal_id"`
	Progress int    `json:"progress"`
}

type habitoNuevoPayload struct {
	Name       string `json:"name"`
	TargetDays *int32 `json:"target_days,omitempty"`
}

type metaNuevaPayload struct {
	Title     string `json:"title"`
	Dimension string `json:"dimension"`
	Deadline  string `json:"deadline,omitempty"` // YYYY-MM-DD, "" = sin fecha
}

type setPayload struct {
	Exercise string   `json:"exercise"`
	Reps     *int32   `json:"reps,omitempty"`
	WeightKg *float64 `json:"weight_kg,omitempty"`
}

type entrenamientoPayload struct {
	Type string       `json:"type"`
	Note string       `json:"note,omitempty"`
	Sets []setPayload `json:"sets"`
}

// goalDimensions replica el oneof del handler HTTP de metas.
var goalDimensions = map[string]bool{
	"checkin": true, "finanzas": true, "entrenamiento": true, "mente": true, "general": true,
}

const maxWorkoutSets = 20

// maxActionsPerTurn es el tope de acciones que el modelo puede proponer en un
// turno. Si llegan más, el turno completo se descarta (all-or-nothing).
const maxActionsPerTurn = 5

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
		if p.OccurredOn != "" {
			if _, err := time.Parse("2006-01-02", p.OccurredOn); err != nil {
				return nil, fmt.Errorf("occurred_on inválido (YYYY-MM-DD)")
			}
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
	case actionHabitoNuevo:
		var p habitoNuevoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			return nil, fmt.Errorf("falta name")
		}
		if p.TargetDays != nil && *p.TargetDays < 1 {
			return nil, fmt.Errorf("target_days debe ser positivo")
		}
		return json.Marshal(p)
	case actionMetaNueva:
		var p metaNuevaPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		p.Title = strings.TrimSpace(p.Title)
		if p.Title == "" {
			return nil, fmt.Errorf("falta title")
		}
		if !goalDimensions[p.Dimension] {
			return nil, fmt.Errorf("dimension inválida: %s", p.Dimension)
		}
		if p.Deadline != "" {
			if _, err := time.Parse("2006-01-02", p.Deadline); err != nil {
				return nil, fmt.Errorf("deadline inválido (YYYY-MM-DD)")
			}
		}
		return json.Marshal(p)
	case actionEntrenamiento:
		var p entrenamientoPayload
		if err := dec(&p); err != nil {
			return nil, err
		}
		p.Type = strings.TrimSpace(p.Type)
		if p.Type == "" {
			return nil, fmt.Errorf("falta type")
		}
		if len(p.Sets) == 0 || len(p.Sets) > maxWorkoutSets {
			return nil, fmt.Errorf("sets debe tener entre 1 y %d series", maxWorkoutSets)
		}
		for i, s := range p.Sets {
			if strings.TrimSpace(s.Exercise) == "" {
				return nil, fmt.Errorf("serie %d sin exercise", i+1)
			}
			if s.Reps != nil && *s.Reps < 1 {
				return nil, fmt.Errorf("serie %d: reps debe ser positivo", i+1)
			}
			if s.WeightKg != nil && *s.WeightKg <= 0 {
				return nil, fmt.Errorf("serie %d: weight_kg debe ser positivo", i+1)
			}
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
	case actionHabitoNuevo:
		var p habitoNuevoPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo crear el hábito %q.", p.Name)
	case actionMetaNueva:
		var p metaNuevaPayload
		_ = json.Unmarshal(payload, &p)
		if p.Deadline != "" {
			return fmt.Sprintf("Propongo crear la meta %q (%s) para %s.", p.Title, p.Dimension, p.Deadline)
		}
		return fmt.Sprintf("Propongo crear la meta %q (%s).", p.Title, p.Dimension)
	case actionEntrenamiento:
		var p entrenamientoPayload
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Propongo registrar un entrenamiento de %s con %d series.", p.Type, len(p.Sets))
	}
	return "Propongo una acción."
}

// Errores del ciclo de vida de una acción. El handler los traduce a HTTP.
var (
	ErrActionNotFound = errors.New("acción no encontrada")
	ErrActionConflict = errors.New("la acción ya fue resuelta")
	ErrActionInvalid  = errors.New("acción inválida")
)

// Interfaces estrechas (capacidades) sobre los servicios de dominio.
type checkinUpserter interface {
	Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error)
}

type checkinUndoer interface {
	Today(ctx context.Context, userID uuid.UUID, date time.Time) (*checkin.CheckIn, error)
	Delete(ctx context.Context, userID uuid.UUID, date time.Time) (bool, error)
}

type txCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error)
}

type txDeleter interface {
	Delete(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

type habitChecker interface {
	SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error)
}

type habitCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in habits.HabitInput, today time.Time) (*habits.Habit, error)
}

type habitDeleter interface {
	Delete(ctx context.Context, userID, habitID uuid.UUID) (bool, error)
}

type goalPatcher interface {
	Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error)
}

type goalCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in goals.GoalInput, today time.Time) (*goals.Goal, error)
}

type goalDeleter interface {
	Delete(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

type goalReader interface {
	List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error)
}

type workoutCreator interface {
	CreateWorkout(ctx context.Context, userID uuid.UUID, in training.WorkoutInput) (*training.Workout, error)
}

type workoutDeleter interface {
	DeleteWorkout(ctx context.Context, userID, id uuid.UUID) (bool, error)
}

// Interfaces compuestas por servicio real: el ejecutor depende de una por
// servicio de dominio (cada una embebe las capacidades que necesita).
type checkinSvc interface {
	checkinUpserter
	checkinUndoer
}

type financeSvc interface {
	txCreator
	txDeleter
}

type habitsSvc interface {
	habitChecker
	habitCreator
	habitDeleter
}

type goalsSvc interface {
	goalPatcher
	goalCreator
	goalDeleter
	goalReader
}

type trainingSvc interface {
	workoutCreator
	workoutDeleter
}

// actionExecutor traduce una acción confirmada a la llamada del servicio de
// dominio correspondiente. Re-valida el payload (defensa en profundidad: ya se
// validó al proponer, pero el dato vivió en la DB entre medio).
type actionExecutor struct {
	checkin  checkinSvc
	finance  financeSvc
	habits   habitsSvc
	goals    goalsSvc
	training trainingSvc
}

// NewActionExecutor arma el ejecutor con los servicios reales (wiring en server.go).
func NewActionExecutor(c checkinSvc, f financeSvc, h habitsSvc, g goalsSvc, t trainingSvc) *actionExecutor {
	return &actionExecutor{checkin: c, finance: f, habits: h, goals: g, training: t}
}

// Tipos de result persistidos al confirmar; el undo los lee para revertir.
type checkinResult struct {
	Prev *checkinPayload `json:"prev"`
	Date string          `json:"date"`
}

type idResult struct {
	ID string `json:"id"`
}

type metaResult struct {
	PrevProgress int32  `json:"prev_progress"`
	GoalID       string `json:"goal_id"`
}

type dateResult struct {
	HabitID string `json:"habit_id"`
	Date    string `json:"date"`
}

// execute aplica la acción confirmada y devuelve el result (json) que el undo
// usará para revertir. El result captura el estado previo y/o el id creado.
func (e *actionExecutor) execute(ctx context.Context, userID uuid.UUID, kind string, payload []byte, today time.Time) (json.RawMessage, error) {
	normalized, err := parseActionPayload(kind, string(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrActionInvalid, err)
	}
	dateStr := today.Format("2006-01-02")
	switch kind {
	case actionCheckin:
		var p checkinPayload
		_ = json.Unmarshal(normalized, &p)
		// Lee el check-in del día ANTES del upsert para poder restaurarlo.
		var prev *checkinPayload
		if cur, err := e.checkin.Today(ctx, userID, today); err != nil {
			return nil, err
		} else if cur != nil {
			prev = &checkinPayload{Mood: cur.Mood, Energy: cur.Energy, Discipline: cur.Discipline, Note: cur.Note}
		}
		if _, err := e.checkin.Upsert(ctx, userID, checkin.Input{
			Date: today, Mood: p.Mood, Energy: p.Energy, Discipline: p.Discipline, Note: p.Note,
		}); err != nil {
			return nil, err
		}
		return json.Marshal(checkinResult{Prev: prev, Date: dateStr})
	case actionMovimiento:
		var p movimientoPayload
		_ = json.Unmarshal(normalized, &p)
		occurred := today
		if p.OccurredOn != "" {
			occurred, _ = time.Parse("2006-01-02", p.OccurredOn) // ya validado
		}
		tx, err := e.finance.Create(ctx, userID, finance.Input{
			Type: p.Type, Amount: p.AmountCentavos, OccurredOn: occurred, Category: p.Category, Remark: p.Remark,
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(idResult{ID: tx.ID})
	case actionHabito:
		var p habitoPayload
		_ = json.Unmarshal(normalized, &p)
		h, err := e.habits.SetCheck(ctx, userID, uuid.MustParse(p.HabitID), today, true, today)
		if err != nil {
			return nil, err
		}
		if h == nil {
			return nil, fmt.Errorf("%w: hábito no encontrado", ErrActionInvalid)
		}
		return json.Marshal(dateResult{HabitID: p.HabitID, Date: dateStr})
	case actionMeta:
		var p metaPayload
		_ = json.Unmarshal(normalized, &p)
		// Lee el progreso actual de la meta ANTES del patch (best-effort: 0 si no se
		// encuentra). Asume que la meta está "activa" — es la única lista que el
		// modelo ve y patchea; una meta fuera de ese estado caería al fallback 0.
		var prevProgress int32
		if list, err := e.goals.List(ctx, userID, "activa", today); err == nil {
			for _, g := range list {
				if g.ID == p.GoalID {
					prevProgress = g.Progress
					break
				}
			}
		}
		prog := int32(p.Progress)
		g, err := e.goals.Patch(ctx, userID, uuid.MustParse(p.GoalID), goals.GoalPatch{Progress: &prog}, today)
		if err != nil {
			return nil, err
		}
		if g == nil {
			return nil, fmt.Errorf("%w: meta no encontrada", ErrActionInvalid)
		}
		return json.Marshal(metaResult{PrevProgress: prevProgress, GoalID: p.GoalID})
	case actionHabitoNuevo:
		var p habitoNuevoPayload
		_ = json.Unmarshal(normalized, &p)
		h, err := e.habits.Create(ctx, userID, habits.HabitInput{Name: p.Name, TargetDays: p.TargetDays}, today)
		if err != nil {
			return nil, err
		}
		return json.Marshal(idResult{ID: h.ID})
	case actionMetaNueva:
		var p metaNuevaPayload
		_ = json.Unmarshal(normalized, &p)
		var deadline *time.Time
		if p.Deadline != "" {
			d, _ := time.Parse("2006-01-02", p.Deadline) // ya validado en parse
			deadline = &d
		}
		g, err := e.goals.Create(ctx, userID, goals.GoalInput{Title: p.Title, Dimension: p.Dimension, Deadline: deadline}, today)
		if err != nil {
			return nil, err
		}
		return json.Marshal(idResult{ID: g.ID})
	case actionEntrenamiento:
		var p entrenamientoPayload
		_ = json.Unmarshal(normalized, &p)
		sets := make([]training.SetInput, 0, len(p.Sets))
		for _, s := range p.Sets {
			set := training.SetInput{Exercise: s.Exercise, Reps: s.Reps}
			if s.WeightKg != nil {
				g := int32(math.Round(*s.WeightKg * 1000))
				set.WeightGrams = &g
			}
			sets = append(sets, set)
		}
		w, err := e.training.CreateWorkout(ctx, userID, training.WorkoutInput{
			Date: today, Type: p.Type, Note: p.Note, Sets: sets,
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(idResult{ID: w.ID})
	}
	return nil, fmt.Errorf("%w: kind %s", ErrActionInvalid, kind)
}

// undo revierte una acción done usando el result guardado. «Ya no existe»
// (Delete false / Patch nil) NO es error: se transiciona igual. Un error real
// de DB aborta sin transicionar. Result corrupto → ErrActionInvalid.
func (e *actionExecutor) undo(ctx context.Context, userID uuid.UUID, kind string, payload, result []byte) error {
	switch kind {
	case actionCheckin:
		var r checkinResult
		if err := json.Unmarshal(result, &r); err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		date, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			return fmt.Errorf("%w: fecha corrupta", ErrActionInvalid)
		}
		if r.Prev == nil {
			_, err := e.checkin.Delete(ctx, userID, date)
			return err
		}
		_, err = e.checkin.Upsert(ctx, userID, checkin.Input{
			Date: date, Mood: r.Prev.Mood, Energy: r.Prev.Energy,
			Discipline: r.Prev.Discipline, Note: r.Prev.Note,
		})
		return err
	case actionMovimiento:
		var r idResult
		_ = json.Unmarshal(result, &r)
		id, err := uuid.Parse(r.ID)
		if err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		_, err = e.finance.Delete(ctx, userID, id) // false = ya no existe: ok
		return err
	case actionHabito:
		var r dateResult
		_ = json.Unmarshal(result, &r)
		date, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			return fmt.Errorf("%w: fecha corrupta", ErrActionInvalid)
		}
		habitID, err := uuid.Parse(r.HabitID)
		if err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		_, err = e.habits.SetCheck(ctx, userID, habitID, date, false, date)
		return err
	case actionMeta:
		var r metaResult
		_ = json.Unmarshal(result, &r)
		goalID, err := uuid.Parse(r.GoalID)
		if err != nil {
			return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
		}
		prog := r.PrevProgress
		_, err = e.goals.Patch(ctx, userID, goalID, goals.GoalPatch{Progress: &prog}, time.Now().UTC())
		return err // Patch (nil,nil) si ya no existe → err nil: ok
	case actionHabitoNuevo:
		return undoDeleteByID(result, func(id uuid.UUID) (bool, error) { return e.habits.Delete(ctx, userID, id) })
	case actionMetaNueva:
		return undoDeleteByID(result, func(id uuid.UUID) (bool, error) { return e.goals.Delete(ctx, userID, id) })
	case actionEntrenamiento:
		return undoDeleteByID(result, func(id uuid.UUID) (bool, error) { return e.training.DeleteWorkout(ctx, userID, id) })
	}
	return fmt.Errorf("%w: kind %s", ErrActionInvalid, kind)
}

func undoDeleteByID(result []byte, del func(uuid.UUID) (bool, error)) error {
	var r idResult
	_ = json.Unmarshal(result, &r)
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
	}
	_, err = del(id) // false = ya no existe (best-effort): ok
	return err
}

// buildChatTools define las functions que se ofrecen al modelo.
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
		{
			Name:        "crear_habito",
			Description: "Crea un hábito nuevo. Úsala solo si el usuario pide explícitamente crear/empezar un hábito. target_days es el objetivo de racha en días (opcional).",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"name":{"type":"string","description":"nombre del hábito"},
				"target_days":{"type":"integer","minimum":1,"description":"objetivo de días (opcional)"}},
				"required":["name"]}`),
		},
		{
			Name:        "crear_meta",
			Description: "Crea una meta nueva. dimension: checkin, finanzas, entrenamiento, mente o general — infiérela del tema (ahorro→finanzas) y usa general si no es claro. deadline opcional en YYYY-MM-DD.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"title":{"type":"string"},
				"dimension":{"type":"string","enum":["checkin","finanzas","entrenamiento","mente","general"]},
				"deadline":{"type":"string","description":"YYYY-MM-DD, opcional"}},
				"required":["title","dimension"]}`),
		},
		{
			Name:        "registrar_entrenamiento",
			Description: "Registra el entrenamiento de HOY con sus series. Úsala solo si el usuario pide registrar un entreno. El peso va en KILOGRAMOS (weight_kg). No inventes reps ni pesos que el usuario no dijo: las series pueden ir solo con el ejercicio.",
			Parameters: json.RawMessage(`{"type":"object","properties":{
				"type":{"type":"string","description":"tipo de sesión: fuerza, cardio, movilidad..."},
				"note":{"type":"string","description":"nota opcional"},
				"sets":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{
					"exercise":{"type":"string"},
					"reps":{"type":"integer","minimum":1},
					"weight_kg":{"type":"number","exclusiveMinimum":0}},
					"required":["exercise"]}}},
				"required":["type","sets"]}`),
		},
	}
}
