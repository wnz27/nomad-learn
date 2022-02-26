package nomad

import (
	"strings"
	"testing"
	"time"

	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// https://github.com/hashicorp/nomad/blob/main/contributing/checklist-rpc-endpoint.md

func TestACLEndpoint_GetAuthMethod(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	authMethod := mock.AuthMethod()
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1000, []*structs.AuthMethod{authMethod})

	anonymousAuthMethod := mock.AuthMethod()
	anonymousAuthMethod.Name = "anonymous"
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1001, []*structs.AuthMethod{anonymousAuthMethod})

	// Create a token with one the authMethod
	token := mock.ACLToken()
	s1.fsm.State().UpsertACLTokens(structs.MsgTypeTestSetup, 1002, []*structs.ACLToken{token})

	// Lookup the authMethod
	get := &structs.AuthMethodSpecificRequest{
		Name: authMethod.Name,
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.SingleAuthMethodResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, uint64(1000), resp.Index)
	assert.Equal(t, authMethod, resp.AuthMethod)

	// Lookup non-existing authMethod
	get.Name = uuid.Generate()
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, uint64(1001), resp.Index)
	assert.Nil(t, resp.AuthMethod)

	// Lookup the authMethod with the token
	get = &structs.AuthMethodSpecificRequest{
		Name: authMethod.Name,
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			AuthToken: token.SecretID,
		},
	}
	var resp2 structs.SingleAuthMethodResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", get, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.EqualValues(t, 1000, resp2.Index)
	assert.Equal(t, authMethod, resp2.AuthMethod)

	// Lookup the anonymous authMethod with no token
	get = &structs.AuthMethodSpecificRequest{
		Name: anonymousAuthMethod.Name,
		QueryOptions: structs.QueryOptions{
			Region: "global",
		},
	}
	var resp3 structs.SingleAuthMethodResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", get, &resp3); err != nil {
		require.NoError(t, err)
	}
	assert.EqualValues(t, 1001, resp3.Index)
	assert.Equal(t, anonymousAuthMethod, resp3.AuthMethod)
}

