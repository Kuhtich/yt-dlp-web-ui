package session

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CookieName is the cookie carrying the application session token.
// The same token is accepted as the X-Authentication header or as the
// token query parameter, see FromRequest.
const CookieName = "jwt-yt-dlp-webui"

// TTL is how long an application issued session stays valid.
// Sessions issued by an OpenID login use this TTL: the OpenID ID token is
// only used to verify the login and expires by itself after about one hour.
const TTL = 30 * 24 * time.Hour

// Create returns a signed session token for the given user.
func Create(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"expiresAt": time.Now().Add(TTL),
		"username":  username,
	})

	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

// Validate checks the token signature and its expiry.
func Validate(tokenValue string) error {
	token, err := jwt.Parse(tokenValue, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})
	if err != nil {
		return err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return errors.New("invalid token")
	}

	expiresAt, ok := claims["expiresAt"].(string)
	if !ok {
		return errors.New("invalid token")
	}

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return err
	}

	if time.Now().After(expiry) {
		return errors.New("token expired")
	}

	return nil
}

// FromRequest extracts the session token from the request: the session cookie
// first, then the X-Authentication header, then the token query parameter.
func FromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(CookieName); err == nil {
		return cookie.Value
	}

	if token := r.Header.Get("X-Authentication"); token != "" {
		return token
	}

	return r.URL.Query().Get("token")
}

// SetCookie writes the session cookie holding the given token.
func SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		HttpOnly: true,
		Path:     "/",
		Secure:   r.TLS != nil,
		Expires:  time.Now().Add(TTL),
	})
}

// ClearCookie expires the session cookie.
func ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		HttpOnly: true,
		Path:     "/",
		Secure:   r.TLS != nil,
		MaxAge:   -1,
	})
}
