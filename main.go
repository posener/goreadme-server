// Package main is an HTTP server that works with Github hooks.
//
// [goreadme](github.com/posener/goreadme) is a tool for creating README.md
// files from Go doc of a given package.
// This project is the Github app on top of this tool. It fully automates
// the process of keeping the README.md file updated.
//
// ## Usage
//
// 1. Go to [https://github.com/apps/goreadme](https://github.com/apps/goreadme).
// 2. Press the "Configure" button.
// 3. choose your account, or an organization that owns the repository.
// 4. Review the permissions and provide access to goreadme to repositories.
// 5. Click Save.
//
// You should see PRs from goreadme bot in your github repos.
//
// For more features, or to trigger goreadme on demand to to
// [goreadme site](https://goreadme.herokuapp.com).
//
// ## How does it Work?
//
// Once integrated with a repository, goreadme is registered on a Github hook,
// that calls goreadme server whenever the repository default branch is
// modified. Goreadme then computes the new README.md file and compairs it
// to the exiting one. If a change is needed, Goreadme will create a PR with
// the new content of the README.md file.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/posener/goreadme-server/internal/googleanalytics"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/posener/goreadme-server/internal/auth"
	"github.com/posener/goreadme-server/internal/githubapp"
	"github.com/sirupsen/logrus"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

var (
	domain             = os.Getenv("DOMAIN")
	port               = os.Getenv("PORT")
	dbURL              = os.Getenv("DATABASE_URL")
	sessionSecret      = os.Getenv("SESSION_SECRET")
	githubAppID        = os.Getenv("GITHUB_APP_ID")
	githubKey          = os.Getenv("GITHUB_KEY")
	githubClientID     = os.Getenv("GITHUB_ID")
	githubClientSecret = os.Getenv("GITHUB_SECRET")
	debug              = os.Getenv("DEBUG_SERVER") == "1"
)

func main() {
	ctx := context.Background()
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cfg := githubapp.Config{
		AppID:      githubAppID,
		PrivateKey: []byte(githubKey),
		Expires:    time.Second * 60 * 10,
	}

	client := cfg.Clients(ctx)
	db, err := gorm.Open("postgres", dbURL)
	if err != nil {
		logrus.Fatalf("Connect to DB on %s: %v", dbURL, err)
	}
	defer db.Close()
	if debug {
		db.LogMode(true)
	}

	if err := db.AutoMigrate(&Job{}, &Project{}).Error; err != nil {
		logrus.Fatalf("Migrate database: %s", err)
	}

	a := &auth.Auth{
		SessionSecret: sessionSecret,
		GithubID:      githubClientID,
		GithubSecret:  githubClientSecret,
		Domain:        domain,
		RedirectPath:  "/auth/callback",
		LoginPath:     "/",
		HomePath:      "/",
	}

	a.Init()

	h := &handler{
		auth:   a,
		db:     db,
		github: client,
	}
	h.debugPR()

	m := mux.NewRouter()
	m.Methods("GET").Path("/").Handler(a.MayLogin(http.HandlerFunc(h.home)))
	m.Methods("GET").Path("/projects").Handler(a.RequireLogin(http.HandlerFunc(h.projectsList)))
	m.Methods("GET").Path("/jobs").Handler(a.RequireLogin(http.HandlerFunc(h.jobsList)))
	m.Methods("POST").Path("/add").Handler(a.RequireLogin(http.HandlerFunc(h.addRepoAction)))
	m.Methods("GET").Path("/add").Handler(a.RequireLogin(http.HandlerFunc(h.addRepo)))
	m.Methods("GET").Path("/badge/{owner}/{repo}.svg").HandlerFunc(http.HandlerFunc(h.badge))
	m.Methods("POST").Path("/github/hook").HandlerFunc(h.hook)
	m.Path("/auth/login").Handler(a.LoginHandler())
	m.Path("/auth/logout").Handler(a.LogoutHandler())
	m.Path("/auth/callback").Handler(a.CallbackHandler())

	googleanalytics.AddToRouter(m, "/analytics")

	mh := handlers.RecoveryHandler(handlers.PrintRecoveryStack(true), handlers.RecoveryLogger(logrus.StandardLogger()))(m)
	if debug {
		mh = handlers.LoggingHandler(logrus.StandardLogger().Writer(), mh)
	}

	logrus.Infof("Starting server...")
	http.ListenAndServe(":"+port, mh)
}
