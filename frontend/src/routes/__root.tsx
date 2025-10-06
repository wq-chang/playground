import AppShellLayout from '@/layouts/app-shell/AppShellLayout';
import { initKeycloak } from '@/services/auth/authService';
import { createRootRoute, Outlet } from '@tanstack/react-router';
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools';
import { useEffect } from 'react';

const Root = () => {
  useEffect(() => {
    initKeycloak().catch((err: unknown) => {
      console.error(err);
    });
  }, []);

  return (
    <>
      <AppShellLayout>
        <Outlet />
      </AppShellLayout>
      <TanStackRouterDevtools position="bottom-right" />
    </>
  );
};

export const Route = createRootRoute({
  component: Root,
});
