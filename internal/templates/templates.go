package templates

import (
	"html/template"
	"time"

	prettytime "github.com/andanhm/go-prettytime"
	"github.com/hako/durafmt"
)

var html = template.Must(
	template.New("html").Funcs(
		template.FuncMap{
			"formatDate": func(t time.Time) string {
				return prettytime.Format(t)
			},
			"formatDuration": func(d time.Duration) string {
				return durafmt.ParseShort(d).String()
			},
			"sha": func(sha string) string {
				if len(sha) < 8 {
					return sha
				}
				return sha[:8]
			},
			"color": func(status string) string {
				switch status {
				case "Failed":
					return "danger"
				case "Success":
					return "success"
				default:
					return "warning"
				}
			},
		}).Parse(`
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta http-equiv="X-UA-Compatible" content="IE=edge">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Goreadme</title>
  <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.3.1/css/bootstrap.min.css" integrity="sha384-ggOyR0iXCbMQv3Xipma34MD+dH/1fQ784/j6cY/iJTQUOhcWr7x9JvoRxT2MZw1T" crossorigin="anonymous">
  <link href="https://maxcdn.bootstrapcdn.com/font-awesome/4.7.0/css/font-awesome.min.css" rel="stylesheet">
  <link rel="shortcut icon" type="image/png" href="https://raw.githubusercontent.com/posener/goreadme-server/master/media/favicon.ico"/>
</head>
<body>

{{template "body" .}}

  <script src="https://code.jquery.com/jquery-3.3.1.slim.min.js" integrity="sha384-q8i/X+965DzO0rT7abK41JStQIAqVgRVzpbzo5smXKp4YfRvH+8abtTE1Pi6jizo" crossorigin="anonymous"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.14.7/umd/popper.min.js" integrity="sha384-UO2eT0CpHqdSJQ6hJty5KVphtPhzWj9WO1clHTMGa3JDZwrnQq4sF86dIHNDz0W1" crossorigin="anonymous"></script>
  <script src="https://stackpath.bootstrapcdn.com/bootstrap/4.3.1/js/bootstrap.min.js" integrity="sha384-JjSmVgyd0p3pXB1rRibZUAYoIIy6OrQ6VrjIEaFf/nJGzIxFDsf4x0xIM+B07jRM" crossorigin="anonymous"></script>
  {{if .Error}}
  <script>$('.alert').alert()</script>
  {{end}}
  
  <!-- Global site tag (gtag.js) - Google Analytics -->
  <script async src="/analytics/gtag/js?id=UA-119938419-2"></script>
  <script>
    window.dataLayer = window.dataLayer || [];
    function gtag(){dataLayer.push(arguments);}
    gtag('js', new Date());

    gtag('config', 'UA-119938419-2');
  </script>

</body>
</html>
`))

