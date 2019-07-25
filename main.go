package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/Songmu/prompter"
	"github.com/google/go-github/v27/github"
	"github.com/kelseyhightower/envconfig"
	homedir "github.com/mitchellh/go-homedir"
	flag "github.com/spf13/pflag"
	"go.uber.org/multierr"
	"golang.org/x/oauth2"
	"gopkg.in/go-playground/validator.v9"
)

const (
	appName   = "gh-pr-release"
	envPrefix = "gh_pr_release"
	title     = `Release {{.ReleaseAt.Format "2006-01-02 15:04:05 -0700"}}`
	body      = `{{ range .PullRequests }}* [{{if .Checked}}x{{else}} {{end}}] #{{ .Number }} {{ .Title }}
{{ end }}`
)

var (
	configHomePath   string
	globalConfigPath string

	debug      = flag.Bool("debug", false, "print debug information")
	configPath = flag.String("config", fmt.Sprintf("%s.toml", appName), "configuration file path")
)

func init() {
	configHomePath := os.Getenv("XDG_CONFIG_HOME")
	if configHomePath == "" {
		homeDir, err := homedir.Dir()
		if err != nil {
			log.Fatal(err)
		}
		configHomePath = filepath.Join(homeDir, ".config")
	}
	globalConfigPath = filepath.Join(configHomePath, appName, "config.toml")
}

type Config struct {
	Token  string   `validate:"-"`
	Owner  string   `validate:"required"`
	Repo   string   `validate:"required"`
	Base   string   `validate:"required"`
	Head   string   `validate:"required"`
	Title  string   `validate:"required"`
	Body   string   `validate:"required"`
	Labels []string `validate:"-"`
}

func loadConfig(localConfigPath string) (cfg Config, err error) {
	cfg = Config{
		Base:  "master",
		Head:  "develop",
		Title: title,
		Body:  body,
	}
	for _, path := range []string{globalConfigPath, localConfigPath} {
		_, err = toml.DecodeFile(path, &cfg)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return
		}
	}
	if err = envconfig.Process(envPrefix, &cfg); err != nil {
		return
	}
	return
}

