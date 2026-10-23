package client

import (
	"encoding/json"
	"os"
	"time"

	"go.uber.org/zap"
)

// savedSession is what LoginCached keeps on disk so separate processes (ex: one per camera) can share one login
type savedSession struct {
	APIURL string                  `json:"apiUrl"`
	DOMURL string                  `json:"domUrl"`
	Login  *V3_Auth_Login_Response `json:"login"`
}

// LoginCached reuses the session saved at path while it is valid for at least another 10 minutes, otherwise it logs in
// and saves the new session. A lock file stops several processes logging in at the same time.
// cached reports whether the saved session was used, if the API rejects it call ForgetSession and try again
func (LEZ *LE_EZVIZ_Client) LoginCached(path string) (cached bool, err error) {
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		log.Error("Error locking session file", zap.Error(err))
		return false, err
	}
	defer unlock()
	if LEZ.loadSession(path) {
		return true, nil
	}
	if _, err := LEZ.V3_Login(); err != nil {
		return false, err
	}
	LEZ.saveSession(path)
	return false, nil
}

func (LEZ *LE_EZVIZ_Client) ForgetSession(path string) {
	os.Remove(path)
}

func (LEZ *LE_EZVIZ_Client) loadSession(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var s savedSession
	if err := json.Unmarshal(b, &s); err != nil || s.Login == nil || s.Login.LoginSession.SessionId == nil {
		log.Warn("Ignoring unreadable session file", zap.String("Path", path))
		return false
	}
	claims, err := parseNoVerify(*s.Login.LoginSession.SessionId)
	if err != nil {
		return false
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil || time.Until(exp.Time) < 10*time.Minute {
		return false
	}
	LEZ.API_URL = s.APIURL
	LEZ.DOM_URL = s.DOMURL
	LEZ.LoginResponse = s.Login
	LEZ.Headers["sessionId"] = []string{*s.Login.LoginSession.SessionId}
	log.Debug("Reusing saved session", zap.Time("Expires", exp.Time))
	return true
}

func (LEZ *LE_EZVIZ_Client) saveSession(path string) {
	b, err := json.Marshal(savedSession{APIURL: LEZ.API_URL, DOMURL: LEZ.DOM_URL, Login: LEZ.LoginResponse})
	if err == nil {
		err = os.WriteFile(path, b, 0600)
	}
	if err != nil {
		log.Warn("Could not save session", zap.Error(err))
	}
}

// Login logs in, with a session file it reuses the saved session and logs in again if the API rejects it.
// It also fetches the server info, which every later request needs
func (LEZ *LE_EZVIZ_Client) Login(sessionFile string) error {
	if sessionFile == "" {
		if _, err := LEZ.V3_Login(); err != nil {
			return err
		}
		_, err := LEZ.GetServerInfo()
		return err
	}
	cached, err := LEZ.LoginCached(sessionFile)
	if err != nil {
		return err
	}
	if _, err = LEZ.GetServerInfo(); err != nil && cached {
		log.Warn("Saved session rejected, logging in again")
		LEZ.ForgetSession(sessionFile)
		if _, err = LEZ.LoginCached(sessionFile); err != nil {
			return err
		}
		_, err = LEZ.GetServerInfo()
	}
	return err
}
