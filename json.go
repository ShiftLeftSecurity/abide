package abide

// updateKeyValuesInMap updates every instance of a key within an arbitrary
// `map[string]interface{}` with the given value.
func updateKeyValuesInMap(key string, value interface{}, m map[string]interface{}) map[string]interface{} {
	return updateMap(key, value, m)
}

// Recursively update the map to update the specified key.
func updateMap(key string, value interface{}, m map[string]interface{}) map[string]interface{} {
	for k, v := range m {
		switch s := v.(type) {
		// If slice, iterate through each entry and call updateMap
		// only if it's a map[string]interface{}.
		case []interface{}:
			for i := range s {
				switch s[i].(type) {
				case map[string]interface{}:
					v.([]interface{})[i] = updateMap(key, value, v.([]interface{})[i].(map[string]interface{}))
				}
			}
		case map[string]interface{}:
			m[k] = updateMap(key, value, s)
		default:
			if k == key {
				m[k] = value
			}
		}
	}

	return m
}
