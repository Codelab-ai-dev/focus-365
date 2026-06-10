import { createRootRoute, Outlet } from "@tanstack/react-router";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-ink-950 text-sand-100">
      <Outlet />
    </div>
  ),
});
