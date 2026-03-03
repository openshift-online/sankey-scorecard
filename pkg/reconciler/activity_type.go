package reconciler

// ExtractActivityType extracts the Activity Type value from a Jira issue's
// Unknowns map. The Activity Type custom field value is typically stored as
// a map with a "value" key (Jira's custom field structure).
// Returns empty string if the field is nil, missing, or has an unexpected type.
func ExtractActivityType(unknowns map[string]interface{}, fieldID string) string {
	if unknowns == nil || fieldID == "" {
		return ""
	}

	raw, ok := unknowns[fieldID]
	if !ok || raw == nil {
		return ""
	}

	// The Activity Type field value is typically a map with a "value" key
	if m, ok := raw.(map[string]interface{}); ok {
		if val, ok := m["value"]; ok {
			if s, ok := val.(string); ok {
				return s
			}
		}
		return ""
	}

	// Handle case where the value is directly a string
	if s, ok := raw.(string); ok {
		return s
	}

	return ""
}
