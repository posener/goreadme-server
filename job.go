package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	Private       bool
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
	start    time.Time
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

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Get config
	cfg, err := j.getConfig(ctx)
	if err != nil {
		j.done(err, "Failed getting config")
		return
	}

	// Create new readme for repository.
	newContent := bytes.NewBuffer(nil)
	err = j.goreadme.WithConfig(cfg).Create(ctx, j.githubURL(), newContent)
	if err != nil {
		j.done(err, "Failed running goreadme: %s", err)
		return
	}
	newContent.WriteString(credits)
	newSHA := computeSHA(newContent.Bytes())

	// Check for changes from current readme
	defaultBranchSHA, readmePath, err := j.remoteReadme(ctx, j.DefaultBranch)
	if err != nil {
		j.done(err, "Failed getting github README content")
		return
	}

	// Check if there are any changes from HEAD.
	if defaultBranchSHA == newSHA {
		j.done(nil, "Readme in branch %s is up to date", j.DefaultBranch)
		return
	}

	// Reset goreadme branch - delete it if exists and then create it.
	err = j.createBranch(ctx)
	if err != nil {
		j.done(err, "Failed creating branch")
		return
	}
	
	sha, _, err := j.remoteReadme(ctx, goreadmeBranch)
	if err != nil {
		j.done(err, "Failed get remote readme SHA")
		return
	}

	// Check if the goreadme readme file is the same as the new one.
	if sha == newSHA {
		j.log.Infof("Readme in branch %s is up to date, making sure PR is open", goreadmeBranch)
	} 
	
	// Commit changes to readme file.
	err = j.commit(ctx, readmePath, newContent.Bytes(), sha)
	if err != nil {
		j.done(err, "Failed pushing readme content")
		return
	}

	prNum, createdNewPR, err := j.pullRequest(ctx)
	if err != nil {
		j.done(err, "Failed creating PR")
		return
	}
	j.PR = prNum
	message := "PR updated"
	if createdNewPR {
		message = "Created PR"
	}
	j.done(nil, message)

}

// done saves the job and project state once it is done.
func (j *Job) done(err error, format string, args ...interface{}) {
	j.Message = fmt.Sprintf(format, args...)
	j.Status = "Success"
	j.Duration = time.Now().Sub(j.start)
	if err != nil {
		j.Status = "Failed"
		j.Debug = err.Error()
		j.log.WithError(err).Error(j.Message)
	}
	if err := j.db.Save(j).Error; err != nil {
		j.log.Errorf("Failed saving %s job: %s", strings.ToLower(j.Status), err)
	}
	j.saveProject()
}

// updateProject saves the project data if it is the latest.
func (j *Job) saveProject() {
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

// remoteReadme returns the SHA of the remote README file and its path.
func (j *Job) remoteReadme(ctx context.Context, branch string) (remoteSHA, readmePath string, err error) {
	readme, resp, err := j.github.Repositories.GetReadme(ctx, j.Owner, j.Repo, &github.RepositoryContentGetOptions{Ref: branch})
	var upstreamContent string
	switch {
	case resp.StatusCode == http.StatusNotFound:
		j.log.Infof("No current readme, creating a new readme!")
		return "", defaultReadmePath, nil
	case err != nil:
		return "", "", errors.Wrap(err, "failed reading current readme")
	default:
		upstreamContent, err = readme.GetContent()
		if err != nil {
			return "", "", errors.Wrap(err, "failed get readme content")
		}
		return computeSHA([]byte(upstreamContent)), readme.GetPath(), nil
	}
}

// createBranch gets existing goreadme branch or creates a new goreadme branch.
func (j *Job) createBranch(ctx context.Context) error {
	_, resp, err := j.github.Repositories.GetBranch(ctx, j.Owner, j.Repo, goreadmeBranch)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		// Branch does not exist, create it
		j.log.Infof("Creating new branch")
		_, _, err = j.github.Git.CreateRef(ctx, j.Owner, j.Repo, &github.Reference{
			Ref:    github.String(goreadmeRef),
			Object: &github.GitObject{SHA: github.String(j.HeadSHA)},
		})
		if err != nil {
			return errors.Wrapf(err, "failed creating %q ref", goreadmeRef)
		}
		return nil
	case err != nil:
		return errors.Wrapf(err, "Failed getting %q branch", goreadmeBranch)
	default:
		// Branch exist, delete it
		j.log.Infof("Found existing branch")
		return nil
	}
}

// commit upload the file content to the goreadme branch.
func (j *Job) commit(ctx context.Context, readmePath string, content []byte, sha string) error {
	date := time.Now()
	author := &github.CommitAuthor{
		Name:  github.String(goreadmeAuthor),
		Email: github.String(goreadmeEmail),
		Date:  &date,
	}
	_, _, err := j.github.Repositories.UpdateFile(ctx, j.Owner, j.Repo, readmePath, &github.RepositoryContentFileOptions{
		Author:    author,
		Committer: author,
		Branch:    github.String(goreadmeBranch),
		Content:   content,
		Message:   github.String("Update readme according to go doc"),
		SHA:       github.String(sha),
	})
	return err
}

// pullRequest return a current open pull request or create a new pull request and returns it.
func (j *Job) pullRequest(ctx context.Context) (prNum int, created bool, err error) {
	prs, _, err := j.github.PullRequests.List(ctx, j.Owner, j.Repo, &github.PullRequestListOptions{
		Base: j.DefaultBranch,
	})
	if err != nil {
		return 0, false, errors.Wrap(err, "Failed listing PRs")
	}
	for _, pr := range prs {
		if pr.Head.GetRef() == goreadmeBranch {
			return pr.GetNumber(), false, nil
		}
	}

	// No pr exists, create a new one.
	j.log.Infof("Creating a new PR")
	pr, _, err := j.github.PullRequests.Create(ctx, j.Owner, j.Repo, &github.NewPullRequest{
		Title: github.String("readme: Update according to go doc"),
		Base:  github.String(j.DefaultBranch),
		Head:  github.String(goreadmeBranch),
	})
	if err != nil {
		return 0, false, errors.Wrap(err, "Failed creatring PR")
	}
	return pr.GetNumber(), true, nil
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
	j.start = time.Now()
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

func computeSHA(b []byte) string {
	return plumbing.ComputeHash(plumbing.BlobObject, b).String()
}

const credits = "\nCreated by [goreadme](" + githubAppURL + ")\n"