func createToken(ctx context.Context) (string, error) {
	defaultUsername := ""
	u, err := user.Current()
	if err == nil {
		defaultUsername = u.Username
	}
	username := prompter.Prompt("Username", defaultUsername)
	password := prompter.Password("Password")

	// Create a new client using HTTP Basic Authentication to create new GitHub API Token
	basicAuth := github.BasicAuthTransport{
		Username: username,
		Password: password,
	}
	client := github.NewClient(basicAuth.Client())
	note := appName
	authReq := &github.AuthorizationRequest{
		Scopes: []github.Scope{github.ScopeRepo},
		Note:   &note,
	}
	auth, res, err := client.Authorizations.Create(ctx, authReq)
	if res.StatusCode == http.StatusUnauthorized && strings.Contains(res.Header.Get("x-github-otp"), "required") {
		// Retry with two-factor authentication OTP code.
		basicAuth.OTP = prompter.Prompt("Two-factor authentication OTP code", "")
		client = github.NewClient(basicAuth.Client())
		auth, res, err = client.Authorizations.Create(ctx, authReq)
	}
	if err != nil {
		return "", err
	}
	return auth.GetToken(), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func saveToken(token string) (err error) {
	var (
		f    *os.File
		toml string
	)
	if exists(globalConfigPath) {
		f, err = os.OpenFile(globalConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			return err
		}
		toml = "\n"
	} else {
		if !exists(configHomePath) {
			err = os.MkdirAll(filepath.Dir(globalConfigPath), os.ModePerm)
			if err != nil {
				return
			}
		}
		f, err = os.Create(globalConfigPath)
		if err != nil {
			return
		}
	}
	defer func() {
		err = multierr.Append(err, f.Close())
	}()
	toml += "token = %q\n"
	_, err = fmt.Fprintf(f, toml, token)
	return
}

type PullRequest struct {
	*github.PullRequest
	Checked bool
}

func renderTemplate(name, text string, cfg Config, releaseAt time.Time, pullRequests []PullRequest) (string, error) {
	pr := struct {
		Config
		ReleaseAt    time.Time
		PullRequests []PullRequest
	}{
		Config:       cfg,
		ReleaseAt:    releaseAt,
		PullRequests: pullRequests,
	}
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		return "", err
	}
	buf := bytes.NewBuffer(nil)
	err = tmpl.Execute(buf, pr)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func mergedPullRequests(ctx context.Context, cfg Config, client *github.Client) ([]*github.PullRequest, error) {
	// List merged pull requests into the base branch
	comparison, _, err := client.Repositories.CompareCommits(context.Background(), cfg.Owner, cfg.Repo, cfg.Base, cfg.Head)
	if err != nil {
		return nil, err
	}
	hashes := map[string]bool{}
	for _, c := range comparison.Commits {
		if c.SHA != nil {
			hashes[*c.SHA] = true
		}
	}
	mergedPRs := []*github.PullRequest{}
	opt := &github.PullRequestListOptions{
		State:     "closed",
		Base:      cfg.Head,
		Sort:      "created",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	for {
		prs, resp, err := client.PullRequests.List(context.Background(), cfg.Owner, cfg.Repo, opt)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.MergeCommitSHA != nil && hashes[*pr.MergeCommitSHA] {
				mergedPRs = append(mergedPRs, pr)
				delete(hashes, *pr.MergeCommitSHA)
			}
		}
		if len(hashes) == 0 || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	sort.Slice(mergedPRs, func(i, j int) bool {
		return mergedPRs[i].GetNumber() < mergedPRs[j].GetNumber()
	})
	return mergedPRs, nil
}

func main() {
	flag.Parse()

	if *debug {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	} else {
		log.SetFlags(0)
	}

	// Load configuration
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		log.Fatal(err)
	}
	if cfg.Token == "" {
		log.Println("Could not obtain GitHub API token.")
		log.Println("Trying to create new token...")

		token, err := createToken(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		if err := saveToken(token); err != nil {
			log.Fatal(err)
		}
		cfg.Token = token
	}

	// Create a new client of github api with the api token
	tc := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.Token},
	))
	client := github.NewClient(tc)

	// List pull requests which merged into the head branch
	mergedPRs, err := mergedPullRequests(context.Background(), cfg, client)
	if err != nil {
		log.Fatal(err)
	}
	if len(mergedPRs) == 0 {
		log.Println("No pull requests to be released")
		return
	}
	for _, pr := range mergedPRs {
		log.Printf("To be released: #%d %s", pr.GetNumber(), pr.GetTitle())
	}

	// Find the release pull request
	prs, _, err := client.PullRequests.List(context.Background(), cfg.Owner, cfg.Repo, &github.PullRequestListOptions{
		State: "open",
		Base:  cfg.Base,
		Head:  fmt.Sprintf("%s:%s", cfg.Owner, cfg.Head),
	})
	if err != nil {
		log.Fatal(err)
	}
	var releasePR *github.PullRequest
	alreadyExists := false
	if len(prs) > 0 {
		releasePR = prs[0]
		alreadyExists = true
	}
	checked := map[int]bool{}
	if releasePR != nil {
		log.Printf("An existing release pull request #%d found", releasePR.GetNumber())

		reg := regexp.MustCompile(`(-|\*) *\[x\] *\#(\d+)`)
		for _, groups := range reg.FindAllStringSubmatch(releasePR.GetBody(), -1) {
			n, _ := strconv.Atoi(groups[2])
			checked[n] = true
		}
	}

	// Create title and body of the release pull request
	releaseAt := time.Now()
	pullRequests := []PullRequest{}
	for _, pr := range mergedPRs {
		pullRequests = append(pullRequests, PullRequest{
			PullRequest: pr,
			Checked:     checked[pr.GetNumber()],
		})
	}
	title, err := renderTemplate("title", cfg.Title, cfg, releaseAt, pullRequests)
	if err != nil {
		log.Fatal(err)
	}
	body, err := renderTemplate("body", cfg.Body, cfg, releaseAt, pullRequests)
	if err != nil {
		log.Fatal(err)
	}

	if alreadyExists {
		// Update an existing pull request
		releasePR.Title = &title
		releasePR.Body = &body
		_, _, err := client.PullRequests.Edit(context.Background(), cfg.Owner, cfg.Repo, releasePR.GetNumber(), releasePR)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// Create a new pull request
		releasePR, _, err = client.PullRequests.Create(context.Background(), cfg.Owner, cfg.Repo, &github.NewPullRequest{
			Title: &title,
			Body:  &body,
			Head:  &cfg.Head,
			Base:  &cfg.Base,
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	if len(cfg.Labels) > 0 {
		// Add labels to the pull request
		_, _, err := client.Issues.AddLabelsToIssue(context.Background(), cfg.Owner, cfg.Repo, releasePR.GetNumber(), cfg.Labels)
		if err != nil {
			log.Fatal(err)
		}
	}

	if alreadyExists {
		log.Printf("Updated pull request #%d: %s", releasePR.GetNumber(), releasePR.GetURL())
	} else {
		log.Printf("Created pull request #%d: %s", releasePR.GetNumber(), releasePR.GetURL())
	}
}
