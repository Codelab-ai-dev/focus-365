import { describe, it, expect } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MotionConfig } from "framer-motion";
import { Stat } from "./Stat";
import { ProgressBar } from "./ProgressBar";
import { PageTransition } from "./PageTransition";
import { Reveal, RevealItem } from "./Reveal";

function renderStill(ui: React.ReactNode) {
  return render(<MotionConfig reducedMotion="always">{ui}</MotionConfig>);
}

describe("Stat", () => {
  it("muestra etiqueta y el valor final con sufijo", async () => {
    renderStill(<Stat label="Racha actual" value={12} suffix=" días" />);
    expect(screen.getByText("Racha actual")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("12 días")).toBeInTheDocument());
  });

  it("acepta un formateador (montos)", async () => {
    renderStill(
      <Stat label="Neto" value={320000} format={(n) => `$${(n / 100).toLocaleString("en-US")}`} />
    );
    await waitFor(() => expect(screen.getByText("$3,200")).toBeInTheDocument());
  });
});

describe("ProgressBar", () => {
  it("expone el porcentaje vía role progressbar", () => {
    renderStill(<ProgressBar value={60} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "60");
  });

  it("clampa fuera de rango", () => {
    renderStill(<ProgressBar value={140} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
  });
});

describe("PageTransition y Reveal", () => {
  it("renderizan children", () => {
    renderStill(
      <PageTransition>
        <Reveal>
          <RevealItem>uno</RevealItem>
          <RevealItem>dos</RevealItem>
        </Reveal>
      </PageTransition>
    );
    expect(screen.getByText("uno")).toBeInTheDocument();
    expect(screen.getByText("dos")).toBeInTheDocument();
  });
});
