package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/posener/goreadme"
	"github.com/sirupsen/logrus"
)

// hook is called by github when there is a push to repository.
func (h *handler) hook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(cfg.GithubHookSecret))
	if err != nil {
		logrus.Warnf("Unauthorized request: %s", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Handle different events
	if e := tryPush(payload); e != nil {
		logrus.Info("Push hook triggered")
		if branchOfRef(e.GetRef()) != e.GetRepo().GetDefaultBranch() {
			logrus.Infof("Skipping push to non default branch %q", e.GetRef())
			return
		}
		if e.GetInstallation().GetAppID() == int64(cfg.GithubAppID) {
			logrus.Infof("Skipping self push")
			return
		}
		h.runJob(r.Context(), &Project{
			Install: e.GetInstallation().GetID(),
			Owner:   e.GetRepo().GetOwner().GetName(),
			Repo:    e.GetRepo().GetName(),
			HeadSHA: e.GetHeadCommit().GetID(),
		})
	} else if e := tryInstall(payload); e != nil {
		logrus.Infof("Install hook triggered added=%d removed=%d", len(e.RepositoriesAdded), len(e.RepositoriesRemoved))
		for _, repo := range e.RepositoriesRemoved {
			logrus.Infof("Removed of %s", repo.GetFullName())
		}
		for _, repo := range e.RepositoriesAdded {
			parts := strings.Split(repo.GetFullName(), "/")
			h.runJob(r.Context(), &Project{
				Install: e.GetInstallation().GetID(),
				Owner:   parts[0],
				Repo:    parts[1],
			})
		}
	} else if e := tryPullRequest(payload); e != nil {
		if e.GetAction() != "closed" || !e.GetPullRequest().GetMerged() {
			logrus.Info("Skipping non-merge PR")
			return
		}
		if ref := e.GetPullRequest().GetBase().GetRef(); ref != e.GetRepo().GetDefaultBranch() {
			logrus.Infof("Skipping merge to non-default branch: %s", ref)
			return
		}
		h.runJob(r.Context(), &Project{
			Install:       e.GetInstallation().GetID(),
			Owner:         e.GetRepo().GetOwner().GetLogin(),
			Repo:          e.GetRepo().GetName(),
			DefaultBranch: e.GetRepo().GetDefaultBranch(),
		})
	} else {
		logrus.Warnf("Got unexpected payload: %s", string(payload))
	}
}

func tryPush(payload []byte) *github.PushEvent {
	var e github.PushEvent
	err := json.Unmarshal(payload, &e)
	if err != nil {
		logrus.Errorf("Failed decoding push event: %s", err)
		return nil
	}
	if e.Repo == nil {
		return nil
	}
	return &e
}

func tryInstall(payload []byte) *github.InstallationRepositoriesEvent {
	var e github.InstallationRepositoriesEvent
	err := json.Unmarshal(payload, &e)
	if err != nil {
		logrus.Errorf("Failed decoding push event: %s", err)
		return nil
	}
	if len(e.RepositoriesRemoved) == 0 && len(e.RepositoriesAdded) == 0 {
		return nil
	}
	return &e
}

func tryPullRequest(payload []byte) *github.PullRequestEvent {
	var e github.PullRequestEvent
	err := json.Unmarshal(payload, &e)
	if err != nil {
		logrus.Errorf("Failed decoding push event: %s", err)
		return nil
	}
	if e.PullRequest == nil {
		return nil
	}
	return &e
}

func (h *handler) runJob(ctx context.Context, p *Project) (done <-chan struct{}, jobNum int, err error) {
	user, err := h.github.User(ctx, p.Owner)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed getting user client: %s")
	}

	repo, _, err := user.Github.Repositories.Get(ctx, p.Owner, p.Repo)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed getting repo data")
	}
	p.DefaultBranch = repo.GetDefaultBranch()
	p.Private = repo.GetPrivate()
	p.Stars = repo.GetStargazersCount()

	// Update Head SHA if was not given.
	if p.HeadSHA == "" {
		gitData, _, err := user.Github.Git.GetRef(ctx, p.Owner, p.Repo, "refs/heads/"+p.DefaultBranch)
		if err != nil {
			return nil, 0, errors.Wrap(err, "failed getting git data")
		}
		p.HeadSHA = gitData.GetObject().GetSHA()
	}

	j := &Job{
		Project:  *p,
		db:       h.db,
		github:   user.Github,
		goreadme: goreadme.New(user.Client),
	}
	done, jobNum = j.Run()
	return done, jobNum, nil
}
