# goreadme-server

[![GoDoc](https://godoc.org/github.com/posener/goreadme-server?status.svg)](http://godoc.org/github.com/posener/goreadme-server)
[![goreadme](https://goreadme.herokuapp.com/badge/posener/goreadme-server.svg)](https://goreadme.herokuapp.com)

an HTTP server that works with Github hooks.

[goreadme](github.com/posener/goreadme) is a tool for creating README.md
files from Go doc of a given package.
This project is the Github app on top of this tool. It fully automates
the process of keeping the README.md file updated.

## Usage

1. Go to [[https://github.com/apps/goreadme](https://github.com/apps/goreadme)](https://github.com/apps/goreadme](https://github.com/apps/goreadme)).
2. Press the "Configure" button.
3. Choose your account, or an organization that owns the repository.
4. Review the permissions and provide access to goreadme to repositories.
5. Click Save.

You should see PRs from goreadme bot in your github repos.

For more features, or to trigger goreadme on demand to to
[goreadme site]([https://goreadme.herokuapp.com](https://goreadme.herokuapp.com)).

## How does it Work?

Once integrated with a repository, goreadme is registered on a Github hook,
that calls goreadme server whenever the repository default branch is
modified. Goreadme then computes the new README.md file and compairs it
to the exiting one. If a change is needed, Goreadme will create a PR with
the new content of the README.md file.

Created by [goreadme](https://github.com/apps/goreadme)
