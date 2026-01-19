package auth

import (
	"context"
	"log/slog"

	"go-services/bff/internal/common/tokenutil"
	"go-services/bff/internal/config"
	"go-services/library/apperror"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type oidcVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error)
}

type oauth2Config interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	TokenSource(ctx context.Context, t *oauth2.Token) oauth2.TokenSource
}

type AuthCommandService struct {
	log            *slog.Logger
	providerConfig *config.OIDCProviderConfig
	oauth2Config   oauth2Config
	verifier       oidcVerifier
	tokenGenerator *tokenutil.TokenUtil
}

func NewAuthCommandService(
	log *slog.Logger,
	providerConfig *config.OIDCProviderConfig,
	oauth2Client oauth2Config,
	verifier oidcVerifier,
	tokenGenerator *tokenutil.TokenUtil,
) *AuthCommandService {
	return &AuthCommandService{
		log:            log,
		providerConfig: providerConfig,
		oauth2Config:   oauth2Client,
		verifier:       verifier,
		tokenGenerator: tokenGenerator,
	}
}

func (s *AuthCommandService) GenerateAuthCodeURL() (state string, authURL string, err error) {
	state, err = s.tokenGenerator.GenerateStateToken()
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeInternalError, err, "faield to generate state")
	}

	authURL = s.oauth2Config.AuthCodeURL(state, oauth2.SetAuthURLParam("response_type", "code"))
	return state, authURL, nil
}

// Callback handler - receives redirect from Keycloak
func (s *AuthCommandService) AuthenticateUser(
	ctx context.Context,
	authCode string,
) (sessionToken string, accessToken string, err error) {
	// Exchange authorization code for tokens
	oauth2Token, err := s.oauth2Config.Exchange(ctx, authCode)
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeExternalService, err, "failed to exchange token")
	}

	// Verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return "", "", apperror.New(apperror.CodeExternalService, "no id_token in response")
	}

	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeUnauthorized, err, "failed to verify ID token")
	}

	// Extract user info
	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", "", apperror.Wrap(apperror.CodeExternalService, err, "failed to decode ID token claims")
	}

	sessionToken, err = s.tokenGenerator.GenerateStateToken()
	if err != nil {
		return "", "", apperror.Wrap(apperror.CodeInternalError, err, "faield to generate session token: w")
	}

	// TODO: store refresh token
	return sessionToken, oauth2Token.AccessToken, nil
}

// func (s *AuthCommandService) LogoutHandler(w http.ResponseWriter, r *http.Request) {
// 	sessionToken, err := r.Cookie("session_token")
// 	if err == nil {
// 		s.sessionCommandService.Delete(r.Context(), sessionToken.Value)
// 	}
//
// 	// Clear session cookie
// 	// s.clearCookie(w, sessionTokenCookieName)
// 	http.SetCookie(w, &http.Cookie{
// 		Name:     "session_token",
// 		Value:    "",
// 		Path:     "/",
// 		MaxAge:   -1,
// 		HttpOnly: true,
// 	})
//
// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(map[string]string{
// 		"message": "Logged out successfully",
// 	})
// }

// Refresh token handler
// func (s *AuthCommandService) RefreshHandler(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}
//
// 	sessionToken, err := r.Cookie("session_token")
// 	if err != nil {
// 		http.Error(w, "Unauthorized", http.StatusUnauthorized)
// 		return
// 	}
//
// 	session, err := s.sessionQueryService.GetBySessionID(r.Context(), sessionToken.Value)
// 	if err != nil {
// 		http.Error(w, "Invalid session", http.StatusUnauthorized)
// 		return
// 	}
//
// 	// Create token source with refresh token
// 	token := &oauth2.Token{
// 		RefreshToken: session.RefreshToken,
// 	}
//
// 	ctx := context.Background()
// 	tokenSource := s.oauth2Client.TokenSource(ctx, token)
// 	newToken, err := tokenSource.Token()
// 	if err != nil {
// 		http.Error(w, "Failed to refresh token", http.StatusInternalServerError)
// 		return
// 	}
//
// 	// Update session
// 	session.AccessToken = newToken.AccessToken
// 	if newToken.RefreshToken != "" {
// 		session.RefreshToken = newToken.RefreshToken
// 	}
// 	session.ExpiresAt = newToken.Expiry
// 	// s.sessions.Set(r.Context(), sessionToken.Value, session)
//
// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(map[string]string{
// 		"message": "Token refreshed",
// 	})
// }
