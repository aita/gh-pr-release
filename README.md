# gh-pr-release

gh-pr-release a command line tool that creates a `release pull request`. gh-pr-release is similar to git-pr-release or github-pr-release.

## Install

```
go install github.com/aita/gh-pr-release
```

## Configuration

gh-pr-release reads configuration from a TOML file (`$PWD/gh_pr_release.toml` or `$HOME/.config/gh_pr_release.toml`) or environment variables.

### token (GH_PR_RELEASE_TOKEN)

GitHub API Token.

### owner (GH_PR_RELEASE_OWNER)

The name of organization or person who owns a repository.

### repo (GH_PR_RELEASE_REPO)

Then name of a repository.

### head (GH_PR_RELEASE_HEAD)

The name of branch than your changes are merged. Default is `develop`.

### base (GH_PR_RELEASE_BASE)

The name of branch that you want the changes pulled into. Default is `master`.

### title (GH_PR_RELEASE_TITLE)

The template of a pull request title. Optional.

### body (GH_PR_RELEASE_BODY)

The template of a pull request body. Optional.
