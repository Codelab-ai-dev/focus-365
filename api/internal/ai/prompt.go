package ai

// buildPrompt arma los mensajes system+user para Groq. El system fija idioma
// (español), tono (entrenador cálido) y largo (1-3 frases); el user lleva el
// snapshot del día en JSON para que el modelo detecte patrones reales.
func buildPrompt(snapshotJSON string) (system, user string) {
	system = "Eres un entrenador personal cálido y directo. " +
		"Escribe SIEMPRE en español, en un solo párrafo de 1 a 3 frases, en texto plano (sin markdown ni listas). " +
		"A partir de los datos del día del usuario, detecta el patrón más accionable " +
		"(racha en riesgo, gasto alto, ánimo o energía altos para aprovechar, metas vencidas) " +
		"y dale un consejo concreto y motivador para hoy. No saludes ni te despidas."
	user = "Estos son mis datos de hoy (JSON):\n" + snapshotJSON
	return system, user
}
