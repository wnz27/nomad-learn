package api

import (
	"fmt"
)

// OIDC is used to query the OIDC endpoints.
type OIDC struct {
	client *Client
}

// OIDC returns a new handle on the OIDC.
func (c *Client) OIDC() *OIDC {
	return &OIDC{client: c}
}

func (a *OIDC) GetAuthUrl(args *AuthUrlArgs, q *WriteOptions) (*AuthUrlStub, *WriteMeta, error) {
	if args == nil || args.AuthMethod == "" {
		return nil, nil, fmt.Errorf("missing method name")
	}

	if args == nil || args.RedirectUri == "" {
		return nil, nil, fmt.Errorf("missing redirect uri")
	}

	if args == nil || args.ClientNonce == "" {
		return nil, nil, fmt.Errorf("missing nonce")
	}

	var resp AuthUrlStub
	wm, err := a.client.write("/v1/oidc/auth-url", args, &resp, q)
	if err != nil {
		return nil, nil, err
	}
	return &resp, wm, nil
}

func (a *OIDC) Callback(args *CallbackArgs, q *WriteOptions) (*string, *WriteMeta, error) {
	if args == nil || args.AuthMethod == "" {
		return nil, nil, fmt.Errorf("missing method name")
	}

	if args == nil || args.RedirectUri == "" {
		return nil, nil, fmt.Errorf("missing redirect uri")
	}

	if args == nil || args.ClientNonce == "" {
		return nil, nil, fmt.Errorf("missing nonce")
	}

	var resp string
	wm, err := a.client.write("/v1/oidc/callback", args, &resp, q)
	if err != nil {
		return nil, nil, err
	}
	return &resp, wm, nil
}

type AuthUrlStub struct {
	URL string
}

type AuthUrlArgs struct {
	AuthMethod  string
	RedirectUri string
	ClientNonce string
}

type CallbackArgs struct {
	AuthMethod  string
	RedirectUri string
	ClientNonce string
	Code        string
	State       string
}
