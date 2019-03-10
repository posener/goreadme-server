package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/posener/goreadme-server/internal/auth"
	"github.com/posener/goreadme-server/internal/githubapp"
	"github.com/posener/goreadme-server/internal/templates"
	"github.com/sirupsen/logrus"
)

type handler struct {
	auth   *auth.Auth
	db     *gorm.DB
	github *githubapp.GithubClients
}

type templateData struct {
	User      *github.User
	InstallID int64
	Repos     []*github.Repository
	Projects  []Project
	Jobs      []Job
	Stats     stats
	// Holds an error that happened to show to the user
	Error string
}

type stats struct {
	// TopProjects will contain top open source projects
	TopProjects   []Project
	TotalProjects int
}

type contextKey string

const contextClient contextKey = "client"

func client(r *http.Request) *githubapp.User {
	if client := r.Context().Value(contextClient); client != nil {
		return client.(*githubapp.User)
	}
	return nil
}

func (h *handler) dataFromRequest(w http.ResponseWriter, r *http.Request) *templateData {
	data := templateData{
		Error: r.URL.Query().Get("error"),
		User:  h.auth.User(r),
	}
	if data.User != nil {
		login := data.User.GetLogin()
		userClient, err := h.github.User(r.Context(), login)
		if err != nil {
			logrus.Warnf("Failed getting install ID for login %s: %s", login, err)
		} else {
			data.InstallID = userClient.InstallID
			*r = *r.WithContext(context.WithValue(r.Context(), contextClient, userClient))
		}
	}
	return &data
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	// nil user is valid here.

	err := h.db.Model(&Project{}).Where("private = FALSE").Order("stars DESC").Scan(&data.Stats.TopProjects).Limit(10).Error
	if err != nil {
		logrus.Errorf("Failed scanning open source projects: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = h.db.Model(&Project{}).Count(&data.Stats.TotalProjects).Error
	if err != nil {
		logrus.Errorf("Failed counting projects: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = templates.Home.Execute(w, data)
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed executing template"))
	}
}

func (h *handler) projectsList(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data.User == nil {
		return
	}

	var wh where
	wh.AddValues(r.URL.Query(), "owner", "repo", "id")
	wh.Add("install", data.InstallID)

	err := wh.Apply(h.db.Model(&Project{}).Order("updated_at DESC")).Scan(&data.Projects).Error
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed scanning projects"))
		return
	}

	err = templates.Projects.Execute(w, data)
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed executing template"))
	}
}

func (h *handler) jobsList(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data.User == nil {
		return
	}

	var wh where
	wh.AddValues(r.URL.Query(), "owner", "repo", "id")
	wh.Add("install", data.InstallID)

	err := wh.Apply(h.db.Model(&Job{}).Order("updated_at DESC")).Scan(&data.Jobs).Error
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed scanning jobs"))
		return
	}
	err = templates.JobsList.Execute(w, data)
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed executing template"))
	}
}

func (h *handler) addRepo(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data.User == nil {
		return
	}

	if c := client(r); c != nil {
		repos, _, err := c.Github.Apps.ListRepos(r.Context(), nil)
		if err != nil {
			h.doError(w, r, errors.Wrap(err, "failed getting repos"))
			return
		}
		data.Repos = repos
	}
	err := templates.AddRepo.Execute(w, data)
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed executing template"))
	}
}

func (h *handler) addRepoAction(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data.User == nil {
		return
	}

	var (
		owner = r.FormValue("owner")
		repo  = r.FormValue("repo")
	)

	logrus.Info("Running goreadme in background...")
	_, jobNum, err := h.runJob(r.Context(), &Project{
		Owner:   owner,
		Repo:    repo,
		Install: data.InstallID,
	})
	if err != nil {
		h.doError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/jobs?owner=%s&repo=%s&num=%d", owner, repo, jobNum), http.StatusFound)
}

func (h *handler) badge(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]

	var p Project
	err := h.db.Model(&p).Where("owner = ? AND repo = ?", owner, repo).First(&p).Error
	if err != nil {
		logrus.Errorf("Failed getting project %s/%s", owner, repo)
	}

	w.Header().Add("Content-Type", "image/svg+xml")

	err = templates.Badge.Execute(w, &p)
	if err != nil {
		h.doError(w, r, errors.Wrap(err, "failed executing template"))
	}
}

func (h *handler) doError(w http.ResponseWriter, r *http.Request, err error) {
	logrus.Error(err)
	http.Redirect(w, r, "/?error=internal%20server%error", http.StatusFound)
}

// debugPR runs in debug mode provide the required environment variables.
// Run with:
//
// 		DEBUG_HOOK=1 REPO=repo OWNER=$USER HEAD=$(git rev-parse HEAD) go run .
//
func (h *handler) debugPR() {
	if os.Getenv("DEBUG_HOOK") != "1" {
		return
	}
	logrus.Warnf("Debugging hook mode!")
	done, _, err := h.runJob(context.Background(), &Project{
		Owner:         os.Getenv("OWNER"),
		Repo:          os.Getenv("REPO"),
		HeadSHA:       os.Getenv("HEAD"),
		DefaultBranch: "master",
	})
	if err != nil {
		logrus.Errorf("Failed job: %s", err)
		os.Exit(1)
	}
	<-done
	os.Exit(0)
}

func branchOfRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

type where struct {
	strs []string
	args []interface{}
}

func (w *where) AddValues(values url.Values, keys ...string) *where {
	for _, k := range keys {
		if v := values.Get(k); v != "" {
			w.Add(k, v)
		}
	}
	return w
}

func (w *where) Add(key string, val interface{}) *where {
	w.strs = append(w.strs, fmt.Sprintf("%s = ?", key))
	w.args = append(w.args, val)
	return w
}

func (w *where) Apply(db *gorm.DB) *gorm.DB {
	return db.Where(strings.Join(w.strs, " AND "), w.args...)
}
