package staffutil

import (
	"cyberteam/internal/profile"
	"cyberteam/internal/protocol"
	"os"
)

// GetString extracts a string value from a map with a default fallback.
func GetString(m map[string]any, key, defaultVal string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

// GetEnv returns the environment variable value or a default.
func GetEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// BuildCapabilities converts profile capabilities to protocol capabilities.
// Falls back to the provided defaults if the profile has none.
func BuildCapabilities(prof *profile.Profile) []protocol.Capability {
	if len(prof.Capabilities) > 0 {
		caps := make([]protocol.Capability, len(prof.Capabilities))
		for i, cap := range prof.Capabilities {
			caps[i] = protocol.Capability{
				Name:        cap.Name,
				Description: cap.Description,
				Inputs:      ConvertParams(cap.Inputs),
				Outputs:     ConvertParams(cap.Outputs),
				EstTime:     cap.EstTime,
			}
		}
		return caps
	}
	return nil
}

// BuildCapabilitiesWithDefaults converts profile capabilities to protocol capabilities,
// falling back to provided defaults if the profile has none.
func BuildCapabilitiesWithDefaults(prof *profile.Profile, defaults []protocol.Capability) []protocol.Capability {
	caps := BuildCapabilities(prof)
	if caps != nil {
		return caps
	}
	return defaults
}

// ConvertParams converts profile params to protocol params.
func ConvertParams(params []profile.Param) []protocol.Param {
	if len(params) == 0 {
		return nil
	}
	result := make([]protocol.Param, len(params))
	for i, p := range params {
		result[i] = protocol.Param{
			Name:     p.Name,
			Type:     p.Type,
			Required: p.Required,
			Desc:     p.Desc,
		}
	}
	return result
}
