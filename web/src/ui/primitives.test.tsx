import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Card } from "./Card";
import { Chip } from "./Chip";
import { Button } from "./Button";
import { Input } from "./Input";

describe("Card", () => {
  it("renderiza children y acepta className extra", () => {
    render(<Card className="extra">contenido</Card>);
    const el = screen.getByText("contenido");
    expect(el.className).toContain("extra");
    expect(el.className).toContain("border-ink");
  });

  it("interactive agrega el hover de levantamiento", () => {
    render(<Card interactive>hover</Card>);
    expect(screen.getByText("hover").className).toContain("hover:");
  });
});

describe("Chip", () => {
  it("aplica la variante de color", () => {
    render(<Chip variant="money">+$3,200</Chip>);
    expect(screen.getByText("+$3,200").className).toContain("bg-money-bg");
  });
});

describe("Button", () => {
  it("dispara onClick y respeta disabled", async () => {
    const onClick = vi.fn();
    const { rerender } = render(<Button onClick={onClick}>Guardar</Button>);
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    expect(onClick).toHaveBeenCalledTimes(1);

    rerender(<Button onClick={onClick} disabled>Guardar</Button>);
    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled();
  });

  it("la variante ghost usa surface", () => {
    render(<Button variant="ghost">Cancelar</Button>);
    expect(screen.getByRole("button", { name: "Cancelar" }).className).toContain("bg-surface");
  });
});

describe("Input", () => {
  it("propaga props y escribe", async () => {
    render(<Input aria-label="Email" placeholder="Email" />);
    const input = screen.getByLabelText("Email");
    await userEvent.type(input, "a@b.com");
    expect((input as HTMLInputElement).value).toBe("a@b.com");
  });
});
