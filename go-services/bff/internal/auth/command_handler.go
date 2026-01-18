package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go-services/bff/internal/api"
	"go-services/bff/internal/config"
	"go-services/library/apperror"
)

const (
	stateCookieName    = "oauth_state"
	returnToCookieName = "return_to"
)

type authService interface {
	GenerateAuthCodeURL() (state string, authURL string, err error)
	AuthenticateUser(
		ctx context.Context,
		authCode string,
	) (sessionToken string, accessToken string, err error)
}

type AuthCommandHandler struct {
	log                *slog.Logger
	providerConfig     *config.OIDCProviderConfig
	useHTTPS           bool
	frontendBaseURL    string
	authCommandService authService
}

func NewAuthCommandHandler(
	log *slog.Logger,
	providerConfig *config.OIDCProviderConfig,
	useHTTPS bool,
	frontendBaseURL string,
	authService authService,
) *AuthCommandHandler {
	return &AuthCommandHandler{
		log:                log,
		providerConfig:     providerConfig,
		useHTTPS:           useHTTPS,
		frontendBaseURL:    frontendBaseURL,
		authCommandService: authService,
	}
}

func (h *AuthCommandHandler) LoginHandler(w http.ResponseWriter, r *http.Request) error {
	returnTo := r.URL.Query().Get(returnToCookieName)
	if returnTo == "" {
		returnTo = "/"
	}

	state, authURL, err := h.authCommandService.GenerateAuthCodeURL()
	if err != nil {
		return fmt.Errorf("faild to get auth url: %w", err)
	}

	// Store in cookie for later (can't include it in Keycloak redirect)
	h.setCookie(w, returnToCookieName, returnTo, 5*time.Minute)
	// Store state in cookie for CSRF protection
	h.setCookie(w, stateCookieName, state, 5*time.Minute)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	return nil
}

func (h *AuthCommandHandler) CallbackHandler(w http.ResponseWriter, r *http.Request) error {
	var returnTo string
	returnToCookie, err := r.Cookie(returnToCookieName)
	if err == nil {
		returnTo = returnToCookie.Value
	}

	// Verify state for CSRF protection
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return apperror.New(
			fmt.Sprintf("missing required cookie: %v", stateCookieName),
			apperror.CodeUnauthorized,
			err,
		)
	}
	if r.URL.Query().Get("state") != stateCookie.Value {
		return apperror.New("state mismatch", apperror.CodeUnauthorized, err)
	}

	authCode := r.URL.Query().Get("code")
	// TODO: change return value to struct
	sessionToken, accessToken, err := h.authCommandService.AuthenticateUser(r.Context(), authCode)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	h.clearCookie(w, returnToCookieName)
	h.clearCookie(w, stateCookieName)
	h.setCookie(w, api.AccessTokenCookieName, accessToken, 24*time.Hour)
	h.setCookie(w, api.SessionTokenCookieName, sessionToken, 24*time.Hour)

	frontendURL := fmt.Sprintf("%s%s", h.frontendBaseURL, returnTo)
	http.Redirect(w, r, frontendURL, http.StatusFound)

	return nil
}

func (h *AuthCommandHandler) LogutoutHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.providerConfig.LogoutURL, http.StatusFound)
}

func (h *AuthCommandHandler) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.useHTTPS,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *AuthCommandHandler) clearCookie(w http.ResponseWriter, name string) {
	h.setCookie(w, name, "", -1*time.Second)
}
