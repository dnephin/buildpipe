package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gotestyourself/gotestyourself/assert"
)

func TestResolveBind(t *testing.T) {
	resolver := newFakeResolver(map[string]string{
		"~/{FOO}/": "~/bar/",
	})
	mount := &MountConfig{
		Bind: filepath.FromSlash("~/{FOO}/"),
	}

	res, err := mount.Resolve(resolver)
	assert.NilError(t, err)
	expected := filepath.Join(os.Getenv("HOME"), "bar")
	assert.Equal(t, res.(*MountConfig).Bind, expected)
}
