package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iden3/go-iden3-core/v2/w3c"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/polygonid/sh-id-platform/internal/common"
	"github.com/polygonid/sh-id-platform/internal/core/domain"
	"github.com/polygonid/sh-id-platform/internal/core/ports"
	"github.com/polygonid/sh-id-platform/internal/db/tests"
)

func TestServer_DeleteConnection(t *testing.T) {
	const (
		method     = "polygonid"
		blockchain = "polygon"
		network    = "amoy"
		BJJ        = "BJJ"
	)
	ctx := context.Background()
	server := newTestServer(t, nil)

	handler := getHandler(ctx, server)

	iden, err := server.Services.identity.Create(ctx, "polygon-test", &ports.DIDCreationOptions{Method: method, Blockchain: blockchain, Network: network, KeyType: BJJ})
	require.NoError(t, err)
	issuerDID, err := w3c.ParseDID(iden.Identifier)
	require.NoError(t, err)

	fixture := tests.NewFixture(storage)

	userDID, err := w3c.ParseDID("did:polygonid:polygon:mumbai:2qH7XAwYQzCp9VfhpNgeLtK2iCehDDrfMWUCEg5ig5")
	require.NoError(t, err)

	userDID2, err := w3c.ParseDID("did:polygonid:polygon:mumbai:2qNytPv6dKKhfqopjBdXJU1vSVb3Lbgcidved32R64")
	require.NoError(t, err)

	conn := fixture.CreateConnection(t, &domain.Connection{
		ID:         uuid.New(),
		IssuerDID:  *issuerDID,
		UserDID:    *userDID,
		IssuerDoc:  nil,
		UserDoc:    nil,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	})

	conn2 := fixture.CreateConnection(t, &domain.Connection{
		ID:         uuid.New(),
		IssuerDID:  *issuerDID,
		UserDID:    *userDID2,
		IssuerDoc:  nil,
		UserDoc:    nil,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	})

	_ = fixture.CreateClaim(t, &domain.Claim{
		ID:              uuid.New(),
		Identifier:      common.ToPointer(issuerDID.String()),
		Issuer:          issuerDID.String(),
		SchemaHash:      "ca938857241db9451ea329256b9c06e5",
		SchemaURL:       "https://raw.githubusercontent.com/iden3/claim-schema-vocab/main/schemas/json-ld/auth.json-ld",
		SchemaType:      "AuthBJJCredential",
		OtherIdentifier: userDID2.String(),
		Expiration:      0,
		Version:         0,
		RevNonce:        0,
		CoreClaim:       domain.CoreClaim{},
		Status:          nil,
	})

	type expected struct {
		httpCode int
		message  *string
	}

	type testConfig struct {
		name             string
		connID           uuid.UUID
		deleteCredential bool
		revokeCredential bool
		auth             func() (string, string)
		expected         expected
	}

	for _, tc := range []testConfig{
		{
			name: "No auth header",
			auth: authWrong,
			expected: expected{
				httpCode: http.StatusUnauthorized,
			},
		},
		{
			name:   "should get an error, not existing connection",
			connID: uuid.New(),
			auth:   authOk,
			expected: expected{
				httpCode: http.StatusBadRequest,
				message:  common.ToPointer("The given connection does not exist"),
			},
		},
		{
			name:   "should delete the connection",
			connID: conn,
			auth:   authOk,
			expected: expected{
				httpCode: http.StatusOK,
				message:  common.ToPointer("Connection successfully deleted."),
			},
		},
		{
			name:             "should delete the connection and revoke + delete credentials",
			connID:           conn2,
			deleteCredential: true,
			revokeCredential: true,
			auth:             authOk,
			expected: expected{
				httpCode: http.StatusOK,
				message:  common.ToPointer("Connection successfully deleted. Credentials successfully deleted. Credentials successfully revoked."),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			urlTest := fmt.Sprintf("/v1/%s/connections/%s", issuerDID, tc.connID.String())
			parsedURL, err := url.Parse(urlTest)
			require.NoError(t, err)
			values := parsedURL.Query()
			if tc.deleteCredential {
				values.Add("deleteCredentials", "true")
			}
			if tc.revokeCredential {
				values.Add("revokeCredentials", "true")
			}
			parsedURL.RawQuery = values.Encode()
			req, err := http.NewRequest(http.MethodDelete, parsedURL.String(), nil)
			req.SetBasicAuth(tc.auth())
			require.NoError(t, err)

			handler.ServeHTTP(rr, req)

			require.Equal(t, tc.expected.httpCode, rr.Code)
			switch tc.expected.httpCode {
			case http.StatusBadRequest:
				var response DeleteConnection400JSONResponse
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, *tc.expected.message, response.Message)
			case http.StatusOK:
				var response DeleteConnection200JSONResponse
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, *tc.expected.message, response.Message)
			}
		})
	}
}

