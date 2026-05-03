import AppShellLayout from '@/layouts/app-shell/AppShellLayout';
import { initKeycloak } from '@/services/auth/authService';
import { Outlet } from '@tanstack/react-router';
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools';
import { useEffect } from 'react';

export function RootLayout() {
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
}
