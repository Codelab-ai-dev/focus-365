package ai

import (
	"strings"
	"testing"
)

func TestParseActionPayloadValid(t *testing.T) {
	cases := []struct {
		kind, args string
	}{
		{"checkin", `{"mood":8,"energy":6,"discipline":9,"note":"bien"}`},
		{"checkin", `{"mood":1,"energy":10,"discipline":5}`},
		{"movimiento", `{"type":"expense","amount_centavos":2500000,"category":"comida"}`},
		{"movimiento", `{"type":"income","amount_centavos":1,"category":"sueldo","remark":"junio"}`},
		{"habito", `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
		{"meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`},
	}
	for _, c := range cases {
		if _, err := parseActionPayload(c.kind, c.args); err != nil {
			t.Errorf("%s %s: %v", c.kind, c.args, err)
		}
	}
}

func TestParseActionPayloadInvalid(t *testing.T) {
	cases := []struct {
		name, kind, args string
	}{
		{"kind desconocido", "borrar_todo", `{}`},
		{"json roto", "checkin", `{mood:`},
		{"mood fuera de rango", "checkin", `{"mood":11,"energy":6,"discipline":9}`},
		{"mood faltante", "checkin", `{"energy":6,"discipline":9}`},
		{"type inválido", "movimiento", `{"type":"transfer","amount_centavos":100,"category":"x"}`},
		{"monto cero", "movimiento", `{"type":"expense","amount_centavos":0,"category":"x"}`},
		{"categoría vacía", "movimiento", `{"type":"expense","amount_centavos":100,"category":"  "}`},
		{"uuid inválido", "habito", `{"habit_id":"no-es-uuid"}`},
		{"progress fuera de rango", "meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":101}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseActionPayload(c.kind, c.args); err == nil {
				t.Errorf("esperaba error para %s %s", c.kind, c.args)
			}
		})
	}
}

func TestActionSummaryPorKind(t *testing.T) {
	got := actionSummary("checkin", []byte(`{"mood":8,"energy":6,"discipline":9}`))
	for _, want := range []string{"check-in", "8", "6", "9"} {
		if !strings.Contains(strings.ToLower(got), want) {
			t.Errorf("summary %q no contiene %q", got, want)
		}
	}
	if s := actionSummary("habito", []byte(`{"habit_id":"x"}`)); s == "" {
		t.Error("summary de hábito vacío")
	}
}

func TestBuildChatToolsCuatro(t *testing.T) {
	tools := buildChatTools()
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
		if len(tl.Parameters) == 0 || tl.Description == "" {
			t.Errorf("tool %s incompleta", tl.Name)
		}
	}
	for _, want := range []string{"registrar_checkin", "registrar_movimiento", "marcar_habito", "actualizar_meta"} {
		if !names[want] {
			t.Errorf("falta tool %s", want)
		}
	}
}
