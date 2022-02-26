package agent

import (
	"net/http"
	"strings"

	"github.com/hashicorp/nomad/nomad/structs"
)

func (s *HTTPServer) OIDCAuthURLRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	if req.Method != "POST" && req.Method != "PUT" {
		return nil, CodedError(405, ErrInvalidMethod)
	}

	// Endpoint accepts data in the expected format

	// Get ID from the URL
	authMethodName := strings.TrimPrefix(req.URL.Path, "/v1/oidc/auth-url/")

	if authMethodName == "" {
		return nil, CodedError(400, "Missing Auth Method Name")
	}

	// Parse the policy
	var args structs.OIDCAuthURLRequest
	if err := decodeBody(req, &args.Auth); err != nil {
		return nil, CodedError(500, err.Error())
	}

	var out structs.OIDCAuthURLResponse
	if err := s.agent.RPC("OIDC.AuthURLRequest", &args, &out); err != nil {
		// TODO: Make sure it errors if no auth method is found
		return nil, err
	}

	// TODO: I DONT THINK WE WANT THIS
	// setMeta(resp, &out.QueryMeta)

	return out, nil
}

func (s *HTTPServer) OIDCCallbackRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	if req.Method != "POST" && req.Method != "PUT" {
		return nil, CodedError(405, ErrInvalidMethod)
	}

	// Gets the requested auth method from the store

	var cbParams structs.OIDCCallbackParams
	args := structs.OIDCCallbackRequest{
		Datacenter: s.agent.config.Datacenter,
		Auth:       &cbParams,
	}

	if err := decodeBody(req, &args.Auth); err != nil {
		return nil, CodedError(500, err.Error())
	}

	var out structs.OIDCCallbackResponse
	if err := s.agent.RPC("OIDC.AuthCallback", &args, &out); err != nil {
		return nil, err
	}

	// Errors if there is no matching auth method
	if out.Token == "" {
		return nil, CodedError(404, "Token not found")
	}

	return out.Token, nil
}
