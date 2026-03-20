package capabilities

import (
	"fmt"
)

var unsupportedSchemaKeywords = map[string]struct{}{
	"oneOf":             {},
	"anyOf":             {},
	"allOf":             {},
	"not":               {},
	"patternProperties": {},
}

func ValidateSchemaSubset(schema map[string]any) error {
	return walkSchema(schema)
}

func walkSchema(schema map[string]any) error {
	for key, value := range schema {
		if _, blocked := unsupportedSchemaKeywords[key]; blocked {
			return fmt.Errorf("unsupported schema keyword %q", key)
		}
		if key == "additionalProperties" {
			if boolean, ok := value.(bool); !ok || boolean {
				return fmt.Errorf("unsupported schema keyword %q", key)
			}
		}

		switch typed := value.(type) {
		case map[string]any:
			if err := walkSchema(typed); err != nil {
				return err
			}
		case []any:
			for _, item := range typed {
				nested, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if err := walkSchema(nested); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
