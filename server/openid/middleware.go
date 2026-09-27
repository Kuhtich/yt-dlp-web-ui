package openid

import (
	"net/http"

	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/session"
)

// Middleware accepts the session token issued at sign in.
//
// The OpenID ID token itself is only used once, to verify the login: it
// expires by itself after about one hour and cannot be extended, which used to
// log the user out while the browser was still open.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := session.Validate(session.FromRequest(r)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}
