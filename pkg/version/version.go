package version

// Version represents the current version of Ergs
const Version = "3.6.0"

// BuildVersion returns the version string for display
func BuildVersion() string {
	return "ergs version " + Version
}

// APIVersion returns just the version number for API responses
func APIVersion() string {
	return Version
}