var base = template.Must(html.Parse(`
{{define "body"}}
<nav class="navbar navbar-expand-md navbar-light bg-light">
	<a class="navbar-brand abs" href="/">
		<img src="https://raw.githubusercontent.com/posener/goreadme-server/master/media/icon.png" width="30" height="30" alt="">
		Goreadme
	</a>
	{{ if .User }}
		<button class="navbar-toggler" type="button" data-toggle="collapse" data-target="#collapsingNavbar">
			<span class="navbar-toggler-icon"></span>
		</button>
		<div class="navbar-collapse collapse" id="collapsingNavbar">
			<ul class="navbar-nav">
				<li class="nav-item {{if .Projects}}active{{end}}">
					<a class="nav-link" href="/projects">
						<i class="fa fa-book" aria-hidden="true"></i>
						Projects
					</a>
				</li>
				<li class="nav-item {{if .Jobs}}active{{end}}">
					<a class="nav-link" href="/jobs">
						<i class="fa fa-history" aria-hidden="true"></i>
						History
					</a>
				</li>
				<li class="nav-item {{if .Repos}}active{{end}}">
					<a class="nav-link" href="/add">
						<i class="fa fa-play-circle" aria-hidden="true"></i>
						Integrations
					</a>
				</li>
				<li class="nav-item">
					<a class="nav-link" href="https://github.com{{if .InstallID}}/settings/installations/{{.InstallID}}{{else}}/apps/goreadme/installations/new{{end}}">
						<i class="fa fa-wrench" aria-hidden="true"></i>
						Manage Integration
					</a>
				</li>
			</ul>
			<ul class="navbar-nav ml-auto">
				<li class="nav-item dropdown">
					<a class="nav-link dropdown-toggle" href="#" id="navbarDropdown" role="button" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
						<img src="{{.User.GetAvatarURL}}" width="30" height="30" class="d-inline-block align-top" alt="">
						{{.User.GetLogin}}
					</a>
					<div class="dropdown-menu" aria-labelledby="navbarDropdown">
						<a class="dropdown-item" href="{{.User.GetHTMLURL}}">
							<i class="fa fa-github" aria-hidden="true"></i>
							Github page
						</a>
						<a class="dropdown-item" href="/auth/logout">
							<i class="fa fa-sign-out" aria-hidden="true"></i>
							Logout
						</a>
					</div>
				</li>
			</ul>
		</div>
	{{ end }}
</nav>

	<div class="container p-4">

	{{ if .Error }}
		<div class="alert alert-danger alert-dismissible fade show" role="alert">
			{{.Error}}
			<button type="button" class="close" data-dismiss="alert" aria-label="Close">
				<span aria-hidden="true">&times;</span>
			</button>
		</div>
	{{ end }}

	{{template "content" .}}

	</div>
	
	</div>

	<div class="container-fluid p-3 p-md-5 bg-light">
		<ul class="list-inline">
			<li class="list-inline-item">
				Runs free on <a href="">Heroku</a>
			</li>
			<li class="list-inline-item">
				Stored free on <a href="https://github.com">Github</a>
			</li>
			<li class="list-inline-item">
				Written in free <a href="https://golang.org">Go</a>
			</li>
			<li class="list-inline-item">
				Using free <a href="https://getbootstrap.com">Bootstrap</a>
			</li>
			<li class="list-inline-item">
				Using free <a href="https://fontawesome.com">Font Awesome</a>
			</li>
		</ul>
		<p>Designed anb built by Eyal Posener / 2019</p>
		<ul class="list-inline">
			<li class="list-inline-item"><a href="https://github.com/posener/goreadme">
				<i class="fa fa-github" aria-hidden="true"></i>
				goreadme
			</a></li>
			<li class="list-inline-item"><a href="https://github.com/posener/goreadme-server">
				<i class="fa fa-github" aria-hidden="true"></i>
				goreadme-server
			</a></li>
			<li class="list-inline-item"><a href="https://github.com/posener/goreadme-server/blob/master/LICENSE.txt">
				<i class="fa fa-id-card-o" aria-hidden="true"></i>
				MIT
			</a></li>
			<li class="list-inline-item"><a href="https://github.com/posener/goreadme-server/issues">
				<i class="fa fa-bug" aria-hidden="true"></i>
				Report a Bug
			</a></li>
		</ul>
  	</div>

{{end}}
`))

