package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/github"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/posener/goreadme"
	"github.com/sirupsen/logrus"
	"github.com/src-d/go-git/plumbing"
)

const (
	githubAppURL      = "https://github.com/apps/goreadme"
	timeout           = time.Second * 60 * 1
	configPath        = "goreadme.json"
	defaultReadmePath = "README.md"

	goreadmeAuthor = "goreadme"
	goreadmeEmail  = "posener@gmail.com"
	goreadmeBranch = "goreadme"
	goreadmeRef    = "refs/heads/" + goreadmeBranch
)

type Project struct {
	// Install is installation ID for authentication purposes.
	Install       int64
	Repo          string `gorm:"primary_key"`
	Owner         string `gorm:"primary_key"`
	LastJob       int
	HeadSHA       string
	PR            int
	Message       string
	Status        string
	DefaultBranch string
	Private bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Job struct {
	Project
	Num      int `gorm:"primary_key"`
	Duration time.Duration
	Debug    string

	db       *gorm.DB
	github   *github.Client
	goreadme *goreadme.GoReadme
	log      logrus.FieldLogger
}

// Run runs the pull request flow
func (j *Job) Run() (done <-chan struct{}, jobNum int) {
	err := j.init()
	if err != nil {
		j.log.Errorf("Failed creating job entry in database: %s", err)
		return nil, 0
	}

	ch := make(chan struct{})
	done = ch
	jobNum = j.Num

	j.log.Infof("Starting PR process")

	go j.runInBackground(ch)
	return done, jobNum
}

func (j *Job) runInBackground(done chan<- struct{}) {
	defer close(done)
	start := time.Now()

	b := bytes.NewBuffer(nil)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	saveError := func(err error, format string, args ...interface{}) {
		s := fmt.Sprintf(format, args...)
		j.log.WithError(err).Error(s)
		j.Debug = err.Error()
		j.Message = s
		j.Status = "Failed"
		j.Duration = time.Now().Sub(start)
		if err := j.db.Save(j).Error; err != nil {
			j.log.Errorf("Failed saving failed job: %s", err)
		}
		j.updateProject()
	}

	saveSuccess := func(format string, args ...interface{}) {
		s := fmt.Sprintf(format, args...)
		j.log.Info(s)
		j.Message = s
		j.Status = "Success"
		j.Duration = time.Now().Sub(start)
		err := j.db.Save(j).Error
		if err != nil {
			j.log.Errorf("Failed saving successful job: %s", err)
		}
		j.updateProject()
	}

	// Get config
	cfg, err := j.getConfig(ctx)
	if err != nil {
		saveError(err, "Failed getting config")
		return
	}

	// Create new readme for repository.
	err = j.goreadme.WithConfig(cfg).Create(ctx, j.githubURL(), b)
	if err != nil {
		saveError(err, "Failed running goreadme: %s", err)
		return
	}
	b.WriteString(credits)

	// Check for changes from current readme
	readmePath := defaultReadmePath
	readme, resp, err := j.github.Repositories.GetReadme(ctx, j.Owner, j.Repo, nil)
	var currentContent string
	switch {
	case resp.StatusCode == http.StatusNotFound:
		j.log.Infof("No current readme, creating a new readme!")
	case err != nil:
		saveError(err, "Failed reading current readme")
		return
	default:
		currentContent, err = readme.GetContent()
		if err != nil {
			saveError(err, "Failed get readme content")
			return
		}
		if currentContent == b.String() {
			saveSuccess("Current readme is up to date")
			return
		}
		readmePath = readme.GetPath()
	}

	var sha string

	// Reset goreadme branch - delete it if exists and then create it.
	_, resp, err = j.github.Repositories.GetBranch(ctx, j.Owner, j.Repo, goreadmeBranch)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		// Branch does not exist, create it
		j.log.Infof("Creating new branch")
		_, _, err = j.github.Git.CreateRef(ctx, j.Owner, j.Repo, &github.Reference{
			Ref:    github.String(goreadmeRef),
			Object: &github.GitObject{SHA: github.String(j.HeadSHA)},
		})
		if err != nil {
			saveError(err, "Failed creating %q ref", goreadmeRef)
			return
		}
		sha = plumbing.ComputeHash(plumbing.BlobObject, []byte(currentContent)).String()
	case err != nil:
		saveError(err, "Failed getting %q branch", goreadmeBranch)
		return
	default:
		// Branch exist, delete it
		j.log.Infof("Found existing branch")
		sha = plumbing.ComputeHash(plumbing.BlobObject, b.Bytes()).String()
	}

	// Commit changes to readme file.
	date := time.Now()
	author := &github.CommitAuthor{
		Name:  github.String(goreadmeAuthor),
		Email: github.String(goreadmeEmail),
		Date:  &date,
	}
	_, _, err = j.github.Repositories.UpdateFile(ctx, j.Owner, j.Repo, readmePath, &github.RepositoryContentFileOptions{
		Author:    author,
		Committer: author,
		Branch:    github.String(goreadmeBranch),
		Content:   b.Bytes(),
		Message:   github.String("Update readme according to go doc"),
		SHA:       github.String(sha),
	})
	if err != nil {
		saveError(err, "Failed updating readme content")
		return
	}

	prs, resp, err := j.github.PullRequests.List(ctx, j.Owner, j.Repo, &github.PullRequestListOptions{Head: goreadmeBranch})
	if err != nil {
		saveError(err, "Failed listing PRs")
		return
	}
	if len(prs) == 0 {
		// No pr exists, create a new one.
		j.log.Infof("Creating a new PR")
		pr, _, err := j.github.PullRequests.Create(ctx, j.Owner, j.Repo, &github.NewPullRequest{
			Title: github.String("readme: Update according to go doc"),
			Base:  github.String(j.DefaultBranch),
			Head:  github.String(goreadmeBranch),
		})
		if err != nil {
			saveError(err, "Failed creatring PR")
			return
		}
		j.PR = pr.GetNumber()
		saveSuccess("Created PR")
		return
	}

	if len(prs) > 1 {
		j.log.Warnf("Found %s ambiguous prs, using the first", len(prs))
	}
	j.PR = prs[0].GetNumber()
	saveSuccess("PR updated")

}

