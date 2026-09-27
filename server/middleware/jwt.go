package middlewares

import (
	"net/http"

	"github.com/marcopiovanello/yt-dlp-web-ui/v3/server/session"
)

// Authenticated accepts the session token as a cookie, as the
// X-Authentication header or as the token query parameter.

func Authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := session.Validate(session.FromRequest(r)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}
