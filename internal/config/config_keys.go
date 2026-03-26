package config

func buildValidKeys() map[string]bool {
	keys := make(map[string]bool, len(configKeyRegistry))
	for key := range configKeyRegistry {
		keys[key] = true
	}
	return keys
}

// validKeys is the complete set of dot-separated config keys.
var validKeys = buildValidKeys()
