package roles_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/pkg/roles"
)

func TestRoleIDConstantsMatchSeedData(t *testing.T) {
	assert.Equal(t, uint(1), roles.AdminRoleId)
	assert.Equal(t, uint(2), roles.NormalRoleId)
	assert.Equal(t, uint(3), roles.VipRoleId)
}
