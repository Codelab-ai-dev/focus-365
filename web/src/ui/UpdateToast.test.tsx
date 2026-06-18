import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const updateSpy = vi.fn();
let state: {
  needRefresh: [boolean, (v: boolean) => void];
  offlineReady: [boolean, (v: boolean) => void];
  updateServiceWorker: typeof updateSpy;
};

vi.mock("virtual:pwa-register/react", () => ({
  useRegisterSW: () => state,
}));

import { UpdateToast } from "./UpdateToast";

beforeEach(() => {
  updateSpy.mockClear();
  state = {
    needRefresh: [false, vi.fn()],
    offlineReady: [false, vi.fn()],
    updateServiceWorker: updateSpy,
  };
});

describe("UpdateToast", () => {
  it("no renderiza nada sin update ni offlineReady", () => {
    const { container } = render(<UpdateToast />);
    expect(container).toBeEmptyDOMElement();
  });

  it("muestra el aviso de actualización y recarga al tocar", async () => {
    state.needRefresh = [true, vi.fn()];
    render(<UpdateToast />);
    expect(screen.getByText(/actualización/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /recargar/i }));
    expect(updateSpy).toHaveBeenCalledWith(true);
  });

  it("muestra 'lista para usar sin conexión' cuando offlineReady", () => {
    state.offlineReady = [true, vi.fn()];
    render(<UpdateToast />);
    expect(screen.getByText(/sin conexión/i)).toBeInTheDocument();
  });
});
