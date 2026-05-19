package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofrs/uuid/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"go-services/backend/internal/idp"
	"go-services/library/apperror"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type tokenProvider interface {
	Token(ctx context.Context) (*oauth2.Token, error)
}

type Config struct {
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
}

func (c Config) Validate() error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(c.BaseURL) == "" {
		missing = append(missing, "BaseURL")
	}
	if strings.TrimSpace(c.Realm) == "" {
		missing = append(missing, "Realm")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		missing = append(missing, "ClientID")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		missing = append(missing, "ClientSecret")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required keycloak config fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

func (c Config) TokenURL() string {
	return fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		strings.TrimRight(c.BaseURL, "/"),
		url.PathEscape(c.Realm),
	)
}

func (c Config) userURL(userID uuid.UUID) string {
	return fmt.Sprintf(
		"%s/admin/realms/%s/users/%s",
		strings.TrimRight(c.BaseURL, "/"),
		url.PathEscape(c.Realm),
		url.PathEscape(userID.String()),
	)
}

type Client struct {
	httpClient    httpDoer
	tokenProvider tokenProvider
	cfg           Config
}

type userRepresentation struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Username  string `json:"username"`
}

var _ idp.UserProvider = (*Client)(nil)

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	oauth2Config := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL(),
	}

	return newClient(cfg, httpClient, oauth2Config), nil
}

func newClient(cfg Config, httpClient httpDoer, tokenProvider tokenProvider) *Client {
	return &Client{
		cfg:           cfg,
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
	}
}

func (c *Client) GetUserByID(ctx context.Context, userID uuid.UUID) (idp.User, error) {
	accessToken, err := c.tokenProvider.Token(ctx)
	if err != nil {
		return idp.User{}, apperror.Wrap(apperror.CodeUnauthorized, err, "failed to authenticate to keycloak")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.userURL(userID), nil)
	if err != nil {
		return idp.User{}, apperror.Wrap(apperror.CodeInternalError, err, "failed to build keycloak user request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken.AccessToken)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return idp.User{}, apperror.Wrap(apperror.CodeExternalService, err, "failed to fetch user %s from keycloak", userID)
	}
	defer closeResponseBody(res.Body)

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return idp.User{}, apperror.New(apperror.CodeUnauthorized, "keycloak rejected user lookup for %s", userID)
	case http.StatusForbidden:
		return idp.User{}, apperror.New(apperror.CodeForbidden, "keycloak denied user lookup for %s", userID)
	case http.StatusNotFound:
		return idp.User{}, apperror.New(apperror.CodeNotFound, "keycloak user %s not found", userID)
	case http.StatusTooManyRequests:
		return idp.User{}, apperror.New(apperror.CodeTooManyRequests, "keycloak rate limited user lookup for %s", userID)
	default:
		return idp.User{}, apperror.New(
			apperror.CodeExternalService,
			"unexpected keycloak response status %d for user %s",
			res.StatusCode,
			userID,
		)
	}

	var payload userRepresentation
	if decodeErr := json.NewDecoder(res.Body).Decode(&payload); decodeErr != nil {
		return idp.User{}, apperror.Wrap(apperror.CodeSerializationError, decodeErr, "failed to decode keycloak user %s", userID)
	}

	id, err := uuid.FromString(payload.ID)
	if err != nil {
		return idp.User{}, apperror.Wrap(apperror.CodeSerializationError, err, "failed to parse keycloak user id %q", payload.ID)
	}

	return idp.User{
		ID:        id,
		FirstName: payload.FirstName,
		LastName:  payload.LastName,
		Email:     payload.Email,
		Username:  payload.Username,
	}, nil
}

func closeResponseBody(body io.Closer) {
	if err := body.Close(); err != nil {
		return
	}
}
