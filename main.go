// Package main is an HTTP server that works with Github hooks.
//
// (Goreadme) https://github.com/posener/goreadme is a tool for creating README.md
// files from Go doc of a given package.
// This project is the Github app on top of this tool. It fully automates
// the process of keeping the README.md file updated.
//
// Usage
//
// 1. Go to https://github.com/apps/goreadme.
//
// 2. Press the "Configure" button.
//
// 3. Choose your account, or an organization that owns the repository.
//
// 4. Review the permissions and provide access to goreadme to repositories.
//
// 5. Click Save.
//
// You should see PRs from goreadme bot in your github repos.
// For more features, or to trigger goreadme on demand, use the
// (Goreadme website) https://goreadme.herokuapp.com.
//
// How does it Work
//
// Once integrated with a repository, goreadme is registered on a Github hook,
// that calls goreadme server whenever the repository default branch is
// modified. Goreadme then computes the new README.md file and compairs it
// to the exiting one. If a change is needed, Goreadme will create a PR with
// the new content of the README.md file.
//
// Customization
//
// Adding a `goreadme.json` file to your repository main directory can enable some
// customization to the generated readme file. The configuration is available
// according to (goreadme.Config struct) https://godoc.org/github.com/posener/goreadme#Config.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/posener/goreadme-server/internal/googleanalytics"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/kelseyhightower/envconfig"
	"github.com/posener/goreadme-server/internal/auth"
	"github.com/posener/githubapp"
	"github.com/posener/githubapp/cache"
	"github.com/sirupsen/logrus"

	_ "github.com/jinzhu/gorm/dialects/postgres"
)

var cfg struct {
	Domain           string `required:"true" split_words:"true"`
	Port             int    `required:"true" split_words:"true"`
	DatabaseURL      string `required:"true" split_words:"true"`
	SessionSecret    string `required:"true" split_words:"true"`
	GithubAppID      int    `required:"true" split_words:"true"`
	GithubKey        string `required:"true" split_words:"true"`
	GithubID         string `required:"true" split_words:"true"`
	GithubSecret     string `required:"true" split_words:"true"`
	GithubHookSecret string `required:"true" split_words:"true"`
	Debug            bool   `default:"false" envconfig:"debug_server"`
}

func init() {
	flag.Usage = func() {
		envconfig.Usage("", &cfg)
	}
	flag.Parse()
	err := envconfig.Process("", &cfg)
	if err != nil {
		logrus.Fatal(err)
	}
}

func main() {
	ctx := context.Background()
	if cfg.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ghCfg := githubapp.Config{
		AppID:      strconv.Itoa(cfg.GithubAppID),
		PrivateKey: []byte(cfg.GithubKey),
	}

	client := ghCfg.NewApp(ctx, githubapp.OptWithCache(cache.New(time.Minute*5, time.Minute*10)))
	db, err := gorm.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logrus.Fatalf("Connect to DB on %s: %v", cfg.DatabaseURL, err)
	}
	defer db.Close()
	if cfg.Debug {
		db.LogMode(true)
	}

	if err := db.AutoMigrate(&Job{}, &Project{}).Error; err != nil {
		logrus.Fatalf("Migrate database: %s", err)
	}

	a := &auth.Auth{
		SessionSecret: cfg.SessionSecret,
		GithubID:      cfg.GithubID,
		GithubSecret:  cfg.GithubSecret,
		Domain:        cfg.Domain,
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
	if cfg.Debug {
		mh = handlers.LoggingHandler(logrus.StandardLogger().Writer(), mh)
	}

	logrus.Infof("Starting server...")
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), mh)
}
