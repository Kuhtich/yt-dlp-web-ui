package user

import (
	"encoding/json"
	"net/http"

	"github.com/marcopiovanello/yt-dlp-web-ui/v3/server/config"
	"github.com/marcopiovanello/yt-dlp-web-ui/v3/server/session"
)

const TOKEN_COOKIE_NAME = session.CookieName

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var (
		username = config.Instance().Username
		password = config.Instance().Password
	)

	if username != req.Username || password != req.Password {
		http.Error(w, "invalid username or password", http.StatusBadRequest)
		return
	}

	tokenString, err := session.Create(req.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(tokenString); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func Logout(w http.ResponseWriter, r *http.Request) {
	session.ClearCookie(w, r)
}
