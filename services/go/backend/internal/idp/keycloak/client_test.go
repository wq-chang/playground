package keycloak

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofrs/uuid/v5"
	"golang.org/x/oauth2"

	"go-services/backend/internal/idp"
	"go-services/library/apperror"
	"go-services/library/assert"
	"go-services/library/require"
)

type fakeTokenProvider struct {
	token *oauth2.Token
	err   error
}

func (p fakeTokenProvider) Token(context.Context) (*oauth2.Token, error) {
	if p.err != nil {
		return nil, p.err
	}

	return p.token, nil
}

func TestNewClient_ValidateConfig(t *testing.T) {
	_, err := NewClient(Config{}, nil)

	assert.ErrorContains(t, err, "missing required keycloak config fields", "expected config validation error")
}

func TestGetUserByID(t *testing.T) {
	userID, err := uuid.NewV4()
	require.NoError(t, err, "failed to generate user id")

	expectedUser := idp.User{
		ID:        userID,
		FirstName: "Ada",
		LastName:  "Lovelace",
		Email:     "ada@example.com",
		Username:  "adal",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Method, http.MethodGet, "request method")
		assert.Equal(
			t,
			r.URL.Path,
			"/admin/realms/playground/users/"+userID.String(),
			"request path",
		)
		assert.Equal(t, r.Header.Get("Accept"), "application/json", "accept header")
		assert.Equal(t, r.Header.Get("Authorization"), "Bearer access-token", "authorization header")

		_, writeErr := w.Write([]byte(`{"id":"` + userID.String() + `","firstName":"Ada","lastName":"Lovelace","email":"ada@example.com","username":"adal"}`))
		require.NoError(t, writeErr, "failed to write response")
	}))
	defer ts.Close()

	client := newClient(
		Config{BaseURL: ts.URL, Realm: "playground", ClientID: "", ClientSecret: ""},
		ts.Client(),
		fakeTokenProvider{token: &oauth2.Token{AccessToken: "access-token"}, err: nil},
	)

	got, err := client.GetUserByID(context.Background(), userID)

	require.NoError(t, err, "expected successful lookup")
	assert.Equal(t, got, expectedUser, "fetched user")
}

func TestGetUserByID_TokenError(t *testing.T) {
	userID, err := uuid.NewV4()
	require.NoError(t, err, "failed to generate user id")

	client := newClient(
		Config{BaseURL: "http://keycloak", Realm: "playground", ClientID: "", ClientSecret: ""},
		http.DefaultClient,
		fakeTokenProvider{token: nil, err: errors.New("boom")},
	)

	_, err = client.GetUserByID(context.Background(), userID)

	var appErr *apperror.AppError
	if assert.ErrorAs(t, err, &appErr, "expected app error") {
		assert.Equal(t, appErr.Code, apperror.CodeUnauthorized, "app error code")
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	userID, err := uuid.NewV4()
	require.NoError(t, err, "failed to generate user id")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := newClient(
		Config{BaseURL: ts.URL, Realm: "playground", ClientID: "", ClientSecret: ""},
		ts.Client(),
		fakeTokenProvider{token: &oauth2.Token{AccessToken: "access-token"}, err: nil},
	)

	_, err = client.GetUserByID(context.Background(), userID)

	var appErr *apperror.AppError
	if assert.ErrorAs(t, err, &appErr, "expected app error") {
		assert.Equal(t, appErr.Code, apperror.CodeNotFound, "app error code")
	}
}

func TestGetUserByID_InvalidPayload(t *testing.T) {
	userID, err := uuid.NewV4()
	require.NoError(t, err, "failed to generate user id")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write([]byte(`{"id":"not-a-uuid","firstName":"Ada","lastName":"Lovelace","email":"ada@example.com","username":"adal"}`))
		require.NoError(t, writeErr, "failed to write response")
	}))
	defer ts.Close()

	client := newClient(
		Config{BaseURL: ts.URL, Realm: "playground", ClientID: "", ClientSecret: ""},
		ts.Client(),
		fakeTokenProvider{token: &oauth2.Token{AccessToken: "access-token"}, err: nil},
	)

	_, err = client.GetUserByID(context.Background(), userID)

	var appErr *apperror.AppError
	if assert.ErrorAs(t, err, &appErr, "expected app error") {
		assert.Equal(t, appErr.Code, apperror.CodeSerializationError, "app error code")
	}
}
