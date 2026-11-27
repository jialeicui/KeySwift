package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jialeicui/golibevdev"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestConfig(t *testing.T, content string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return LoadConfig(path)
}

func TestLoadConfigModifierDefaults(t *testing.T) {
	cfg, err := loadTestConfig(t, `[]`)
	require.NoError(t, err)
	assert.Equal(t, DefaultOptimisticModifiers(), cfg.Settings.OptimisticModifiers)
	assert.True(t, cfg.Settings.PointerFlush)
}

func TestLoadConfigModifierDefaultsPreservedWithOtherSettings(t *testing.T) {
	cfg, err := loadTestConfig(t, `[{"type":"settings","chatterThreshold":"80ms"}]`)
	require.NoError(t, err)
	assert.Equal(t, DefaultOptimisticModifiers(), cfg.Settings.OptimisticModifiers)
	assert.True(t, cfg.Settings.PointerFlush)
}

func TestLoadConfigOptimisticModifiersOverride(t *testing.T) {
	cfg, err := loadTestConfig(t, `[{"type":"settings","optimisticModifiers":["shift"],"pointerFlush":false}]`)
	require.NoError(t, err)
	assert.Equal(t, []keys.Key{golibevdev.KeyLeftShift}, cfg.Settings.OptimisticModifiers)
	assert.False(t, cfg.Settings.PointerFlush)
}

func TestLoadConfigOptimisticModifiersEmptyDisables(t *testing.T) {
	cfg, err := loadTestConfig(t, `[{"type":"settings","optimisticModifiers":[]}]`)
	require.NoError(t, err)
	assert.NotNil(t, cfg.Settings.OptimisticModifiers)
	assert.Empty(t, cfg.Settings.OptimisticModifiers)
}

func TestLoadConfigOptimisticModifiersValidation(t *testing.T) {
	_, err := loadTestConfig(t, `[{"type":"settings","optimisticModifiers":["a"]}]`)
	require.ErrorContains(t, err, "not a modifier")

	_, err = loadTestConfig(t, `[{"type":"settings","optimisticModifiers":["nonexistent"]}]`)
	require.ErrorContains(t, err, "invalid optimisticModifiers")
}
