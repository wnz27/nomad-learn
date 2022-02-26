package nomad

import (
	"time"

	metrics "github.com/armon/go-metrics"
	log "github.com/hashicorp/go-hclog"
	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

// ACL endpoint is used for manipulating ACL tokens and auth methods
type AuthMethod struct {
	srv    *Server
	logger log.Logger
}

// UpsertAuthMethods is used to create or update a set of auth methods
func (a *AuthMethod) UpsertAuthMethods(args *structs.AuthMethodUpsertRequest, reply *structs.GenericResponse) error {
	args.Region = a.srv.config.AuthoritativeRegion

	if done, err := a.srv.forward("AuthMethod.UpsertAuthMethods", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "acl", "upsert_auth_methods"}, time.Now())

	// Check management level permissions
	if acl, err := a.srv.ResolveToken(args.AuthToken); err != nil {
		return err
	} else if acl == nil || !acl.IsManagement() {
		return structs.ErrPermissionDenied
	}

	// Validate non-zero set of auth methods
	if len(args.AuthMethods) == 0 {
		return structs.NewErrRPCCoded(400, "must specify as least one auth method")
	}

	// Validate each auth method, compute hash
	for idx, authMethod := range args.AuthMethods {
		if err := authMethod.Validate(); err != nil {
			return structs.NewErrRPCCodedf(404, "auth method %d invalid: %v", idx, err)
		}
		authMethod.SetHash()
	}

	// Update via Raft
	_, index, err := a.srv.raftApply(structs.AuthMethodUpsertRequestType, args)
	if err != nil {
		return err
	}

	// Update the index
	reply.Index = index
	return nil
}

// DeleteAuthMethods is used to delete auth methods
func (a *AuthMethod) DeleteAuthMethods(args *structs.AuthMethodDeleteRequest, reply *structs.GenericResponse) error {
	// Ensure ACLs are enabled, and always flow modification requests to the authoritative region
	if !a.srv.config.ACLEnabled {
		return aclDisabled
	}
	args.Region = a.srv.config.AuthoritativeRegion

	if done, err := a.srv.forward("AuthMethod.DeleteAuthMethods", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "acl", "delete_auth_methods"}, time.Now())

	// Check management level permissions
	if acl, err := a.srv.ResolveToken(args.AuthToken); err != nil {
		return err
	} else if acl == nil || !acl.IsManagement() {
		return structs.ErrPermissionDenied
	}

	// Validate non-zero set of auth methods
	if len(args.Names) == 0 {
		return structs.NewErrRPCCoded(400, "must specify as least one auth method")
	}

	// Update via Raft
	_, index, err := a.srv.raftApply(structs.AuthMethodDeleteRequestType, args)
	if err != nil {
		return err
	}

	// Update the index
	reply.Index = index
	return nil
}

// ListAuthMethods is used to list the auth methods
func (a *AuthMethod) ListAuthMethods(args *structs.AuthMethodListRequest, reply *structs.AuthMethodListResponse) error {
	if !a.srv.config.ACLEnabled {
		return aclDisabled
	}

	if done, err := a.srv.forward("AuthMethod.ListAuthMethods", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "acl", "list_auth_methods"}, time.Now())

	// Check management level permissions
	acl, err := a.srv.ResolveToken(args.AuthToken)
	if err != nil {
		return err
	} else if acl == nil {
		return structs.ErrPermissionDenied
	}

	// TODO: This should be listing all of the auth methods
	// regardless of permissions. Make sure this is valid.

	// Setup the blocking query
	opts := blockingOptions{
		queryOpts: &args.QueryOptions,
		queryMeta: &reply.QueryMeta,
		run: func(ws memdb.WatchSet, state *state.StateStore) error {
			// Iterate over all the auth_methods
			var err error
			var iter memdb.ResultIterator
			if prefix := args.QueryOptions.Prefix; prefix != "" {
				iter, err = state.AuthMethodByNamePrefix(ws, prefix)
			} else {
				iter, err = state.AuthMethods(ws)
			}
			if err != nil {
				return err
			}

			// Convert all the auth_methods to a list stub
			reply.AuthMethods = nil
			for {
				raw := iter.Next()
				if raw == nil {
					break
				}
				authMethod := raw.(*structs.AuthMethod)
				reply.AuthMethods = append(reply.AuthMethods, authMethod.Stub())
			}

			// Use the last index that affected the auth_method table
			index, err := state.Index("auth_method")
			if err != nil {
				return err
			}

			// Ensure we never set the index to zero, otherwise a blocking query cannot be used.
			// We floor the index at one, since realistically the first write must have a higher index.
			if index == 0 {
				index = 1
			}
			reply.Index = index
			return nil
		}}
	return a.srv.blockingRPC(&opts)
}

