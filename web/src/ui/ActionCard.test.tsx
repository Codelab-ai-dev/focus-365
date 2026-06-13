import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionCard } from "./ActionCard";
import type { Action } from "@/lib/ai";

const base: Action = { id: "a1", kind: "movimiento", payload: { type: "expense", amount_centavos: 25000, category: "comida" }, status: "proposed" };

describe("ActionCard", () => {
  it("muestra título y detalle del movimiento y botones en proposed", () => {
    render(<ActionCard action={base} pending={false} onResolve={() => {}} />);
    expect(screen.getByText("Movimiento")).toBeInTheDocument();
    expect(screen.getByText(/Gasto de \$250\.00 en comida/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Confirmar" })).toBeInTheDocument();
  });

  it("done muestra Hecha + Deshacer; resolver llama onResolve", async () => {
    const onResolve = vi.fn();
    render(<ActionCard action={{ ...base, status: "done" }} pending={false} onResolve={onResolve} />);
    await userEvent.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(onResolve).toHaveBeenCalledWith("a1", "undo");
  });
});
