package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
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

var githubHookSecret = []byte(os.Getenv("GITHUB_HOOK_SECRET")) // Secret for github hooks

var githubAppIDInt, _ = strconv.Atoi(githubAppID)

type handler struct {
	auth   *auth.Auth
	db     *gorm.DB
	github *githubapp.GithubClients
}

type templateData struct {
	User               *github.User
	InstallID          int64
	Repos              []*github.Repository
	Projects           []Project
	Jobs               []Job
	OpenSourceProjects []Project
	// Holds an error that happened to show to the user
	Error string
}

func (h *handler) dataFromRequest(w http.ResponseWriter, r *http.Request) *templateData {
	var (
		data templateData
		err  error
	)
	data.User = h.auth.User(r)
	data.InstallID, err = h.github.InstallID(r.Context(), data.User.GetLogin())
	if err != nil {
		logrus.Warnf("Failed getting install ID: %s", err)
	}
	data.Error = r.URL.Query().Get("error")
	return &data
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)

	err := h.db.Model(&Project{}).Where("private = FALSE").Order("updated_at DESC").Scan(&data.OpenSourceProjects).Limit(50).Error
	if err != nil {
		logrus.Errorf("Failed scanning open source projects: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = templates.Home.Execute(w, data)
	if err != nil {
		h.doError(errors.Wrap(err, "failed executing template"), w, r)
	}
}

func (h *handler) projectsList(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data == nil {
		return
	}

	var wh where
	wh.AddValues(r.URL.Query(), "owner", "repo", "id")
	wh.Add("install", data.InstallID)

	err := wh.Apply(h.db.Model(&Project{}).Order("updated_at DESC")).Scan(&data.Projects).Error
	if err != nil {
		logrus.Errorf("Failed scanning projects: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = templates.Projects.Execute(w, data)
	if err != nil {
		logrus.Errorf("Failed executing template: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *handler) jobsList(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data == nil {
		return
	}

	var wh where
	wh.AddValues(r.URL.Query(), "owner", "repo", "id")
	wh.Add("install", data.InstallID)

	err := wh.Apply(h.db.Model(&Job{}).Order("updated_at DESC")).Scan(&data.Jobs).Error
	if err != nil {
		h.doError(errors.Wrap(err, "failed scanning jobs"), w, r)
		return
	}
	err = templates.JobsList.Execute(w, data)
	if err != nil {
		h.doError(errors.Wrap(err, "failed executing template"), w, r)
	}
}

func (h *handler) addRepo(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data == nil {
		return
	}

	gh, err := h.github.UserGithubClient(r.Context(), data.User.GetLogin())
	if err != nil {
		h.doError(errors.Wrap(err, "failed getting github client"), w, r)
		return
	}

	repos, _, err := gh.Apps.ListRepos(r.Context(), nil)
	if err != nil {
		h.doError(errors.Wrap(err, "failed getting repos"), w, r)
		return
	}

	data.Repos = repos
	err = templates.AddRepo.Execute(w, data)
	if err != nil {
		h.doError(errors.Wrap(err, "failed executing template"), w, r)
	}
}

func (h *handler) addRepoAction(w http.ResponseWriter, r *http.Request) {
	data := h.dataFromRequest(w, r)
	if data == nil {
		return
	}

	var (
		owner = r.FormValue("owner")
		repo  = r.FormValue("repo")
	)

	cl, err := h.github.UserGithubClient(r.Context(), data.User.GetLogin())
	if err != nil {
		h.doError(errors.Wrap(err, "failed getting user client"), w, r)
	}

	repoData, _, err := cl.Repositories.Get(r.Context(), owner, repo)
	if err != nil {
		h.doError(errors.Wrap(err, "failed getting repository data"), w, r)
		return
	}
	logrus.Info("Running goreadme in background...")
	_, jobNum := h.runJob(r.Context(), &Project{
		Owner:         owner,
		Repo:          repo,
		Install:       data.InstallID,
		DefaultBranch: repoData.GetDefaultBranch(),
	})
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
		h.doError(errors.Wrap(err, "failed executing template"), w, r)
	}
}

func (h *handler) doError(err error, w http.ResponseWriter, r *http.Request) {
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
	done, _ := h.runJob(context.Background(), &Project{
		Owner:         os.Getenv("OWNER"),
		Repo:          os.Getenv("REPO"),
		HeadSHA:       os.Getenv("HEAD"),
		DefaultBranch: "master",
	})
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
