package conf

import "path/filepath"

// AppGroup represents a group of applications with similar characteristics
type AppGroup struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	AppPatterns []string `yaml:"appPatterns"` // Window class patterns to match applications
}

// MatchesApp checks if the given window class/application matches this app group
func (a *AppGroup) MatchesApp(windowClass string) bool {
	for _, pattern := range a.AppPatterns {
		matched, _ := filepath.Match(pattern, windowClass)
		if matched {
			return true
		}
	}
	return false
}
