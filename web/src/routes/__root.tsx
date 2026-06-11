import { createRootRoute, Outlet } from "@tanstack/react-router";
import { TopBar } from "@/components/TopBar";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-ink-950 text-sand-100">
      <TopBar />
      <Outlet />
    </div>
  ),
});
