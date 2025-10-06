import '@mantine/core/styles.css';
import { routeTree } from '@/routeTree.gen';
import { MantineProvider } from '@mantine/core';
import { createRouter, RouterProvider } from '@tanstack/react-router';
import { StrictMode } from 'react';
import ReactDOM from 'react-dom/client';

const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  // eslint-disable-next-line @typescript-eslint/consistent-type-definitions
  interface Register {
    router: typeof router;
  }
}

// Render the app
const rootElement = document.getElementById('root');

if (!rootElement) {
  throw Error('Root Element not found');
}

if (!rootElement.innerHTML) {
  const root = ReactDOM.createRoot(rootElement);
  root.render(
    <StrictMode>
      <MantineProvider defaultColorScheme="auto">
        <RouterProvider router={router} />
      </MantineProvider>
    </StrictMode>,
  );
}
