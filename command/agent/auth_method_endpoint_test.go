package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/stretchr/testify/assert"
)

func TestHTTP_AuthMethodList(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		p1 := mock.AuthMethod()
		p2 := mock.AuthMethod()
		p3 := mock.AuthMethod()
		args := structs.AuthMethodUpsertRequest{
			AuthMethods: []*structs.AuthMethod{p1, p2, p3},
			WriteRequest: structs.WriteRequest{
				Region:    "global",
				AuthToken: s.RootToken.SecretID,
			},
		}
		var resp structs.GenericResponse
		if err := s.Agent.RPC("AuthMethod.UpsertAuthMethods", &args, &resp); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("GET", "/v1/auth_methods", nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		// Make the request
		obj, err := s.Server.AuthMethodsRequest(respW, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Check for the index
		if respW.Result().Header.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}
		if respW.Result().Header.Get("X-Nomad-KnownLeader") != "true" {
			t.Fatalf("missing known leader")
		}
		if respW.Result().Header.Get("X-Nomad-LastContact") == "" {
			t.Fatalf("missing last contact")
		}

		// Check the output
		n := obj.([]*structs.AuthMethodListStub)
		if len(n) != 3 {
			t.Fatalf("bad: %#v", n)
		}
	})
}

func TestHTTP_AuthMethodQuery(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		p1 := mock.AuthMethod()
		args := structs.AuthMethodUpsertRequest{
			AuthMethods: []*structs.AuthMethod{p1},
			WriteRequest: structs.WriteRequest{
				Region:    "global",
				AuthToken: s.RootToken.SecretID,
			},
		}
		var resp structs.GenericResponse
		if err := s.Agent.RPC("AuthMethod.UpsertAuthMethods", &args, &resp); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("GET", "/v1/auth_method/"+p1.Name, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		// Make the request
		obj, err := s.Server.AuthMethodSpecificRequest(respW, req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		// Check for the index
		if respW.Result().Header.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}
		if respW.Result().Header.Get("X-Nomad-KnownLeader") != "true" {
			t.Fatalf("missing known leader")
		}
		if respW.Result().Header.Get("X-Nomad-LastContact") == "" {
			t.Fatalf("missing last contact")
		}

		// Check the output
		n := obj.(*structs.AuthMethod)
		if n.Name != p1.Name {
			t.Fatalf("bad: %#v", n)
		}
	})
}

func TestHTTP_AuthMethodCreate(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		// Make the HTTP request
		p1 := mock.AuthMethod()
		buf := encodeReq(p1)
		req, err := http.NewRequest("PUT", "/v1/auth_method/"+p1.Name, buf)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		// Make the request
		obj, err := s.Server.AuthMethodSpecificRequest(respW, req)
		assert.Nil(t, err)
		assert.Nil(t, obj)

		// Check for the index
		if respW.Result().Header.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}

		// Check auth method was created
		state := s.Agent.server.State()
		out, err := state.AuthMethodByName(nil, p1.Name)
		assert.Nil(t, err)
		assert.NotNil(t, out)

		p1.CreateIndex, p1.ModifyIndex = out.CreateIndex, out.ModifyIndex
		assert.Equal(t, p1.Name, out.Name)
		assert.Equal(t, p1, out)
	})
}

func TestHTTP_AuthMethodDelete(t *testing.T) {
	t.Parallel()
	httpACLTest(t, nil, func(s *TestAgent) {
		p1 := mock.AuthMethod()
		args := structs.AuthMethodUpsertRequest{
			AuthMethods: []*structs.AuthMethod{p1},
			WriteRequest: structs.WriteRequest{
				Region:    "global",
				AuthToken: s.RootToken.SecretID,
			},
		}
		var resp structs.GenericResponse
		// TODO: THESE RPC CALLS ARE WRONG
		if err := s.Agent.RPC("AuthMethod.UpsertAuthMethods", &args, &resp); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Make the HTTP request
		req, err := http.NewRequest("DELETE", "/v1/auth_method/"+p1.Name, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		respW := httptest.NewRecorder()
		setToken(req, s.RootToken)

		// Make the request
		obj, err := s.Server.AuthMethodSpecificRequest(respW, req)
		assert.Nil(t, err)
		assert.Nil(t, obj)

		// Check for the index
		if respW.Result().Header.Get("X-Nomad-Index") == "" {
			t.Fatalf("missing index")
		}

		// Check auth method was created
		state := s.Agent.server.State()
		out, err := state.AuthMethodByName(nil, p1.Name)
		assert.Nil(t, err)
		assert.Nil(t, out)
	})
}
