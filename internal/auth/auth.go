package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dghubble/gologin"
	"github.com/dghubble/gologin/github"
	"github.com/dghubble/sessions"
	gogithub "github.com/google/go-github/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	githuboauth2 "golang.org/x/oauth2/github"
)

const (
	sessionName    = "goreadme"
	sessionUserKey = "user"
)

type Auth struct {
	SessionSecret string
	GithubID      string
	GithubSecret  string
	Domain        string
	RedirectPath  string
	LoginPath     string
	HomePath      string
	Scopes        []string

	sessionStore *sessions.CookieStore
}

func (a *Auth) Init() {
	a.sessionStore = sessions.NewCookieStore([]byte(a.SessionSecret), nil)
}

func (a *Auth) CallbackHandler() http.Handler {
	return github.StateHandler(a.cookieConfig(),
		github.CallbackHandler(a.config(), http.HandlerFunc(a.loginSuccess), http.HandlerFunc(a.loginFailed)))
}

func (a *Auth) LoginHandler() http.Handler {
	return github.StateHandler(a.cookieConfig(), github.LoginHandler(a.config(), nil))
}

func (a *Auth) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.sessionStore.Destroy(w, sessionName)
		http.Redirect(w, r, a.LoginPath, http.StatusFound)
	})
}

// loginSuccess issues a cookie session after successful Github login
func (a *Auth) loginSuccess(w http.ResponseWriter, r *http.Request) {
	logrus.Infof("Login succeeded")
	u, err := github.UserFromContext(r.Context())
	if err != nil {
		logrus.Errorf("Getting user from context: %s", err)
		http.Error(w, "Failed", http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(u)
	if err != nil {
		logrus.Errorf("Marshaling user: %+v: %s", u, err)
		http.Error(w, "Failed", http.StatusInternalServerError)
		return
	}

	session := a.sessionStore.New(sessionName)
	session.Values[sessionUserKey] = string(b)
	session.Save(w)
	http.Redirect(w, r, a.HomePath, http.StatusFound)
}

func (a *Auth) loginFailed(w http.ResponseWriter, r *http.Request) {
	err := gologin.ErrorFromContext(r.Context())
	logrus.Infof("Login failed: %s", err)
	http.Redirect(w, r, a.LoginPath+"?error=unauthorized", http.StatusFound)
}

func (a *Auth) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     a.GithubID,
		ClientSecret: a.GithubSecret,
		RedirectURL:  a.Domain + a.RedirectPath,
		Scopes:       a.Scopes,
		Endpoint:     githuboauth2.Endpoint,
	}
}

func (a *Auth) cookieConfig() gologin.CookieConfig {
	c := gologin.CookieConfig{
		Name:     "gologin-temp",
		HTTPOnly: true,
		Secure:   true,
		Domain:   a.Domain,
	}
	if !strings.HasPrefix(a.Domain, "https") {
		logrus.Warn("Using insecure cookie")
		c.Secure = false
	}
	return c
}

type contextKey string

const keyUser contextKey = "user"

// RequireLogin redirects unauthenticated users to the login route.
// It stores the user in the request context in case that the
// request is authenticated.
func (a *Auth) RequireLogin(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !a.IsAuthenticated(r) {
			http.Redirect(w, r, a.LoginPath, http.StatusFound)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), keyUser, a.user(r)))
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// MayLogin sets the user to the context if it is available.
func (a *Auth) MayLogin(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if a.IsAuthenticated(r) {
			r = r.WithContext(context.WithValue(r.Context(), keyUser, a.user(r)))
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// User returns the user from the request context.
func (a *Auth) User(r *http.Request) *gogithub.User {
	u := r.Context().Value(keyUser)
	if u == nil {
		return nil
	}
	return u.(*gogithub.User)
}

// IsAuthenticated returns true if the user has a signed session cookie.
func (a *Auth) IsAuthenticated(r *http.Request) bool {
	_, err := a.sessionStore.Get(r, sessionName)
	return err == nil
}

// user returns the user object from the session.
func (a *Auth) user(r *http.Request) *gogithub.User {
	s, err := a.sessionStore.Get(r, sessionName)
	if err != nil {
		logrus.Errorf("Failed getting user: %s", err)
		return nil
	}
	jsonData, ok := s.Values[sessionUserKey].(string)
	if !ok {
		logrus.Errorf("Failed converting user key: %s", s.Values[sessionUserKey])
		return nil
	}
	var u gogithub.User
	err = json.Unmarshal([]byte(jsonData), &u)
	if err != nil {
		logrus.Errorf("Failed marhsalling user data %s: %s", jsonData, err)
		return nil
	}

	return &u
}