func TestACLEndpoint_GetAuthMethod_Blocking(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the authMethods
	p1 := mock.AuthMethod()
	p2 := mock.AuthMethod()

	// First create an unrelated authMethod
	time.AfterFunc(100*time.Millisecond, func() {
		err := state.UpsertAuthMethods(structs.MsgTypeTestSetup, 100, []*structs.AuthMethod{p1})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Upsert the authMethod we are watching later
	time.AfterFunc(200*time.Millisecond, func() {
		err := state.UpsertAuthMethods(structs.MsgTypeTestSetup, 200, []*structs.AuthMethod{p2})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Lookup the authMethod
	req := &structs.AuthMethodSpecificRequest{
		Name: p2.Name,
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 150,
			AuthToken:     root.SecretID,
		},
	}
	var resp structs.SingleAuthMethodResponse
	start := time.Now()
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	if resp.Index != 200 {
		t.Fatalf("Bad index: %d %d", resp.Index, 200)
	}
	if resp.AuthMethod == nil || resp.AuthMethod.Name != p2.Name {
		t.Fatalf("bad: %#v", resp.AuthMethod)
	}

	// Eval delete triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		err := state.DeleteAuthMethods(structs.MsgTypeTestSetup, 300, []string{p2.Name})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req.QueryOptions.MinQueryIndex = 250
	var resp2 structs.SingleAuthMethodResponse
	start = time.Now()
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethod", req, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
	if resp2.Index != 300 {
		t.Fatalf("Bad index: %d %d", resp2.Index, 300)
	}
	if resp2.AuthMethod != nil {
		t.Fatalf("bad: %#v", resp2.AuthMethod)
	}
}

func TestACLEndpoint_GetAuthMethods(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	authMethod := mock.AuthMethod()
	authMethod2 := mock.AuthMethod()
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1000, []*structs.AuthMethod{authMethod, authMethod2})

	// Lookup the authMethod
	get := &structs.AuthMethodSetRequest{
		Names: []string{authMethod.Name, authMethod2.Name},
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.AuthMethodSetResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethods", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, uint64(1000), resp.Index)
	assert.Equal(t, 2, len(resp.AuthMethods))
	assert.Equal(t, authMethod, resp.AuthMethods[authMethod.Name])
	assert.Equal(t, authMethod2, resp.AuthMethods[authMethod2.Name])

	// Lookup non-existing authMethod
	get.Names = []string{uuid.Generate()}
	resp = structs.AuthMethodSetResponse{}
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethods", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, uint64(1000), resp.Index)
	assert.Equal(t, 0, len(resp.AuthMethods))
}

func TestACLEndpoint_GetAuthMethods_Blocking(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the authMethods
	p1 := mock.AuthMethod()
	p2 := mock.AuthMethod()

	// First create an unrelated authMethod
	time.AfterFunc(100*time.Millisecond, func() {
		err := state.UpsertAuthMethods(structs.MsgTypeTestSetup, 100, []*structs.AuthMethod{p1})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Upsert the authMethod we are watching later
	time.AfterFunc(200*time.Millisecond, func() {
		err := state.UpsertAuthMethods(structs.MsgTypeTestSetup, 200, []*structs.AuthMethod{p2})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	// Lookup the authMethod
	req := &structs.AuthMethodSetRequest{
		Names: []string{p2.Name},
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 150,
			AuthToken:     root.SecretID,
		},
	}
	var resp structs.AuthMethodSetResponse
	start := time.Now()
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethods", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	if resp.Index != 200 {
		t.Fatalf("Bad index: %d %d", resp.Index, 200)
	}
	if len(resp.AuthMethods) == 0 || resp.AuthMethods[p2.Name] == nil {
		t.Fatalf("bad: %#v", resp.AuthMethods)
	}

	// Eval delete triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		err := state.DeleteAuthMethods(structs.MsgTypeTestSetup, 300, []string{p2.Name})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req.QueryOptions.MinQueryIndex = 250
	var resp2 structs.AuthMethodSetResponse
	start = time.Now()
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.GetAuthMethods", req, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
	if resp2.Index != 300 {
		t.Fatalf("Bad index: %d %d", resp2.Index, 300)
	}
	if len(resp2.AuthMethods) != 0 {
		t.Fatalf("bad: %#v", resp2.AuthMethods)
	}
}

func TestACLEndpoint_ListAuthMethods(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	p1 := mock.AuthMethod()
	p2 := mock.AuthMethod()

	p1.Name = "aaaaaaaa-3350-4b4b-d185-0e1992ed43e9"
	p2.Name = "aaaabbbb-3350-4b4b-d185-0e1992ed43e9"
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1000, []*structs.AuthMethod{p1, p2})

	// Create a token with one of those authMethods
	token := mock.ACLToken()
	s1.fsm.State().UpsertACLTokens(structs.MsgTypeTestSetup, 1001, []*structs.ACLToken{token})

	// Lookup the authMethods
	get := &structs.AuthMethodListRequest{
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.AuthMethodListResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", get, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.EqualValues(1000, resp.Index)
	assert.Len(resp.AuthMethods, 2)

	// Lookup the authMethods by prefix
	get = &structs.AuthMethodListRequest{
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			Prefix:    "aaaabb",
			AuthToken: root.SecretID,
		},
	}
	var resp2 structs.AuthMethodListResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", get, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.EqualValues(1000, resp2.Index)
	assert.Len(resp2.AuthMethods, 1)

	// List authMethods using the created token
	get = &structs.AuthMethodListRequest{
		QueryOptions: structs.QueryOptions{
			Region:    "global",
			AuthToken: token.SecretID,
		},
	}
	var resp3 structs.AuthMethodListResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", get, &resp3); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.EqualValues(1000, resp3.Index)
	if assert.Len(resp3.AuthMethods, 1) {
		assert.Equal(resp3.AuthMethods[0].Name, p1.Name)
	}
}

// TestACLEndpoint_ListAuthMethods_Unauthenticated asserts that
// unauthenticated ListAuthMethods returns anonymous authMethod if one
// exists, otherwise, empty
func TestACLEndpoint_ListAuthMethods_Unauthenticated(t *testing.T) {
	t.Parallel()

	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	listAuthMethods := func() (*structs.AuthMethodListResponse, error) {
		// Lookup the authMethods
		get := &structs.AuthMethodListRequest{
			QueryOptions: structs.QueryOptions{
				Region: "global",
			},
		}

		var resp structs.AuthMethodListResponse
		err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", get, &resp)
		if err != nil {
			return nil, err
		}
		return &resp, nil
	}

	p1 := mock.AuthMethod()
	p1.Name = "aaaaaaaa-3350-4b4b-d185-0e1992ed43e9"
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1000, []*structs.AuthMethod{p1})

	t.Run("no anonymous authMethod", func(t *testing.T) {
		resp, err := listAuthMethods()
		require.NoError(t, err)
		require.Empty(t, resp.AuthMethods)
		require.Equal(t, uint64(1000), resp.Index)
	})

	// now try with anonymous authMethod
	p2 := mock.AuthMethod()
	p2.Name = "anonymous"
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1001, []*structs.AuthMethod{p2})

	t.Run("with anonymous authMethod", func(t *testing.T) {
		resp, err := listAuthMethods()
		require.NoError(t, err)
		require.Len(t, resp.AuthMethods, 1)
		require.Equal(t, "anonymous", resp.AuthMethods[0].Name)
		require.Equal(t, uint64(1001), resp.Index)
	})
}

func TestACLEndpoint_ListAuthMethods_Blocking(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	state := s1.fsm.State()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the authMethod
	authMethod := mock.AuthMethod()

	// Upsert eval triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		if err := state.UpsertAuthMethods(structs.MsgTypeTestSetup, 2, []*structs.AuthMethod{authMethod}); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req := &structs.AuthMethodListRequest{
		QueryOptions: structs.QueryOptions{
			Region:        "global",
			MinQueryIndex: 1,
			AuthToken:     root.SecretID,
		},
	}
	start := time.Now()
	var resp structs.AuthMethodListResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp)
	}
	assert.Equal(t, uint64(2), resp.Index)
	if len(resp.AuthMethods) != 1 || resp.AuthMethods[0].Name != authMethod.Name {
		t.Fatalf("bad: %#v", resp.AuthMethods)
	}

	// Eval deletion triggers watches
	time.AfterFunc(100*time.Millisecond, func() {
		if err := state.DeleteAuthMethods(structs.MsgTypeTestSetup, 3, []string{authMethod.Name}); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	req.MinQueryIndex = 2
	start = time.Now()
	var resp2 structs.AuthMethodListResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.ListAuthMethods", req, &resp2); err != nil {
		t.Fatalf("err: %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("should block (returned in %s) %#v", elapsed, resp2)
	}
	assert.Equal(t, uint64(3), resp2.Index)
	assert.Equal(t, 0, len(resp2.AuthMethods))
}

func TestACLEndpoint_DeleteAuthMethods(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	p1 := mock.AuthMethod()
	s1.fsm.State().UpsertAuthMethods(structs.MsgTypeTestSetup, 1000, []*structs.AuthMethod{p1})

	// Lookup the authMethods
	req := &structs.AuthMethodDeleteRequest{
		Names: []string{p1.Name},
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.GenericResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.DeleteAuthMethods", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.NotEqual(t, uint64(0), resp.Index)
}

func TestACLEndpoint_UpsertAuthMethods(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	p1 := mock.AuthMethod()

	// Lookup the authMethods
	req := &structs.AuthMethodUpsertRequest{
		AuthMethods: []*structs.AuthMethod{p1},
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.GenericResponse
	if err := msgpackrpc.CallWithCodec(codec, "AuthMethod.UpsertAuthMethods", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	assert.NotEqual(t, uint64(0), resp.Index)

	// Check we created the authMethod
	out, err := s1.fsm.State().AuthMethodByName(nil, p1.Name)
	assert.Nil(t, err)
	assert.NotNil(t, out)
}

func TestACLEndpoint_UpsertAuthMethods_Invalid(t *testing.T) {
	t.Parallel()

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create the register request
	am1 := mock.AuthMethod()

	// Lookup the authMethods
	req := &structs.AuthMethodUpsertRequest{
		AuthMethods: []*structs.AuthMethod{am1},
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}
	var resp structs.GenericResponse
	err := msgpackrpc.CallWithCodec(codec, "AuthMethod.UpsertAuthMethods", req, &resp)
	assert.NotNil(t, err)
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("bad: %s", err)
	}
}
