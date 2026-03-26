package edgefunc

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestEdgeFunctionEffectiveTimeout(t *testing.T) {
	t.Parallel()

	fn := EdgeFunction{Timeout: 0}
	testutil.Equal(t, DefaultTimeout, fn.EffectiveTimeout())

	fn.Timeout = 3 * time.Second
	testutil.Equal(t, 3*time.Second, fn.EffectiveTimeout())
}

func TestEdgeFunctionValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{
			Name:       "hello",
			EntryPoint: "handler",
			Timeout:    2 * time.Second,
			EnvVars:    map[string]string{"FOO": "bar"},
			Public:     true,
			Source:     "export default function handler() {}",
		}
		testutil.NoError(t, fn.Validate())
	})

	t.Run("valid with source path", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{
			Name:       "hello",
			EntryPoint: "handler",
			SourcePath: "functions/hello.ts",
		}
		testutil.NoError(t, fn.Validate())
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{EntryPoint: "handler"}
		testutil.ErrorContains(t, fn.Validate(), "name is required")
	})

	t.Run("missing entry point", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{Name: "hello"}
		testutil.ErrorContains(t, fn.Validate(), "entry point is required")
	})

	t.Run("negative timeout", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{Name: "hello", EntryPoint: "handler", Timeout: -1 * time.Second}
		testutil.ErrorContains(t, fn.Validate(), "timeout must be non-negative")
	})

	t.Run("missing source and source path", func(t *testing.T) {
		t.Parallel()
		fn := EdgeFunction{Name: "hello", EntryPoint: "handler"}
		testutil.ErrorContains(t, fn.Validate(), "source or source path is required")
	})
}
