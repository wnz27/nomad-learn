package agent

import (
	"net/http"
	"strings"

	"github.com/hashicorp/nomad/nomad/structs"
)

func (s *HTTPServer) AuthMethodsRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	if req.Method != "GET" {
		return nil, CodedError(405, ErrInvalidMethod)
	}

	args := structs.AuthMethodListRequest{}
	if s.parse(resp, req, &args.Region, &args.QueryOptions) {
		return nil, nil
	}

	var out structs.AuthMethodListResponse
	if err := s.agent.RPC("AuthMethod.ListAuthMethods", &args, &out); err != nil {
		return nil, err
	}

	setMeta(resp, &out.QueryMeta)
	if out.AuthMethods == nil {
		out.AuthMethods = make([]*structs.AuthMethodListStub, 0)
	}
	return out.AuthMethods, nil
}

func (s *HTTPServer) AuthMethodSpecificRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	name := strings.TrimPrefix(req.URL.Path, "/v1/auth_method/")
	if len(name) == 0 {
		return nil, CodedError(400, "Missing Auth Method Name")
	}
	switch req.Method {
	case "GET":
		return s.authMethodQuery(resp, req, name)
	case "PUT", "POST":
		return s.authMethodUpdate(resp, req, name)
	case "DELETE":
		return s.authMethodDelete(resp, req, name)
	default:
		return nil, CodedError(405, ErrInvalidMethod)
	}
}

func (s *HTTPServer) authMethodQuery(resp http.ResponseWriter, req *http.Request,
	authMethodName string) (interface{}, error) {
	args := structs.AuthMethodSpecificRequest{
		Name: authMethodName,
	}
	if s.parse(resp, req, &args.Region, &args.QueryOptions) {
		return nil, nil
	}

	var out structs.SingleAuthMethodResponse
	if err := s.agent.RPC("AuthMethod.GetAuthMethod", &args, &out); err != nil {
		return nil, err
	}

	setMeta(resp, &out.QueryMeta)
	if out.AuthMethod == nil {
		return nil, CodedError(404, "Auth method not found")
	}
	return out.AuthMethod, nil
}

func (s *HTTPServer) authMethodUpdate(resp http.ResponseWriter, req *http.Request,
	authMethodName string) (interface{}, error) {
	// Parse the policy
	var authMethod structs.AuthMethod
	if err := decodeBody(req, &authMethod); err != nil {
		return nil, CodedError(500, err.Error())
	}

	// Ensure the method name matches
	if authMethod.Name != authMethodName {
		return nil, CodedError(400, "Auth method name does not match request path")
	}

	// Format the request
	args := structs.AuthMethodUpsertRequest{
		AuthMethods: []*structs.AuthMethod{&authMethod},
	}
	s.parseWriteRequest(req, &args.WriteRequest)

	var out structs.GenericResponse
	if err := s.agent.RPC("AuthMethod.UpsertAuthMethods", &args, &out); err != nil {
		return nil, err
	}
	setIndex(resp, out.Index)
	return nil, nil
}

func (s *HTTPServer) authMethodDelete(resp http.ResponseWriter, req *http.Request,
	authMethodName string) (interface{}, error) {

	args := structs.AuthMethodDeleteRequest{
		Names: []string{authMethodName},
	}
	s.parseWriteRequest(req, &args.WriteRequest)

	var out structs.GenericResponse
	if err := s.agent.RPC("AuthMethod.DeleteAuthMethods", &args, &out); err != nil {
		return nil, err
	}
	setIndex(resp, out.Index)
	return nil, nil
}
