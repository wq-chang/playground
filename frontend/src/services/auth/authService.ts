import { useAuthStore } from '@/stores/authStore';
import Keycloak from 'keycloak-js';

const keycloak = new Keycloak({
  url: import.meta.env.VITE_REALM_URL,
  realm: import.meta.env.VITE_REALM_NAME,
  clientId: import.meta.env.VITE_REALM_CLIENT_ID,
});

let isInitiated = false;

export const initKeycloak = async () => {
  if (isInitiated) {
    return;
  }
  isInitiated = true;

  const isAuthenticated = await keycloak.init({
    checkLoginIframe: false,
    onLoad: 'check-sso',
    silentCheckSsoRedirectUri:
      window.location.origin + '/silent-check-sso.html',
  });

  useAuthStore
    .getState()
    .setAuth(
      isAuthenticated,
      keycloak.token ?? null,
      keycloak.refreshToken ?? null,
    );
};

export const login = async () => {
  await keycloak.login({
    redirectUri: window.location.origin + '/',
  });

  useAuthStore
    .getState()
    .setAuth(
      keycloak.authenticated,
      keycloak.token ?? null,
      keycloak.refreshToken ?? null,
    );
};

export const logout = async () => {
  await keycloak.logout({
    redirectUri: window.location.origin + '/',
  });

  useAuthStore.getState().removeAuth();
};
