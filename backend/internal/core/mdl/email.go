package mdl

import "net/mail"

// IsValidEmail reports whether addr is a syntactically valid bare email address
// (e.g. "user@example.com"). Display-name forms ("Name <user@example.com>")
// are rejected. This is a best-effort format check — it does not verify that
// the address exists or is reachable. The only reliable validation is a
// successful magic-link verification, which proves the email reached the user.
func IsValidEmail(addr string) bool {
	parsed, err := mail.ParseAddress(addr)
	return err == nil && parsed.Address == addr
}