func (j *Job) updateProject() {
	tx := j.db.Begin()
	var currentProject Project
	query := tx.Model(Project{}).Where("owner = ? AND repo = ?", j.Owner, j.Repo).First(&currentProject)
	if err := query.Error; !query.RecordNotFound() && err != nil {
		j.log.Errorf("Failed querying for existing project: %s", err)
		tx.Rollback()
		return
	}
	if currentProject.LastJob > j.LastJob {
		j.log.Infof("Skipping update project due to newer version")
		tx.Rollback()
		return
	}
	err := tx.Save(&j.Project).Error
	if err != nil {
		j.log.Errorf("Failed saving new project: %s", err)
		tx.Rollback()
		return
	}
	tx.Commit()
}

func (j *Job) getConfig(ctx context.Context) (goreadme.Config, error) {
	var cfg goreadme.Config
	cfgContent, _, resp, err := j.github.Repositories.GetContents(ctx, j.Owner, j.Repo, configPath, nil)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return cfg, nil
	case err != nil:
		return cfg, errors.Wrap(err, "failed get config file")
	}
	content, err := cfgContent.GetContent()
	if err != nil {
		return cfg, errors.Wrap(err, "failed get config content")
	}
	err = json.Unmarshal([]byte(content), &cfg)
	if err != nil {
		return cfg, errors.Wrapf(err, "unmarshaling config content %s", content)
	}
	return cfg, nil
}

func (j *Job) init() error {
	tx := j.db.Begin()

	var maxNum struct{ Num int }
	err := tx.Table("jobs").Select("MAX(num) as num").Where("owner = ? AND repo = ?", j.Owner, j.Repo).First(&maxNum).Error
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "get max job")
	}
	j.Num = maxNum.Num + 1
	j.LastJob = j.Num
	j.Status = "Started"
	j.log = logrus.WithFields(logrus.Fields{
		"sha": shortSHA(j.HeadSHA),
		"job": fmt.Sprintf("%s/%s#%d", j.Owner, j.Repo, j.Num),
	})
	err = tx.Create(j).Error
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "creating job")
	}
	err = tx.Save(&j.Project).Error
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "saving project")
	}
	return tx.Commit().Error
}

func (j *Job) setNextNum() error {
	return nil
}

func (j *Job) githubURL() string {
	return "github.com/" + j.Owner + "/" + j.Repo
}

func shortSHA(sha string) string {
	if len(sha) < 8 {
		return sha
	}
	return sha[:8]
}

const credits = "\nCreated by [goreadme](" + githubAppURL + ")\n"
