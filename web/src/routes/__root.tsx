import { createRootRoute, Outlet } from "@tanstack/react-router";
import { TopBar } from "@/components/TopBar";
import { UpdateToast } from "@/ui/UpdateToast";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-bg text-ink">
      <TopBar />
      <Outlet />
      <UpdateToast />
    </div>
  ),
});
