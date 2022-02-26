package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/cap/oidc"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/stretchr/testify/assert"
)

func TestHTTP_OIDCAuthURLRequest(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		// Adds an auth method to the db with an upsert call
		am := setUpAuthMethod(t, s)

		request := structs.OIDCAuthURLRequest{
			Auth: &structs.OIDCAuthURLParams{
				AuthMethod:  am.Name,
				RedirectURI: "https://nomad.nomad/v1/oidc/callback",
				ClientNonce: "nonce",
				Meta:        map[string]string{"foo": "bar"},
			},
		}
		buf := encodeReq(request)

		req, err := http.NewRequest("POST", "/v1/oidc/auth-url/"+am.Name, buf)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		obj, err := s.Server.OIDCAuthURLRequest(respW, req)
		assert.Nil(t, err)

		// Check for the index
		// if respW.Result().Header.Get("X-Nomad-Index") == "" {
		// 	t.Fatalf("missing index")
		// }

		// Ensure that the correct url was returned
		urlResp := obj.(structs.OIDCAuthURLResponse)

		// TODO: MAKE THIS OUT OF ITS COMPONENT PARTS
		expectedURL := "https://127.0.0.1:55330/authorize?client_id=client-id&nonce=nonce&redirect_uri=https%3A%2F%2Fexample.com&response_type=code&scope=policyA&state=st_q0jIoyduiIMHpIuPUzdC"

		assert.Equal(
			t,
			urlResp.URL,
			expectedURL,
		)
	})
}

func TestHTTP_OIDCCallbackRequest(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		// Adds an auth method to the db with an upsert call
		am := setUpAuthMethod(t, s)

		request := structs.OIDCCallbackParams{
			AuthMethod:  am.Name,
			RedirectURI: "https://nomad.nomad/v1/oidc/callback",
			Code:        "code",
			State:       "state",
			ClientNonce: "nonce",
		}
		buf := encodeReq(request)
		req, err := http.NewRequest("POST", "/v1/oidc/callback/", buf)

		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		// TODO: MAKE THE ACTUAL REQUEST THAT YOU NEED
		obj, err := s.Server.OIDCCallbackRequest(respW, req)
		assert.Nil(t, err)
		assert.Nil(t, obj)

		// Check for the index
		if respW.Result().Header.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}

		// TODO: ASSERT SOMETHING ABOUT THE REPONSE
	})
}

// func TestHTTP_OIDCIntegrationTest(t *testing.T) {
// 	t.Parallel()
// 	httpACLTest(t, nil, func(s *TestAgent) {

// 		// ==============================
// 		// Create our OIDC test provider
// 		// ==============================

// 		oidcTP := oidc.StartTestProvider(t)
// 		oidcTP.SetClientCreds("client-id", "big-secret")
// 		// p.privKey, p.pubKey, p.alg, p.keyID
// 		// _, _, _, _ := oidcTP.SigningKeys()
// 		oidcTP.SigningKeys()

// 		// ======================================
// 		// Create an auth method for the provider
// 		// ======================================

// 		// Adds an auth method to the db with an upsert call
// 		am := setUpAuthMethod(t, s)

// 		// ==============================================================
// 		// Get the auth URL for the new method
// 		// (this will reach out to the test provider behind the scenes)
// 		// ==============================================================

// 		// TODO: PASS IN THE PROPER PARAMS
// 		// urlReq := mock.OIDCAuthUrl()
// 		// buf := encodeReq(urlReq)

// 		req, err := http.NewRequest("POST", "/v1/oidc/auth-url/"+am.Name, nil)
// 		if err != nil {
// 			t.Fatalf("err: %v", err)
// 		}
// 		respW := httptest.NewRecorder()
// 		setToken(req, s.RootToken)

// 		obj, err := s.Server.OIDCAuthURLRequest(respW, req)
// 		assert.Nil(t, err)
// 		assert.Nil(t, obj)

// 		// ==================
// 		// Make the callback
// 		// ==================

// 		req2, err := http.NewRequest("GET", "/v1/oidc/callback/"+am.Name, nil)
// 		if err != nil {
// 			t.Fatalf("err: %v", err)
// 		}
// 		respW2 := httptest.NewRecorder()
// 		setToken(req, s.RootToken)

// 		// TODO: MAKE THE ACTUAL REQUEST THAT YOU NEED
// 		obj2, err := s.Server.AuthMethodSpecificRequest(respW2, req2)
// 		assert.Nil(t, err)
// 		assert.Nil(t, obj2)

// 		// Check for the index
// 		if respW.Result().Header.Get("X-Nomad-Index") == "" {
// 			t.Fatalf("missing index")
// 		}

// 		// ============================================================
// 		// Assert that the token returned has the expected permissions
// 		// ============================================================

// 		// TODO: ASSERT PROPER PERMISSIONS
// 	})
// }

func setUpAuthMethod(t *testing.T, s *TestAgent) *structs.AuthMethod {
	// Create our OIDC test provider
	oidcTP := oidc.StartTestProvider(t)
	oidcTP.SetClientCreds("client-id", "big-secret")
	_, _, tpAlg, _ := oidcTP.SigningKeys()

	allowedRedirects := []string{
		"https://nomad.nomad/v1/oidc/callback",
		"https://nomad.nomad/ui/oidc/callback",
	}

	oidcTP.SetExpectedState("state")
	oidcTP.SetExpectedAuthCode("code")
	oidcTP.SetExpectedAuthNonce("nonce")
	oidcTP.SetCustomAudience([]string{}...)
	oidcTP.SetAllowedRedirectURIs(allowedRedirects)

	claims := map[string]interface{}{
		"policies": []string{"policyA", "policyB"},
		"admin":    false,
	}

	oidcTP.SetCustomClaims(claims)

	config := &structs.AuthMethodConfig{
		OIDCDiscoveryURL:    oidcTP.Addr(),
		OIDCClientID:        "client-id",
		OIDCClientSecret:    "big-secret",
		BoundAudiences:      []string{},
		AllowedRedirectURIs: allowedRedirects,
		ClaimMappings:       map[string]string{},
		ListClaimMappings:   map[string]string{},
		DiscoveryCaPem:      []string{oidcTP.CACert()},
		SigningAlgs:         []string{string(tpAlg)},
	}

	am := &structs.AuthMethod{
		Name:        fmt.Sprintf("auth-method-%s", uuid.Generate()),
		Type:        "oidc",
		MaxTokenTTL: "5m",
		Config:      *config,
		CreateTime:  time.Now().UTC(),
		CreateIndex: 10,
		ModifyIndex: 20,
	}
	am.SetHash()

	// Make the HTTP request to add the method

	args := structs.AuthMethodUpsertRequest{
		AuthMethods: []*structs.AuthMethod{am},
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: s.RootToken.SecretID,
		},
	}
	var resp structs.GenericResponse
	if err := s.Agent.RPC("AuthMethod.UpsertAuthMethods", &args, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	return am
}