var Home = template.Must(template.Must(base.Clone()).Parse(`
{{define "title"}}Login{{end}}
{{define "content"}}
<div class="row">
	<div class="col-lg-7 col-12 mx-auto">
		<h4>Welcome</h4>
		<p>
			Goreadme is a service that automatically creates and updates readme
			files for Go Github projects from the Go doc of the project.
		</p>

		<h5>Usage</h5>
		<ol>
			<li>
				Go to <a target="_blank" href="https://github.com/apps/goreadme">Goreadme Github App page</a>.
			</li>
			<li>Press the "Configure" button.</li>
			<li>Choose your account, or an organization that owns the repository.</li>
			<li>Review the permissions and provide access to goreadme to repositories.</li>
			<li>Click Save.</li>
		</ol>
		<p>
			You should see PRs from goreadme bot in your Github repositories.
		</p>
		<h5>How does it Work?</h5>
		<p>
			Once integrated with a repository, goreadme is registered on a Github hooks,
			that calls goreadme server whenever the repository default branch is
			modified. Goreadme then computes the new readme file and compairs it
			to the exiting one. If a change is needed, Goreadme will create a PR with
			the new content of the README.md file.
			Genrating the readme file can also be triggered manually <a href="/projects">here</a>.
		</p>
		<p>
			Goreadme service uses <a href="github.com/posener/goreadme">goreadme</a> - is a tool
			created by the service author, for generating README.md files from Go doc of a given package.
		</p>
		<h5>Customization</h5>
		<p>
			Adding a <code>goreadme.json</code> file to your repository main directory can enable some
			customization to the generated readme file. The configuration is available
			according to <a href="https://godoc.org/github.com/posener/goreadme#Config"><code>goreadme.Config</code></a>
		</p>
	</div>
	<div class="col-lg-5 col-12">

	{{ if not .User }}
		<div class="jumbotron text-center">
			<h4>Login</h4>
			<p>
				In order to use goreadme with your Github repositories, login is required.
			</p>
			<form action="/auth/login">
			<button type="submit" class="btn btn-outline-primary">
				<i class="fa fa-x2 fa-github" aria-hidden="true"></i>
				Login with Github
			</button>
			</form>
		</div>
	{{ end }}

		<div class="card">
			<div class="card-body">
				<h4 class="card-title">
					Stats
				</h4>
				<h5 class="card-subtitle p-2 text-muted">
					<i class="fa fa-x2 fa-balance-scale"></i>
					Total: {{.Stats.TotalProjects}}
				</h5>
				<h5 class="card-subtitle p-2 text-muted">
					<i class="fa fa-x2 fa-trophy"></i>
					Top Open Source Goreadmes
				</h5>
				<ul class="list-group">
				{{ range .Stats.TopProjects }}
					<a href="https://github.com/{{.Owner}}/{{.Repo}}" class="list-group-item d-flex justify-content-between align-items-center">
						{{.Owner}}/{{.Repo}}
						<span class="badge badge-info">{{.Stars}} <i class="fa fa-star"></i></span>
					</a>
				{{ end }}
				</ul>
			</div>
		</div>
	</div>

</div>
{{end}}
`))

var headline = template.Must(base.Parse(`
{{ define "headline" }}
<div class="row row border-top rounded-sm bg-light">

<div class="col-8 p-2 pl-3">
	<a href="/jobs?owner={{.Owner}}&repo={{.Repo}}"><i class="fa fa-filter" aria-hidden="true"></i></a>
	<a href="https://github.com/{{.Owner}}/{{.Repo}}"><i class="fa fa-github" aria-hidden="true"></i></a>
	{{.Owner}}/{{.Repo}}
</div>

<div class="col-3 p-2 pl-2">
	<div class="text-{{ color .Status }}">{{.Status}}</div>
	{{if .PR}}
	<div>
		<small><a href="https://github.com/{{.Owner}}/{{.Repo}}/pull/{{.PR}}">PR#{{.PR}}</a></small>
	</div>
	{{end}}
</div>

<div class="col-1 p-2">
	<form action="/add" method="post" class="float-right">
		<input type="hidden" name="repo" value="{{.Repo}}">
		<input type="hidden" name="owner" value="{{.Owner}}">
		<button type="submit" class="btn btn-outline-primary btn-sm">
			<i class="fa fa-play-circle" aria-hidden="true"></i>
		</button>
	</form>
</div>

</div>
{{ end }}
`))

var branch = template.Must(base.Parse(`
{{ define "branch" }}
<div>
	<a href="https://github.com/{{.Owner}}/{{.Repo}}/tree/{{.DefaultBranch}}">{{.DefaultBranch}}</a>
</div>
<div><small>
	<a href="https://github.com/{{.Owner}}/{{.Repo}}/commits/{{.HeadSHA}}">{{sha .HeadSHA}}</a>
</small></div>

{{ end }}
`))

var message = template.Must(base.Parse(`
{{ define "message" }}

<i class="fa fa-quote-left fa-1x fa-pull-left fa-border" aria-hidden="true"></i>
<small>{{.Message}}</small>

{{ end }}
`))

var projectRow = template.Must(base.Parse(`
{{ define "projectRow" }}
<div class="row">
<div class="col-12">

{{ template "headline" . }}

<div class="row mt-md-2">
	<div class="col-md-2 col-6 p-2">
		{{ template "branch" . }}
	</div>
	<div class="col-md-2 col-6 p-2">
		<div>
			<i class="fa fa-calendar" aria-hidden="true"></i>
			{{formatDate .UpdatedAt}}
		</div>
		<div><small>
			<i class="fa fa-hashtag" aria-hidden="true"></i>
			{{.LastJob}}
		</small></div>	
	</div>

	<div class="col-md-8 col-12 p-2 pl-3 pr-3 p-lg-2">
		{{ template "message" . }}
	</div>

</div>

</div>
</div>
{{ end }}
`))

