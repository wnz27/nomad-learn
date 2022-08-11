package nomad

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/hashicorp/cap/oidc"
	log "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// oidcAuthExpiry is the duration that an OIDC-based login is valid for.
	// We default this to 30 days for now but that is arbitrary. We can change
	// this default anytime or choose to make it configurable one day on the
	// server.
	oidcAuthExpiry = 30 * 24 * time.Hour

	// oidcReqExpiry is the time that an OIDC auth request is valid for.
	// 5 minutes should be plenty of time to complete auth.
	oidcReqExpiry = 5 * 60 * time.Minute
)

// ACL endpoint is used for manipulating ACL tokens and auth methods
type OIDC struct {
	srv    *Server
	logger log.Logger
	ctx    *RPCContext
}

// AuthURLRequest is used to... TODO
func (o *OIDC) AuthURLRequest(args *structs.OIDCAuthURLRequest, reply *structs.OIDCAuthURLResponse) error {
	args.Region = o.srv.config.AuthoritativeRegion

	if done, err := o.srv.forward("OIDC.AuthURLRequest", args, args, reply); done {
		return err
	}

	defer metrics.MeasureSince([]string{"nomad", "oidc", "auth_url_request"}, time.Now())

	// ===================
	//   GET AUTH METHOD
	// ===================

	// Validate that an auth method was passed in
	if args.Auth.AuthMethod == "" {
		return structs.NewErrRPCCoded(400, "must specify an auth method")
	}

	am, err := o.getAuthMethodByName(args.Auth.AuthMethod)
	if err != nil {
		return err
	}

	// =====================
	//   GET OIDC PROVIDER
	// =====================

	provider, err := o.getProviderForAuthMethod(am)
	if err != nil {
		return err
	}

	// ===================
	//   BUILD A REQUEST
	// ===================

	// Create a minimal request to get the auth URL
	oidcReqOpts := []oidc.Option{
		oidc.WithNonce(args.Auth.ClientNonce),
	}

	// TODO: ADD IN SCOPES TO THE REQUEST?
	// if v := am.Oidc.Scopes; len(v) > 0 {
	// 	oidcReqOpts = append(oidcReqOpts, oidc.WithScopes(v...))
	// }

	// =============================
	//   REQUEST URL FROM PROVIDER
	// =============================

	// TODO: ASK WHAT THIS IS DOING
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	oidcReq, err := oidc.NewRequest(
		oidcReqExpiry,
		args.Auth.RedirectURI,
		oidcReqOpts...,
	)
	if err != nil {
		return err
	}

	// Get the auth URL
	url, err := provider.AuthURL(ctx, oidcReq)
	if err != nil {
		return err
	}

	// =======================
	//   RETURN THE URL INFO
	// =======================

	reply.URL = url

	return nil
}

// =======================
// =======================
// =======================
//   TODO: REMOVE ME: THIS IS TO CATCH THE EYE WHEN SCROLLING
// =======================
// =======================
// =======================

