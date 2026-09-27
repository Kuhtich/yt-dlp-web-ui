package user

import (
	"encoding/json"
	"net/http"

	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/config"
	"github.com/marcopiovanello/yt-dlp-web-ui/v4/server/session"
	"golang.org/x/crypto/bcrypt"
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
		username     = config.Instance().Authentication.Username
		passwordHash = config.Instance().Authentication.PasswordHash
	)

	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password))
	if err != nil {
		http.Error(w, "invalid username or password", http.StatusBadRequest)
		return
	}

	if username != req.Username {
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
