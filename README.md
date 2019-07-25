# gh-pr-release

gh-pr-release is a command line tool that creates a `release pull request` with GitHub API. gh-pr-release is similar to [git-pr-release](https://github.com/motemen/git-pr-release) and [github-pr-release](https://github.com/uiureo/github-pr-release).

## Install

```
go install github.com/aita/gh-pr-release
```

## Configuration

gh-pr-release reads configuration from TOML files (`$PWD/gh-pr-release.toml` and `$HOME/.config/gh-pr-release/config.toml`) or environment variables.

### token (GH_PR_RELEASE_TOKEN)

GitHub API Token.

### owner (GH_PR_RELEASE_OWNER)

The name of organization or person who owns the repository.

### repo (GH_PR_RELEASE_REPO)

Then name of the repository.

### head (GH_PR_RELEASE_HEAD)

The name of branch than your changes are merged into. Default is `develop`.

### base (GH_PR_RELEASE_BASE)

The name of branch that you want the changes pulled into. Default is `master`.

### title (GH_PR_RELEASE_TITLE)

**Optional.** The template of pull request title.

### body (GH_PR_RELEASE_BODY)

**Optional.** The template of pull request body.