// AuthCallback is used to... TODO
func (o *OIDC) AuthCallback(args *structs.OIDCCallbackRequest, reply *structs.OIDCCallbackResponse) error {
	args.Region = o.srv.config.AuthoritativeRegion

	if done, err := o.srv.forward("OIDC.AuthCallbackRequest", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "oidc", "auth_callback_request"}, time.Now())

	// ===================
	//   GET AUTH METHOD
	// ===================

	// Validate that an auth method was passed in
	if args.Auth.AuthMethod == "" {
		return structs.NewErrRPCCoded(400, "must specify an auth method")
	}

	am, err := o.getAuthMethodByName(args.Auth.AuthMethod)
	if err != nil {
		return err
	}

	// =====================
	//   GET OIDC PROVIDER
	// =====================

	provider, err := o.getProviderForAuthMethod(am)
	if err != nil {
		return err
	}

	// ====================================
	//   EXCHANGE CODE FOR PROVIDER TOKEN
	// ====================================

	// Create a minimal request to get the auth URL
	oidcReqOpts := []oidc.Option{
		oidc.WithNonce(args.Auth.ClientNonce),
		oidc.WithState(args.Auth.State),
	}

	if auds := am.Config.BoundAudiences; len(auds) > 0 {
		oidcReqOpts = append(oidcReqOpts, oidc.WithAudiences(auds...))
	}

	// TODO: SHOULD HAVE SCOPES (see above)?

	oidcReq, err := oidc.NewRequest(
		oidcReqExpiry,
		args.Auth.RedirectURI,
		oidcReqOpts...,
	)

	if err != nil {
		return err
	}

	// TODO: ASK WHAT THIS IS DOING
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	// Exchange our code for our token
	oidcToken, err := provider.Exchange(ctx, oidcReq, args.Auth.State, args.Auth.Code)
	if err != nil {
		return err
	}

	// ======================================
	//   EXTRACT CLAIMS FROM PROVIDER TOKEN
	// ======================================

	// Extract the claims as a raw JSON message.
	var jsonClaims json.RawMessage
	if err := oidcToken.IDToken().Claims(&jsonClaims); err != nil {
		return err
	}

	// Structurally extract only the claim fields we care about.
	var idClaimVals idClaims
	if err := json.Unmarshal([]byte(jsonClaims), &idClaimVals); err != nil {
		return err
	}

	// Valid OIDC providers should never behave this way.
	if idClaimVals.Iss == "" || idClaimVals.Sub == "" {
		return status.Errorf(codes.Internal, "OIDC provider returned empty issuer or subscriber ID")
	}

	// Get the user info if we have a user account, and merge those claims in.
	// User claims override all the claims in the ID token.
	var userClaims json.RawMessage
	if userTokenSource := oidcToken.StaticTokenSource(); userTokenSource != nil {
		if err := provider.UserInfo(ctx, userTokenSource, idClaimVals.Sub, &userClaims); err != nil {
			return err
		}
	}

	// =====================================
	//   LOOKUP POLICY/POLICIES FOR CLAIMS
	// =====================================

	// fmt.Println("============")

	// fmt.Println("jsonClaims pretty")
	// j, _ := json.Marshal(&jsonClaims)
	// fmt.Println(string(j))

	// fmt.Println("idClaimVals")
	// fmt.Println(idClaimVals)

	// fmt.Println("userClaims")
	// fmt.Println(string(userClaims))

	// fmt.Println("============")

	// fmt.Println("idClaimVals.Policies")
	// fmt.Println(idClaimVals.Policies)

	// ===================================
	//   GENERATE ACL TOKEN FOR POLICIES
	// ===================================

	// Create a new global management token, override any parameter
	// TODO: Nicer name
	stateSnapshot, err := o.srv.State().Snapshot()
	if err != nil {
		return err
	}

	aclRole, err := stateSnapshot.GetACLRoleByName(nil, idClaimVals.Role)
	if err != nil {
		return err
	}

	roles := []*structs.ACLTokenRoleLink{{Name: idClaimVals.Role, ID: aclRole.ID}}

	token := &structs.ACLToken{
		AccessorID: uuid.Generate(),
		SecretID:   uuid.Generate(),
		Name:       "OIDC Token",
		Type:       structs.ACLClientToken,
		Global:     true,
		// Policies:   idClaimVals.Policies,
		Roles:      roles,
		CreateTime: time.Now().UTC(),
	}
	token.SetHash()

	var tokens []*structs.ACLToken
	tokens = append(tokens, token)

	upsertArgs := &structs.ACLTokenUpsertRequest{
		Tokens: tokens,
	}

	_, _, err = o.srv.raftApply(structs.ACLTokenUpsertRequestType, upsertArgs)
	if err != nil {
		return err
	}

	// ================================
	//   RETURN ACL TOKEN IN RESPONSE
	// ================================

	// TODO: SHOULD PROBABLY ALLOW FOR TOKEN EXPIRY
	// FOR THIS TO WORK PROPERLY
	// See https://github.com/hashicorp/consul/pull/5353

	reply.Token = token.SecretID

	return nil
}

func (o *OIDC) getAuthMethodByName(amName string) (*structs.AuthMethod, error) {
	args := structs.AuthMethodSpecificRequest{
		Name: amName,
	}
	args.Region = o.srv.config.AuthoritativeRegion

	reply := structs.SingleAuthMethodResponse{}

	a := AuthMethod{
		srv:    o.srv,
		logger: o.logger,
	}

	if err := a.GetAuthMethod(&args, &reply); err != nil {
		return nil, err
	}

	return reply.AuthMethod, nil
}

func (o *OIDC) getProviderForAuthMethod(am *structs.AuthMethod) (*oidc.Provider, error) {
	// TODO: THIS SHOULD BE CACHED
	// LOOK AT ProviderCache in Waypoint

	var algs []oidc.Alg
	if len(am.Config.SigningAlgs) > 0 {
		for _, alg := range am.Config.SigningAlgs {
			algs = append(algs, oidc.Alg(alg))
		}
	} else {
		algs = []oidc.Alg{oidc.RS256}
	}

	oidcCfg, err := oidc.NewConfig(
		am.Config.OIDCDiscoveryURL,
		am.Config.OIDCClientID,
		oidc.ClientSecret(am.Config.OIDCClientSecret),
		algs,
		am.Config.AllowedRedirectURIs,
		oidc.WithAudiences(am.Config.BoundAudiences...),
		oidc.WithProviderCA(strings.Join(am.Config.DiscoveryCaPem, "\n")),
	)
	if err != nil {
		return nil, err
	}

	// If we made it here, the provider isn't in the cache OR the config changed.
	// Initialize a new provider.
	return oidc.NewProvider(oidcCfg)
}

// idClaims are the claims for the ID token that we care about. There
// are many more claims[1] but we only add what we need.
//
// [1]: https://openid.net/specs/openid-connect-core-1_0.html#StandardClaims
type idClaims struct {
	Iss           string   `json:"iss"`
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	Policies      []string `json:"http://nomad.internal/policies"`
	Role          string   `json:"http://nomad.internal/role"`
	EmailVerified bool     `json:"email_verified"`
}
