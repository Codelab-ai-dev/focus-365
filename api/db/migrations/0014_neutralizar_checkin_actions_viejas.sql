-- +goose Up
-- Las acciones de check-in confirmadas ANTES de la migración 0013 guardaron su
-- `result` con la forma vieja ({prev:{mood,energy,discipline,note}, date}). El
-- nuevo undo las interpretaría como existed=false y borraría el día (perdiendo
-- las reflexiones 4D). Las marcamos como 'undone' para que ya no se puedan
-- deshacer: el dato del check-in ya está aplicado y se conserva.
UPDATE ai_actions SET status = 'undone'
WHERE kind = 'checkin' AND status = 'done';

-- +goose Down
-- No reversible (no sabemos cuáles eran 'done' originalmente); no-op seguro.
SELECT 1;
