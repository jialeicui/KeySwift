package conf

import (
	"errors"
	"fmt"

	"github.com/expr-lang/expr"
)

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("configuration is nil")
	}

	if c.ModeActions == nil {
		return errors.New("mode_actions is required")
	}

	for name, action := range c.ModeActions {
		if err := validateModeAction(name, action); err != nil {
			return err
		}
	}

	return nil
}

func validateModeAction(name string, ma ModeAction) error {
	if _, err := expr.Compile(ma.If); err != nil {
		return fmt.Errorf("mode condition '%s' is invalid: %w", name, err)
	}

	triggers := ma.Triggers

	if len(triggers) == 0 {
		return fmt.Errorf("mode triggers '%s' is empty", name)
	}

	for i, binding := range triggers {
		if binding == nil {
			return fmt.Errorf("trigger binding %d in mode '%s' is nil", i, name)
		}

		if binding.SourceEvent == nil {
			return fmt.Errorf("source_event in binding %d of mode '%s' is missing", i, name)
		}

		if binding.Action == nil {
			return fmt.Errorf("triggers in binding %d of mode '%s' is missing", i, name)
		}
	}

	return nil
}

// Additional validation functions for specific event types can be added here
