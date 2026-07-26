package auth

// Method identifies one supported interactive authentication flow.
type Method string

const (
	MethodPassword       Method = "password"
	MethodBrowserSession Method = "browser-session"
)

// Valid reports whether method names a supported authentication flow.
func (method Method) Valid() bool {
	return method == MethodPassword || method == MethodBrowserSession
}
