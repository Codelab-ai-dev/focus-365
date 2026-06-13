package ai

// extractSystemPrompt instruye al modelo a devolver SOLO JSON con los
// movimientos detectados en el comprobante.
const extractSystemPrompt = `Eres un extractor de movimientos financieros. Recibes el contenido de un comprobante (recibo, ticket, estado de cuenta o CSV) y devuelves SOLO un objeto JSON con esta forma exacta:
{"movimientos":[{"type":"income|expense","amount_centavos":<entero positivo en centavos>,"category":"<categoría corta>","remark":"<opcional>","occurred_on":"YYYY-MM-DD (opcional, la fecha del movimiento si aparece)"}]}
Reglas: gastos/cargos = expense, ingresos/abonos = income. El monto SIEMPRE en centavos enteros y positivo (ej: $250.00 → 25000). Si no hay fecha clara, omite occurred_on. No incluyas transferencias internas. No inventes montos. Si no hay movimientos, devuelve {"movimientos":[]}. Responde ÚNICAMENTE el JSON, sin texto adicional.`
