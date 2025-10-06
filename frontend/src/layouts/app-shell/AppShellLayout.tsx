import { CustomNavLink } from '@/components/CustomNavLink';
import { login, logout } from '@/services/auth/authService';
import { useAuthStore } from '@/stores/authStore';
import {
  AppShell,
  AppShellHeader,
  AppShellMain,
  AppShellNavbar,
  AppShellSection,
  Burger,
  Group,
  NavLink,
  ScrollArea,
} from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { LogIn, LogOut } from 'lucide-react';
import { ReactNode } from 'react';

type AppShellLayoutProps = {
  children: ReactNode;
};

const renderAuthButton = (isAuthenticated: boolean) => {
  const icon = isAuthenticated ? <LogOut /> : <LogIn />;
  const label = isAuthenticated ? 'Logout' : 'Login';
  const authAction = isAuthenticated ? logout : login;

  return (
    <NavLink
      leftSection={icon}
      label={label}
      onClick={() => void authAction()}
    />
  );
};

const AppShellLayout = ({ children }: AppShellLayoutProps) => {
  const [opened, { toggle }] = useDisclosure();
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  return (
    <AppShell
      header={{ height: 60 }}
      navbar={{ width: 300, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
      <AppShellHeader>
        <Group h="100%" px="md">
          <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
          Header has a burger icon below sm breakpoint
        </Group>
      </AppShellHeader>
      <AppShellNavbar p="md">
        <AppShellSection grow component={ScrollArea}>
          <CustomNavLink to="/" label="Home" />
          <CustomNavLink to="/about" label="About" />
        </AppShellSection>
        <AppShellSection>{renderAuthButton(isAuthenticated)}</AppShellSection>
      </AppShellNavbar>
      <AppShellMain>{children}</AppShellMain>
    </AppShell>
  );
};

export default AppShellLayout;
