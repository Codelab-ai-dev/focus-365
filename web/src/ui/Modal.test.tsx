import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Modal } from "./Modal";

afterEach(() => {
  document.body.style.overflow = "";
});

describe("Modal", () => {
  it("no renderiza nada cuando open=false", () => {
    render(
      <Modal open={false} onClose={() => {}} title="Título">
        <p>contenido</p>
      </Modal>
    );
    expect(screen.queryByText("contenido")).not.toBeInTheDocument();
  });

  it("renderiza el título y los hijos cuando open=true", () => {
    render(
      <Modal open onClose={() => {}} title="Lun 16 jun">
        <p>contenido</p>
      </Modal>
    );
    expect(screen.getByText("Lun 16 jun")).toBeInTheDocument();
    expect(screen.getByText("contenido")).toBeInTheDocument();
  });

  it("llama onClose al presionar Escape", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>x</p>
      </Modal>
    );
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("llama onClose al click en el overlay pero no dentro del contenido", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>adentro</p>
      </Modal>
    );
    // click dentro del contenido: NO cierra
    fireEvent.click(screen.getByText("adentro"));
    expect(onClose).not.toHaveBeenCalled();
    // click en el overlay (role=dialog): cierra
    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("llama onClose con el botón ✕", () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="T">
        <p>x</p>
      </Modal>
    );
    fireEvent.click(screen.getByLabelText("Cerrar"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
