package api

import (
	"fmt"
	"time"
)

// AuthMethods is used to query the Auth Methods endpoints.
type AuthMethods struct {
	client *Client
}

// AuthMethods returns a new handle on the Auth Methods.
func (c *Client) AuthMethods() *AuthMethods {
	return &AuthMethods{client: c}
}

// List is used to dump all of the policies.
func (a *AuthMethods) List(q *QueryOptions) ([]*AuthMethodListStub, *QueryMeta, error) {
	var resp []*AuthMethodListStub
	qm, err := a.client.query("/v1/auth_methods", &resp, q)
	if err != nil {
		return nil, nil, err
	}
	return resp, qm, nil
}

// Upsert is used to create or update a method
func (a *AuthMethods) Upsert(method *AuthMethod, q *WriteOptions) (*WriteMeta, error) {
	if method == nil || method.Name == "" {
		return nil, fmt.Errorf("missing method name")
	}
	wm, err := a.client.write("/v1/auth_methods/"+method.Name, method, nil, q)
	if err != nil {
		return nil, err
	}
	return wm, nil
}

// Delete is used to delete a method
func (a *AuthMethods) Delete(methodName string, q *WriteOptions) (*WriteMeta, error) {
	if methodName == "" {
		return nil, fmt.Errorf("missing method name")
	}
	wm, err := a.client.delete("/v1/auth_methods/"+methodName, nil, nil, q)
	if err != nil {
		return nil, err
	}
	return wm, nil
}

// Info is used to query a specific method
func (a *AuthMethods) Info(methodName string, q *QueryOptions) (*AuthMethod, *QueryMeta, error) {
	if methodName == "" {
		return nil, nil, fmt.Errorf("missing method name")
	}
	var resp AuthMethod
	wm, err := a.client.query("/v1/auth_methods/"+methodName, &resp, q)
	if err != nil {
		return nil, nil, err
	}
	return &resp, wm, nil
}

type AuthMethodListStub struct {
	Name        string
	Description string
	CreateIndex uint64
	ModifyIndex uint64
	Hash        []byte
}

type AuthMethodConfig struct {
	OIDCDiscoveryURL    string
	OIDCClientID        string
	OIDCClientSecret    string
	BoundAudiences      []string
	AllowedRedirectURIs []string
	ClaimMappings       map[string]string
	ListClaimMappings   map[string]string
}

type AuthMethod struct {
	Name        string
	Type        string
	MaxTokenTTL string
	Config      AuthMethodConfig
	CreateTime  time.Time
	CreateIndex uint64
	ModifyIndex uint64
}
