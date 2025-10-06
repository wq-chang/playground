import { useAuthStore } from '@/stores/authStore';
import { beforeEach, describe, expect, it, vi } from 'vitest';

let authService: typeof import('@/services/auth/authService');

const { initMock, loginMock, logoutMock, setAuthMock, removeAuthMock } =
  vi.hoisted(() => {
    return {
      initMock: vi.fn().mockResolvedValue(true),
      loginMock: vi.fn(),
      logoutMock: vi.fn(),
      setAuthMock: vi.fn(),
      removeAuthMock: vi.fn(),
    };
  });

vi.mock('@/stores/authStore', () => {
  return {
    useAuthStore: {
      getState: vi.fn(() => ({
        setAuth: setAuthMock,
        removeAuth: removeAuthMock,
      })),
    },
  };
});

vi.mock('keycloak-js', () => {
  return {
    default: vi.fn().mockImplementation(() => ({
      init: initMock,
      login: loginMock,
      logout: logoutMock,
      token: 'fake-token',
      refreshToken: 'fake-refresh-token',
      authenticated: true,
    })),
  };
});

beforeEach(async () => {
  vi.clearAllMocks();
  vi.resetModules();
  authService = await import('@/services/auth/authService');
});

describe('initKeycloak', () => {
  it('should initialize Keycloak and set auth', async () => {
    await authService.initKeycloak();

    const store = useAuthStore.getState();
    expect(store.setAuth).toHaveBeenCalledWith(
      true,
      'fake-token',
      'fake-refresh-token',
    );
  });

  it('should not re-init if already initiated', async () => {
    await authService.initKeycloak();
    await authService.initKeycloak();

    const store = useAuthStore.getState();
    expect(store.setAuth).toHaveBeenCalledTimes(1);
  });
});

describe('login', () => {
  it('should call login and set auth', async () => {
    await authService.login();

    const store = useAuthStore.getState();
    expect(store.setAuth).toHaveBeenCalledWith(
      true,
      'fake-token',
      'fake-refresh-token',
    );
  });
});

describe('logout', () => {
  it('should call logout and remove auth', async () => {
    await authService.logout();

    const store = useAuthStore.getState();
    expect(store.removeAuth).toHaveBeenCalled();
  });
});
