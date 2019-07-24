package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/BurntSushi/xdg"
	"github.com/google/go-github/v27/github"
	"github.com/kelseyhightower/envconfig"
	flag "github.com/spf13/pflag"
	"golang.org/x/oauth2"
	"gopkg.in/go-playground/validator.v9"
)

const (
	cfgName = "gh_pr_release.toml"
	title   = `Release {{.ReleaseAt.Format "2006-01-02 15:04:05 -0700"}}`
	body    = `{{ range .PullRequests }}* [{{if .Checked}}x{{else}} {{end}}] #{{ .Number }} {{ .Title }}
{{ end }}`
)

type Config struct {
	Token string `validate:"required"`
	Owner string `validate:"required"`
	Repo  string `validate:"required"`
	Base  string `validate:"required"`
	Head  string `validate:"required"`
	Title string `validate:"required"`
	Body  string `validate:"required"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	configPath := flag.String("config", cfgName, "configuration file path")
	flag.Parse()

	cfg := Config{
		Base:  "master",
		Head:  "develop",
		Title: title,
		Body:  body,
	}
	paths := []string{}
	if path, err := (xdg.Paths{}).ConfigFile(cfgName); err == nil {
		paths = append(paths, path)
	}
	paths = append(paths, *configPath)
	for _, path := range paths {
		_, err := toml.DecodeFile(path, &cfg)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := envconfig.Process("gh_pr_release", &cfg); err != nil {
		log.Fatal(err)
	}
	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		log.Fatal(err)
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
	if len(prs) > 0 {
		releasePR = prs[0]
	}
	checked := map[int]bool{}
	if releasePR != nil {
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

	if releasePR != nil {
		// Update an existing pull request
		releasePR.Title = &title
		releasePR.Body = &body
		_, _, err := client.PullRequests.Edit(context.Background(), cfg.Owner, cfg.Repo, releasePR.GetNumber(), releasePR)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// Create a new release pull request
		_, _, err := client.PullRequests.Create(context.Background(), cfg.Owner, cfg.Repo, &github.NewPullRequest{
			Title: &title,
			Body:  &body,
			Head:  &cfg.Head,
			Base:  &cfg.Base,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
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
