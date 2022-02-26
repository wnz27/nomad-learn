package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthMethods_ListUpsert(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t, nil, nil)
	defer s.Stop()
	ap := c.AuthMethods()

	// Listing when nothing exists returns empty
	result, qm, err := ap.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if qm.LastIndex != 1 {
		t.Fatalf("bad index: %d", qm.LastIndex)
	}
	if n := len(result); n != 0 {
		t.Fatalf("expected 0 methods, got: %d", n)
	}

	// Register a method
	method := &AuthMethod{
		Name: "test",
	}
	wm, err := ap.Upsert(method, nil)
	assert.Nil(t, err)
	assertWriteMeta(t, wm)

	// Check the list again
	result, qm, err = ap.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	assertQueryMeta(t, qm)
	if len(result) != 1 {
		t.Fatalf("expected policy, got: %#v", result)
	}
}

func TestAuthMethods_Delete(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t, nil, nil)
	defer s.Stop()
	ap := c.AuthMethods()

	// Register a method
	method := &AuthMethod{
		Name: "test",
	}
	wm, err := ap.Upsert(method, nil)
	assert.Nil(t, err)
	assertWriteMeta(t, wm)

	// Delete the method
	wm, err = ap.Delete(method.Name, nil)
	assert.Nil(t, err)
	assertWriteMeta(t, wm)

	// Check the list again
	result, qm, err := ap.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	assertQueryMeta(t, qm)
	if len(result) != 0 {
		t.Fatalf("unexpected policy, got: %#v", result)
	}
}

func TestAuthMethods_Info(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t, nil, nil)
	defer s.Stop()
	ap := c.AuthMethods()

	// Register a method
	method := &AuthMethod{
		Name: "test",
	}
	wm, err := ap.Upsert(method, nil)
	assert.Nil(t, err)
	assertWriteMeta(t, wm)

	// Query the method
	out, qm, err := ap.Info(method.Name, nil)
	assert.Nil(t, err)
	assertQueryMeta(t, qm)
	assert.Equal(t, method.Name, out.Name)
}

func TestAuthMethods_List(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t, nil, nil)
	defer s.Stop()
	at := c.ACLTokens()

	// Expect out bootstrap token
	result, qm, err := at.List(nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if qm.LastIndex == 0 {
		t.Fatalf("bad index: %d", qm.LastIndex)
	}
	if n := len(result); n != 1 {
		t.Fatalf("expected 1 token, got: %d", n)
	}
}
