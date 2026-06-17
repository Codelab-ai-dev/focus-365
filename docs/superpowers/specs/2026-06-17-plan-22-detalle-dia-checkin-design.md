# Plan 22 — Detalle del día en el historial del check-in — Diseño

**Fecha:** 2026-06-17
**Estado:** Aprobado (diseño) · pendiente plan de implementación
**Autor:** Gustavo (con Claude)

## 1. Visión y alcance

En el historial del check-in (ruta `/check-in`), cada fila pasa a ser
**clickeable**; al tocarla se abre un **modal** con el detalle de ese día. Hoy la
fila solo muestra fecha + ánimo/energía y no hay forma de ver lo que se escribió.

**Rebanada solo-frontend:** `list(30)` ya devuelve el `CheckIn` completo (las 4
dimensiones, win, avoided), así que el modal se arma con datos ya cargados. Los
compromisos de ese día se piden con el endpoint existente `getDue(date)`. **No
hay cambios de backend ni migración.**

**Decisiones (brainstorming):**
- El modal muestra el **check-in completo + los compromisos del día**.
- Las dimensiones/win/avoided **vacías se ocultan** (un check-in parcial de la IA
  puede traer solo ánimo/energía).
- Cerrar con **✕, tecla Esc o click en el fondo**.

**Fuera de alcance:** editar el día desde el modal (es solo lectura); cualquier
cambio de backend; paginación del historial.

## 2. Componentes

### `web/src/ui/Modal.tsx` (nuevo, reutilizable)
Shell de modal neo-brutalista:
- Render vía `createPortal` a `document.body`.
- Fondo oscurecido (overlay) + `Card` centrada con `border-2 border-ink` y
  `shadow-brutal`; alto máximo con scroll interno si el contenido es largo.
- Props `{ open: boolean; onClose: () => void; title: string; children }`.
- `role="dialog"`, `aria-modal="true"`, `aria-label`/`aria-labelledby` con el
  título; botón ✕ con `aria-label="Cerrar"`.
- Cierra al presionar **Esc** (listener mientras `open`) y al click en el
  **overlay** (no al click dentro de la Card).
- Bloquea el scroll del `body` mientras está abierto (restaura al cerrar).
- Si `!open` → no renderiza nada.

### `DayDetailModal` (en `web/src/routes/check-in.tsx`)
Recibe el `CheckIn` del día seleccionado y arma el contenido dentro de `Modal`:
- **Header:** fecha legible (p. ej. "lun 16 jun"), parseando `YYYY-MM-DD` como
  fecha **local** (no `new Date("YYYY-MM-DD")`, que es UTC y puede correr un día).
- **Ánimo / Energía:** siempre.
- **Las 4 dimensiones** (Espiritual/Emocional/Física/Financiera): cada una solo
  si su texto no está vacío, con el mismo `Chip` de variante que el formulario.
- **🏆 Win** y **🚫 Evité:** solo si tienen texto.
- **📋 Compromisos del día:** `useQuery(["commitments","due", date], () =>
  getDue(date))`; muestra contador `N/M ✓` y cada compromiso con ✓ (cumplido) o
  ✗ (no). Si la lista está vacía → "sin compromisos ese día".

## 3. Cambios en `check-in.tsx`

- Nuevo estado `const [selected, setSelected] = useState<CheckIn | null>(null)`.
- En el historial, cada `RevealItem` envuelve un `<button type="button">`
  (mismo look de la `Card` actual) con `onClick={() => setSelected(ci)}` y
  `aria-label={\`Ver detalle del ${ci.date}\`}`.
- `<DayDetailModal checkin={selected} onClose={() => setSelected(null)} />` se
  monta siempre y se muestra cuando `selected !== null` (el `open` del `Modal`
  es `selected !== null`).
- El formulario de hoy, la sección "ayer te comprometiste" y el backend no
  cambian.

## 4. Manejo de errores

- `getDue` falla → la sección de compromisos queda vacía (sin romper el modal).
- Día con solo ánimo/energía (sin reflexiones) → se muestran únicamente esos
  campos; las secciones vacías no aparecen.
- Esc / click en el fondo / ✕ cierran el modal y restauran el scroll del body.

## 5. Testing

- **`ui/Modal.tsx`:** renderiza los hijos cuando `open=true`; no renderiza nada
  cuando `open=false`; llama `onClose` al presionar Esc; llama `onClose` al click
  en el overlay pero **no** al click dentro de la Card.
- **`check-in.tsx`:** tocar una fila del historial abre el modal con los datos de
  ese día (fecha, ánimo/energía, una dimensión con texto visible); una dimensión
  vacía no se muestra; la sección de compromisos refleja un `getDue` mockeado
  (contador y ✓/✗); cerrar con ✕ desmonta el contenido.

## 6. Criterios de aceptación

- Tocar un día del historial abre un modal con el detalle de ese día.
- Se ven ánimo/energía, las dimensiones con contenido, win/evité si los hay, y
  los compromisos de ese día con su estado.
- El modal cierra con ✕, Esc y click en el fondo.
- Suite web en verde + build; smoke de producción (abrir un día, ver el detalle).
