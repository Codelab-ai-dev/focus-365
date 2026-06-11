package habits

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// List devuelve los hábitos del usuario (activos o archivados) con sus rachas
// calculadas. today se pasa desde el cliente (su zona local). Evita N+1
// trayendo todos los logs en una sola query y agrupándolos por hábito.
func (s *Service) List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]Habit, error) {
	var rows []store.Habit
	var err error
	if archived {
		rows, err = s.q.ListArchivedHabits(ctx, userID)
	} else {
		rows, err = s.q.ListHabits(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Habit{}, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, h := range rows {
		ids[i] = h.ID
	}
	logs, err := s.q.ListLogsByHabitIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byHabit := make(map[uuid.UUID][]time.Time)
	for _, l := range logs {
		byHabit[l.HabitID] = append(byHabit[l.HabitID], l.Day)
	}
	out := make([]Habit, 0, len(rows))
	for _, h := range rows {
		out = append(out, *buildHabit(h, byHabit[h.ID], today))
	}
	return out, nil
}

// Create crea (o devuelve, idempotente por nombre activo) el hábito.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in HabitInput, today time.Time) (*Habit, error) {
	h, err := s.q.CreateHabit(ctx, store.CreateHabitParams{
		UserID: userID, Name: strings.TrimSpace(in.Name), TargetDays: in.TargetDays,
	})
	if err != nil {
		return nil, err
	}
	return s.habitView(ctx, h, today)
}

// SetCheck marca (done) o desmarca (!done) el día y devuelve el hábito
// recalculado. (nil, nil) si el hábito no es del usuario → 404 en el handler.
func (s *Service) SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*Habit, error) {
	h, err := s.q.GetHabit(ctx, store.GetHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if done {
		if err := s.q.UpsertHabitLog(ctx, store.UpsertHabitLogParams{HabitID: h.ID, Day: day}); err != nil {
			return nil, err
		}
	} else {
		if err := s.q.DeleteHabitLog(ctx, store.DeleteHabitLogParams{HabitID: h.ID, Day: day}); err != nil {
			return nil, err
		}
	}
	return s.habitView(ctx, h, today)
}

// Archive marca el hábito como archivado. (nil, nil) si no es del usuario o ya
// estaba archivado.
func (s *Service) Archive(ctx context.Context, userID, habitID uuid.UUID, today time.Time) (*Habit, error) {
	h, err := s.q.ArchiveHabit(ctx, store.ArchiveHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.habitView(ctx, h, today)
}

// Delete borra el hábito y sus logs (cascade). Devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID, habitID uuid.UUID) (bool, error) {
	n, err := s.q.DeleteHabit(ctx, store.DeleteHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// habitView recarga los logs de un solo hábito y arma la vista con rachas.
func (s *Service) habitView(ctx context.Context, h store.Habit, today time.Time) (*Habit, error) {
	logs, err := s.q.ListLogsByHabitIDs(ctx, []uuid.UUID{h.ID})
	if err != nil {
		return nil, err
	}
	days := make([]time.Time, 0, len(logs))
	for _, l := range logs {
		days = append(days, l.Day)
	}
	return buildHabit(h, days, today), nil
}

func buildHabit(h store.Habit, days []time.Time, today time.Time) *Habit {
	current, best, doneToday, doneYesterday := computeStreaks(days, today)
	return &Habit{
		ID:            h.ID.String(),
		Name:          h.Name,
		TargetDays:    h.TargetDays,
		CurrentStreak: current,
		BestStreak:    best,
		DoneToday:     doneToday,
		DoneYesterday: doneYesterday,
		ArchivedAt:    h.ArchivedAt,
		CreatedAt:     h.CreatedAt,
	}
}

// computeStreaks deriva, a partir de los días con log, la racha actual, el
// récord histórico y si hoy/ayer están marcados. Es pura (no toca DB). today
// se pasa desde afuera para no depender del reloj del server.
func computeStreaks(days []time.Time, today time.Time) (current, best int, doneToday, doneYesterday bool) {
	if len(days) == 0 {
		return 0, 0, false, false
	}
	// Set de días normalizados a YYYY-MM-DD para lookups O(1).
	set := make(map[string]bool, len(days))
	for _, day := range days {
		set[day.Format(dateLayout)] = true
	}
	yesterday := today.AddDate(0, 0, -1)
	doneToday = set[today.Format(dateLayout)]
	doneYesterday = set[yesterday.Format(dateLayout)]

	// Racha actual: ancla en hoy si está hecho; si no, en ayer si está hecho;
	// si ninguno, la racha se cortó (0). Cuenta consecutivos hacia atrás.
	var anchor time.Time
	switch {
	case doneToday:
		anchor = today
	case doneYesterday:
		anchor = yesterday
	}
	if !anchor.IsZero() {
		for c := anchor; set[c.Format(dateLayout)]; c = c.AddDate(0, 0, -1) {
			current++
		}
	}

	// Récord: corrida consecutiva más larga sobre los días únicos ordenados.
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys) // YYYY-MM-DD ordena cronológicamente como string
	run := 0
	var prev time.Time
	for i, k := range keys {
		day, _ := time.Parse(dateLayout, k)
		if i > 0 && day.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
		prev = day
	}
	return current, best, doneToday, doneYesterday
}
