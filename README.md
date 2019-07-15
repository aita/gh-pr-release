# gh-pr-release

gh-pr-release a command line tool that creates a `release pull request`. gh-pr-release is similar to git-pr-release.

## Configuration

gh-pr-release reads configuration data from a git configuration file. You can also write your configuration data into the per-repository `.gh-pr-release` file instead of `.git/config` or `~/.gitconfig`.

### gh-pr-release.token

GitHub API Token.

```
[gh-pr-release]
    token = <YOUR GITHUB API TOKEN>
```

### gh-pr-release.branch.head

The name of branch than your changes are merged. Default is `develop`.

```
[gh-pr-release "branch"]
    head = staging
```

### gh-pr-release.branch.base

The name of branch that you want the changes pulled into. Default is `master`.

```
[gh-pr-release "branch"]
    base = production
```
