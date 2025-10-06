import { create } from 'zustand';

type AuthState = {
  accessToken: string | null;
  refreshToken: string | null;
  isAuthenticated: boolean;
  setAuth: (
    isAuthenticated: boolean,
    accessToken: string | null,
    refreshToken: string | null,
  ) => void;
  removeAuth: () => void;
};

export const useAuthStore = create<AuthState>((set) => ({
  isAuthenticated: false,
  accessToken: null,
  refreshToken: null,

  setAuth: (isAuthenticated, accessToken, refreshToken) => {
    set({ isAuthenticated, accessToken, refreshToken });
  },

  removeAuth: () => {
    set({ isAuthenticated: false, accessToken: null, refreshToken: null });
  },
}));
