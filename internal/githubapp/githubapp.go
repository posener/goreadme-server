package githubapp

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
)

var defaultHeader = &jws.Header{Algorithm: "RS256", Typ: "JWT"}

// Config is the configuration for using JWT to fetch tokens.
type Config struct {
	AppID      string
	PrivateKey []byte
	Expires    time.Duration
}

type GithubClients struct {
	cfg       Config
	appGithub *github.Client
	// user clients are stored in cache.
	cache     *cache.Cache
	mu        sync.RWMutex
}

type User struct {
	Client    *http.Client
	Github    *github.Client
	InstallID int64
}

// Clients returns a struct that can provide user installation clients
// for the given application.
func (c *Config) Clients(ctx context.Context) *GithubClients {
	cl := oauth2.NewClient(ctx, oauth2.ReuseTokenSource(nil, jwtSource{
		ctx:     ctx,
		appID:   c.AppID,
		expires: c.Expires,
		pk:      parseKey(c.PrivateKey),
	}))
	return &GithubClients{
		cfg:       *c,
		appGithub: github.NewClient(cl),
		cache:     cache.New(5*time.Minute, 10*time.Minute),
	}
}

// User returns github installation client for a given user login.
func (c *GithubClients) User(ctx context.Context, login string) (*User, error) {
	if login == "" {
		return nil, fmt.Errorf("empty login")
	}
	c.mu.RLock()
	if user, ok := c.cache.Get(login); ok {
		c.mu.RUnlock()
		return user.(*User), nil
	}
	c.mu.RUnlock()
	logrus.Debugf("Creating new client for user: %s", login)

	inst, _, err := c.appGithub.Apps.FindUserInstallation(ctx, login)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting user installation")
	}

	appID, _ := strconv.Atoi(c.cfg.AppID)
	tr, err := ghinstallation.New(http.DefaultTransport, appID, int(inst.GetID()), c.cfg.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "get install transport")
	}

	cl := &http.Client{Transport: tr}
	user := &User{
		Client:    cl,
		Github:    github.NewClient(cl),
		InstallID: inst.GetID(),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.SetDefault(login, user)
	return user, nil
}

// jwtSource is a source that always does a signed JWT request for a token.
// It should typically be wrapped with a reuseTokenSource.
type jwtSource struct {
	ctx     context.Context
	appID   string
	expires time.Duration
	pk      *rsa.PrivateKey
}

func (js jwtSource) Token() (*oauth2.Token, error) {
	exp := time.Now().Add(js.expires)
	claimSet := &jws.ClaimSet{Iss: js.appID, Exp: exp.Unix()}
	h := *defaultHeader
	payload, err := jws.Encode(&h, claimSet, js.pk)
	if err != nil {
		return nil, err
	}
	logrus.Infof("Using new application token")
	return &oauth2.Token{TokenType: "bearer", AccessToken: payload, Expiry: exp}, nil
}

func parseKey(key []byte) *rsa.PrivateKey {
	block, _ := pem.Decode(key)
	if block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			panic(fmt.Errorf("private key should be a PEM or plain PKCS1 or PKCS8; parse error: %v", err))
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		panic("private key is invalid")
	}
	return parsed
}
