import { RootLayout } from '@/layouts/RootLayout';
import { createRootRoute } from '@tanstack/react-router';

export const Route = createRootRoute({
  component: RootLayout,
});