var jobRow = template.Must(base.Parse(`
{{ define "jobRow" }}
<div class="row">
<div class="col-12">

{{ template "headline" . }}

<div class="row mt-md-2">

	<div class="col-md-3 col-6">
		{{ template "branch" . }}
	</div>

	<div class="col-md-3 col-6 p-2">
		<div>
			<i class="fa fa-hashtag" aria-hidden="true"></i>
			{{.Num}}
		</div>
		<div>
			<i class="fa fa-calendar" aria-hidden="true"></i>
			{{formatDate .UpdatedAt}}
		</div>
		<div>
			<i class="fa fa-clock-o" aria-hidden="true"></i>
			{{formatDuration .Duration}}
		</div>
	</div>

	<div class="col-md-3 col-12 p-2 pl-3 pr-3 p-lg-2">
		{{ template "message" . }}
	</div>

</div>

</div>
</div>
{{ end }}
`))

var Projects = template.Must(template.Must(base.Clone()).Parse(`
{{define "title"}}Projects{{end}}
{{define "content"}}
<div class="row m-md-2 justify-content-md-center">
<div class="col-xl-8 col-lg-10 col-12">
{{if .Projects}}
		{{ range .Projects }}

		{{ template "projectRow" . }}

		{{ end }}
{{else}}
	No readmes. Please <a href="/add">add a repository</a>.
{{end}}
</div>
</div>
{{end}}
`))

var AddRepo = template.Must(template.Must(base.Clone()).Parse(`
{{define "title"}}View Installed Repositories{{end}}
{{define "content"}}
{{if .Repos}}
<div class="row">
<div class="col-lg-6">
<table class="table">
{{ range .Repos }}
<tr>
	<td>
			{{.GetFullName}}
	</td>
	<td>
		<form action="/add" method="post">
			<input type="hidden" name="repo" value="{{.GetName}}">
			<input type="hidden" name="owner" value="{{.GetOwner.GetLogin}}">
			<button type="submit" class="btn btn-outline-primary btn-sm">
				<i class="fa fa-play-circle" aria-hidden="true"></i>
			</button>
		</form>
	</td>
</tr>
{{ end }}
</table>
</div>
</div>
{{else}}
No installed repositories. Please <a href="/add">add a repository</a>.
{{end}}
{{end}}
`))

var JobsList = template.Must(template.Must(base.Clone()).Parse(`
{{define "title"}}Jobs List{{end}}
{{define "content"}}

<div class="row m-md-2 justify-content-md-center">
<div class="col-xl-8 col-lg-10 col-12">
{{ if .Jobs }}
		{{ range .Jobs }}

		{{ template "jobRow" . }}

		{{ end }}
{{ else }}
	No readmes. Please <a href="/add">add a repository</a>.
{{ end }}
</div>
</div>
{{ end }}
`))

var Badge = template.Must(template.New("svg").Funcs(
	template.FuncMap{
		"statusColor": func(s string) string {
			switch s {
			case "Success":
				return "#2ecc71"
			case "Failed":
				return "#d35400"
			default:
				return "#2e4053"
			}
		},
	}).Parse(`
<svg xmlns="http://www.w3.org/2000/svg" width="115" height="20">
	<linearGradient id="a" x2="0" y2="100%">
		<stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
		<stop offset="1" stop-opacity=".1"/>
	</linearGradient>
	<rect rx="3" width="115" height="20" fill="#555"/>
	<rect rx="3" x="63" width="53" height="20" fill="{{statusColor .Status}}"/>
	<rect rx="3" width="115" height="20" fill="url(#a)"/>
	<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
		<text x="32" y="15" fill="#010101" fill-opacity=".3">
			goreadme
		</text>
		<text x="32" y="14">
			goreadme
		</text>
		<text x="87" y="15" fill="#010101" fill-opacity=".3">
			{{.Status}}
		</text>
		<text x="87" y="14">
			{{.Status}}
		</text>
	</g>
</svg>
`))
