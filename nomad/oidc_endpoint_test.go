package nomad

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// https://github.com/hashicorp/nomad/blob/main/contributing/checklist-rpc-endpoint.md

func TestACLEndpoint_AuthURLRequest(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a", "b")
}
