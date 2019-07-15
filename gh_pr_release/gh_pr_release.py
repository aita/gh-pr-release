import git
import github

import re
from dataclasses import dataclass, field
from datetime import datetime, timezone
from io import StringIO
from typing import Iterable, Set, List, Optional


RE_PR_REF = re.compile(r"refs/pull/(?P<number>\d+)/head")
RE_REMOTE_URL = re.compile(
    r"(git@github.com:|https://github.com\/)(?P<owner>.+)/(?P<name>.+)\.git"
)
RE_TASK_LIST = re.compile(r"(-|\*)\s+\[x\]\s+#(?P<number>\d+)")


@dataclass
class Context:
    path: str
    cmd: git.cmd.Git
    gh: github.Github
    head: str = "develop"
    base: str = "master"
    owner: str = field(init=False)
    name: str = field(init=False)
    gh_repo: github.Repository = field(init=False)

    def __post_init__(self):
        self.head = get_config(self.cmd, "gh-pr-release.branch.head", default=self.head)
        self.base = get_config(self.cmd, "gh-pr-release.base", default=self.base)

        remote_url = get_config(self.cmd, "remote.origin.url", gh_pr_release=False)
        m = RE_REMOTE_URL.match(remote_url)
        d = m.groupdict()
        self.owner = d["owner"]
        self.name = d["name"]
        self.gh_repo = self.gh.get_repo(f"{self.owner}/{self.name}")


def gh_token(cmd: git.cmd.Git) -> str:
    return get_config(cmd, "gh-pr-release.token")


def get_config(
    cmd: git.cmd.Git,
    key: str,
    default: Optional[str] = None,
    gh_pr_release: bool = True,
):
    if gh_pr_release:
        try:
            return cmd.config("--get", "--file", ".gh-pr-release", key)
        except git.GitCommandError:
            pass
    try:
        return cmd.config("--get", key)
    except git.GitCommandError:
        if default:
            return default
        raise


def merged_commit_hashes(ctx: Context) -> Iterable[str]:
    out = ctx.cmd.log(
        "--merges", "--pretty=format:%P", f"origin/{ctx.base}..origin/{ctx.head}"
    )
    if out == "":
        return
    for line in out.split("\n"):
        main, feature = line.split()
        yield feature


def merged_pull_request_numbers(ctx: Context, hashes: Set[str]) -> Iterable[int]:
    out = ctx.cmd.ls_remote("origin", "refs/pull/*/head")
    if out == "":
        return
    for line in out.split("\n"):
        sha1, ref = line.split()
        if sha1 not in hashes:
            continue
        m = RE_PR_REF.match(ref)
        if m:
            pr_number = int(m.group("number"))
            if sha1 != ctx.cmd.merge_base(sha1, f"origin/{ctx.base}").strip():
                yield pr_number


def merged_pull_requests(
    ctx: Context
) -> List[github.IssuePullRequest.IssuePullRequest]:
    hashes = set(merged_commit_hashes(ctx))
    pr_numbers = merged_pull_request_numbers(ctx, hashes)
    pr_list = []
    for pr_number in pr_numbers:
        pr = ctx.gh_repo.get_pull(pr_number)
        if pr.merged:
            pr_list.append(pr)
    return pr_list


def release_pull_request(
    ctx: Context
) -> Optional[github.IssuePullRequest.IssuePullRequest]:
    result = ctx.gh_repo.get_pulls(
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
    return ctx.gh_repo.create_pull(
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
