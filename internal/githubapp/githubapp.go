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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
)

var defaultHeader = &jws.Header{Algorithm: "RS256", Typ: "JWT"}

// Config is the configuration for using JWT to fetch tokens,
// commonly known as "two-legged OAuth 2.0".
type Config struct {
	AppID      string
	PrivateKey []byte
	Expires    time.Duration
}

func (c *Config) Clients(ctx context.Context) *GithubClients {
	return &GithubClients{
		cfg:          *c,
		client:       oauth2.NewClient(ctx, oauth2.ReuseTokenSource(nil, jwtSource{ctx, c})),
		userClients:  make(map[string]*http.Client),
		userInstalls: make(map[string]int64),
	}
}

type GithubClients struct {
	cfg          Config
	client       *http.Client
	userClients  map[string]*http.Client
	userInstalls map[string]int64
	mu           sync.RWMutex
}

func (c *GithubClients) Client() *http.Client {
	return c.client
}

func (c *GithubClients) GithubClient() *github.Client {
	return github.NewClient(c.client)
}

func (c *GithubClients) InstallID(ctx context.Context, user string) (int64, error) {
	c.mu.RLock()
	if id := c.userInstalls[user]; id != 0 {
		c.mu.RUnlock()
		return id, nil
	}
	c.mu.RUnlock()

	_, err := c.UserClient(ctx, user)
	if err != nil {
		return 0, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userInstalls[user], nil
}

func (c *GithubClients) UserClient(ctx context.Context, user string) (*http.Client, error) {
	c.mu.RLock()
	if cl := c.userClients[user]; cl != nil {
		c.mu.RUnlock()
		return cl, nil
	}
	c.mu.RUnlock()
	logrus.Debugf("Creating new client for user: %s", user)

	inst, _, err := github.NewClient(c.client).Apps.FindUserInstallation(ctx, user)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting user installation")
	}

	logrus.Debugf("Got installation %+v for client %s", inst, user)

	appID, _ := strconv.Atoi(c.cfg.AppID)
	tr, err := ghinstallation.New(http.DefaultTransport, appID, int(inst.GetID()), c.cfg.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "get install transport")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.userClients[user] = &http.Client{Transport: tr}
	c.userInstalls[user] = inst.GetID()
	return c.userClients[user], nil
}

func (c *GithubClients) UserGithubClient(ctx context.Context, user string) (*github.Client, error) {
	cl, err := c.UserClient(ctx, user)
	if err != nil {
		return nil, err
	}
	return github.NewClient(cl), nil
}

// jwtSource is a source that always does a signed JWT request for a token.
// It should typically be wrapped with a reuseTokenSource.
type jwtSource struct {
	ctx  context.Context
	conf *Config
}

func (js jwtSource) Token() (*oauth2.Token, error) {
	pk, err := parseKey(js.conf.PrivateKey)
	if err != nil {
		return nil, err
	}
	// hc := oauth2.NewClient(js.ctx, nil)
	claimSet := &jws.ClaimSet{
		Iss: js.conf.AppID,
	}
	if t := js.conf.Expires; t > 0 {
		claimSet.Exp = time.Now().Add(t).Unix()
	}
	h := *defaultHeader
	payload, err := jws.Encode(&h, claimSet, pk)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Got payload: %s", string(payload))

	return &oauth2.Token{TokenType: "bearer", AccessToken: payload}, nil
}

func parseKey(key []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("private key should be a PEM or plain PKCS1 or PKCS8; parse error: %v", err)
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is invalid")
	}
	return parsed, nil
}
