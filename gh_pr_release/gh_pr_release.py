import git
import github

import re
from configparser import NoOptionError, NoSectionError
from dataclasses import dataclass, field
from datetime import datetime, timezone
from io import StringIO
from pathlib import Path
from typing import List, Optional


RE_PR_REF = re.compile(r"refs/pull/(?P<number>\d+)/head")
RE_REMOTE_URL = re.compile(
    r"(git@github.com:|https://github.com\/)(?P<owner>.+)/(?P<name>.+)\.git"
)
RE_TASK_LIST = re.compile(r"(-|\*)\s+\[x\]\s+#(?P<number>\d+)")


@dataclass
class Context:
    path: str
    gh: github.Github
    head: str = "develop"
    base: str = "master"
    owner: str = field(init=False)
    name: str = field(init=False)
    repo: github.Repository = field(init=False)

    def __post_init__(self):
        self.head = get_config(
            self.path, 'gh-pr-release "branch".head', default=self.head
        )
        self.base = get_config(
            self.path, 'gh-pr-release "branch".base', default=self.base
        )

        remote_url = get_config(self.path, 'remote "origin".url', gh_pr_release=False)
        m = RE_REMOTE_URL.match(remote_url)
        d = m.groupdict()
        self.owner = d["owner"]
        self.name = d["name"]
        self.repo = self.gh.get_repo(f"{self.owner}/{self.name}")


def github_token(path: str) -> str:
    return get_config(path, "gh-pr-release.token")


def get_config(
    path: str, key: str, default: Optional[str] = None, gh_pr_release: bool = True
) -> str:
    def _get_config(cfg):
        return cfg.get(*key.rsplit(".", 1))

    if gh_pr_release:
        try:
            return _get_config(
                git.GitConfigParser(Path(path, ".gh-pr-release").as_posix())
            )
        except (NoSectionError, NoOptionError):
            pass
    try:
        return _get_config(git.Repo(path).config_reader())
    except (NoSectionError, NoOptionError):
        if default:
            return default
        raise


def merged_pull_requests(
    ctx: Context
) -> List[github.IssuePullRequest.IssuePullRequest]:
    commits = ctx.repo.compare(ctx.base, ctx.head).commits
    hashes = [c.sha for c in commits]
    pr_list = []
    pulls = ctx.repo.get_pulls(
        state="closed", base=ctx.head, sort="created", direction="desc"
    )
    for pr in pulls:
        if pr.merge_commit_sha in hashes:
            pr_list.append(pr)
    return reversed(pr_list)


def release_pull_request(
    ctx: Context
) -> Optional[github.IssuePullRequest.IssuePullRequest]:
    result = ctx.repo.get_pulls(
        state="open",
        sort="created",
        direction="desc",
        head=f"{ctx.owner}:{ctx.head}",
        base=ctx.base,
    )
    if result.totalCount == 1:
        return result[0]
    return None


def create_release_pull_request(
    ctx: Context, pr_list: List[github.IssuePullRequest.IssuePullRequest]
) -> github.IssuePullRequest.IssuePullRequest:
    now = datetime.now(timezone.utc).astimezone()
    return ctx.repo.create_pull(
        title=f"Release {now:%Y-%m-%d %H:%M:%S %z}",
        body=pull_request_description(pr_list),
        base=ctx.base,
        head=ctx.head,
    )


def update_release_pull_request(
    ctx: Context,
    release_pr: github.IssuePullRequest.IssuePullRequest,
    pr_list: List[github.IssuePullRequest.IssuePullRequest],
) -> None:
    now = datetime.now(timezone.utc).astimezone()
    release_pr.edit(
        title=f"Release {now:%Y-%m-%d %H:%M:%S %z}",
        body=pull_request_description(pr_list, release_pr.body),
    )


def pull_request_description(
    pr_list: List[github.IssuePullRequest.IssuePullRequest],
    old_body: Optional[str] = None,
) -> str:
    checked = set()
    if old_body:
        for m in re.finditer(RE_TASK_LIST, old_body):
            checked.add(int(m.group("number")))
    io = StringIO()
    for pr in pr_list:
        x_or_space = " "
        if pr.number in checked:
            x_or_space = "x"
        io.write(f"- [{x_or_space}] #{pr.number} {pr.title} @{pr.user.login}\n")
    return io.getvalue()