// GetAuthMethod is used to get a specific auth method
func (a *AuthMethod) GetAuthMethod(args *structs.AuthMethodSpecificRequest, reply *structs.SingleAuthMethodResponse) error {
	if !a.srv.config.ACLEnabled {
		return aclDisabled
	}

	if done, err := a.srv.forward("AuthMethod.GetAuthMethod", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "acl", "get_auth_method"}, time.Now())

	// Check management level permissions
	acl, err := a.srv.ResolveToken(args.AuthToken)
	if err != nil {
		return err
	} else if acl == nil {
		return structs.ErrPermissionDenied
	}

	// Setup the blocking query
	opts := blockingOptions{
		queryOpts: &args.QueryOptions,
		queryMeta: &reply.QueryMeta,
		run: func(ws memdb.WatchSet, state *state.StateStore) error {
			// Look for the auth method
			out, err := state.AuthMethodByName(ws, args.Name)
			if err != nil {
				return err
			}

			// Setup the output
			reply.AuthMethod = out
			if out != nil {
				reply.Index = out.ModifyIndex

				if err != nil {
					return err
				}
			} else {
				// Use the last index that affected the auth method table
				index, err := state.Index("auth_method")
				if err != nil {
					return err
				}
				reply.Index = index
			}
			return nil
		}}
	return a.srv.blockingRPC(&opts)
}

func (a *AuthMethod) requestACLToken(secretID string) (*structs.ACLToken, error) {
	if secretID == "" {
		return structs.AnonymousACLToken, nil
	}

	snap, err := a.srv.fsm.State().Snapshot()
	if err != nil {
		return nil, err
	}

	return snap.ACLTokenBySecretID(nil, secretID)
}

// GetAuthMethods is used to get a set of auth methods
func (a *AuthMethod) GetAuthMethods(args *structs.AuthMethodSetRequest, reply *structs.AuthMethodSetResponse) error {
	if !a.srv.config.ACLEnabled {
		return aclDisabled
	}
	if done, err := a.srv.forward("AuthMethod.GetAuthMethods", args, args, reply); done {
		return err
	}
	defer metrics.MeasureSince([]string{"nomad", "acl", "get_auth_methods"}, time.Now())

	// For client typed tokens, allow them to query any auth methods associated with that token.
	// This is used by clients which are resolving the auth methods to enforce. Any associated
	// auth methods need to be fetched so that the client can determine what to allow.
	token, err := a.requestACLToken(args.AuthToken)
	if err != nil {
		return err
	}

	if token == nil {
		return structs.ErrTokenNotFound
	}
	if token.Type != structs.ACLManagementToken {
		return structs.ErrPermissionDenied
	}

	// Setup the blocking query
	opts := blockingOptions{
		queryOpts: &args.QueryOptions,
		queryMeta: &reply.QueryMeta,
		run: func(ws memdb.WatchSet, state *state.StateStore) error {
			// Setup the output
			reply.AuthMethods = make(map[string]*structs.AuthMethod, len(args.Names))

			// Look for the auth method
			for _, authMethodName := range args.Names {
				out, err := state.AuthMethodByName(ws, authMethodName)
				if err != nil {
					return err
				}
				if out != nil {
					reply.AuthMethods[authMethodName] = out
				}
			}

			// Use the last index that affected the auth method table
			index, err := state.Index("auth_method")
			if err != nil {
				return err
			}
			reply.Index = index
			return nil
		}}
	return a.srv.blockingRPC(&opts)
}
