import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { BarChart } from "./BarChart";

describe("BarChart", () => {
  it("dibuja una barra por dato", () => {
    const { container } = render(
      <BarChart data={[{ label: "16/6", value: 10 }, { label: "23/6", value: 20 }]} unit="kg" />
    );
    expect(container.querySelectorAll("rect")).toHaveLength(2);
    const svg = container.querySelector("svg");
    expect(svg?.getAttribute("aria-label")).toContain("16/6");
  });

  it("lista vacía muestra 'sin datos'", () => {
    const { getByText } = render(<BarChart data={[]} />);
    expect(getByText("sin datos")).toBeInTheDocument();
  });
});