func TestServer_DeleteConnectionCredentials(t *testing.T) {
	server := newTestServer(t, nil)
	handler := getHandler(context.Background(), server)

	fixture := tests.NewFixture(storage)

	issuerDID, err := w3c.ParseDID("did:iden3:polygon:mumbai:wyFiV4w71QgWPn6bYLsZoysFay66gKtVa9kfu6yMZ")
	require.NoError(t, err)
	userDID, err := w3c.ParseDID("did:polygonid:polygon:mumbai:2qH7XAwYQzCp9VfhpNgeLtK2iCehDDrfMWUCEg5ig5")
	require.NoError(t, err)

	conn := fixture.CreateConnection(t, &domain.Connection{
		ID:         uuid.New(),
		IssuerDID:  *issuerDID,
		UserDID:    *userDID,
		IssuerDoc:  nil,
		UserDoc:    nil,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	})

	_ = fixture.CreateClaim(t, &domain.Claim{
		Identifier:      common.ToPointer(issuerDID.String()),
		Issuer:          issuerDID.String(),
		OtherIdentifier: userDID.String(),
		HIndex:          "20060639968773997271173557722944342103398298534714534718204282267207714246563",
	})

	_ = fixture.CreateClaim(t, &domain.Claim{
		Identifier:      common.ToPointer(issuerDID.String()),
		Issuer:          issuerDID.String(),
		OtherIdentifier: userDID.String(),
		HIndex:          "20060639968773997271173557722944342103398298534714534718204282267207714246562",
	})

	type expected struct {
		httpCode int
		message  *string
	}

	type testConfig struct {
		name     string
		connID   uuid.UUID
		auth     func() (string, string)
		expected expected
	}

	for _, tc := range []testConfig{
		{
			name: "No auth header",
			auth: authWrong,
			expected: expected{
				httpCode: http.StatusUnauthorized,
			},
		},

		{
			name:   "should delete the connection",
			connID: conn,
			auth:   authOk,
			expected: expected{
				httpCode: http.StatusOK,
				message:  common.ToPointer("Credentials of the connection successfully deleted"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			url := fmt.Sprintf("/v1/%s/connections/%s/credentials", issuerDID, tc.connID.String())
			req, err := http.NewRequest("DELETE", url, nil)
			req.SetBasicAuth(tc.auth())
			require.NoError(t, err)

			handler.ServeHTTP(rr, req)

			require.Equal(t, tc.expected.httpCode, rr.Code)
			switch tc.expected.httpCode {
			case http.StatusOK:
				var response DeleteConnectionCredentials200JSONResponse
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, *tc.expected.message, response.Message)
			}
		})
	}
}

func TestServer_RevokeConnectionCredentials(t *testing.T) {
	const (
		method     = "polygonid"
		blockchain = "polygon"
		network    = "amoy"
		BJJ        = "BJJ"
	)
	ctx := context.Background()
	server := newTestServer(t, nil)
	handler := getHandler(context.Background(), server)

	iden, err := server.Services.identity.Create(ctx, "polygon-test", &ports.DIDCreationOptions{Method: method, Blockchain: blockchain, Network: network, KeyType: BJJ})
	require.NoError(t, err)
	issuerDID, err := w3c.ParseDID(iden.Identifier)
	require.NoError(t, err)

	fixture := tests.NewFixture(storage)

	userDID, err := w3c.ParseDID("did:polygonid:polygon:mumbai:2qH7XAwYQzCp9VfhpNgeLtK2iCehDDrfMWUCEg5ig5")
	require.NoError(t, err)

	conn := fixture.CreateConnection(t, &domain.Connection{
		ID:         uuid.New(),
		IssuerDID:  *issuerDID,
		UserDID:    *userDID,
		IssuerDoc:  nil,
		UserDoc:    nil,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	})

	_ = fixture.CreateClaim(t, &domain.Claim{
		ID:              uuid.New(),
		Identifier:      common.ToPointer(issuerDID.String()),
		Issuer:          issuerDID.String(),
		SchemaHash:      "ca938857241db9451ea329256b9c06e5",
		SchemaURL:       "https://raw.githubusercontent.com/iden3/claim-schema-vocab/main/schemas/json-ld/auth.json-ld",
		SchemaType:      "AuthBJJCredential",
		OtherIdentifier: userDID.String(),
		Expiration:      0,
		Version:         0,
		RevNonce:        0,
		CoreClaim:       domain.CoreClaim{},
		Status:          nil,
	})

	type expected struct {
		httpCode int
		message  *string
	}

	type testConfig struct {
		name     string
		connID   uuid.UUID
		auth     func() (string, string)
		expected expected
	}

	for _, tc := range []testConfig{
		{
			name: "No auth header",
			auth: authWrong,
			expected: expected{
				httpCode: http.StatusUnauthorized,
			},
		},
		{
			name:   "should revoke the connection credentials",
			connID: conn,
			auth:   authOk,
			expected: expected{
				httpCode: http.StatusAccepted,
				message:  common.ToPointer("Credentials revocation request sent"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			url := fmt.Sprintf("/v1/%s/connections/%s/credentials/revoke", issuerDID, tc.connID.String())
			req, err := http.NewRequest("POST", url, nil)
			req.SetBasicAuth(tc.auth())
			require.NoError(t, err)

			handler.ServeHTTP(rr, req)

			require.Equal(t, tc.expected.httpCode, rr.Code)
			switch tc.expected.httpCode {
			case http.StatusAccepted:
				var response RevokeConnectionCredentials202JSONResponse
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, *tc.expected.message, response.Message)
			}
		})
	}
}
