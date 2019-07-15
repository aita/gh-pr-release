import click
import logging
import git
import github

from .gh_pr_release import (
    Context,
    gh_token,
    merged_pull_requests,
    release_pull_request,
    create_release_pull_request,
    update_release_pull_request,
)

logger = logging.getLogger("gh-pr-release." + __name__)
logger.setLevel(logging.INFO)
handler = logging.StreamHandler()
formatter = logging.Formatter("%(levelname)s: %(message)s")
handler.setFormatter(formatter)
logger.addHandler(handler)


@click.command()
@click.option("--path", default=".")
def main(path):
    cmd = git.cmd.Git(path)
    token = gh_token(cmd)
    gh = github.Github(token)
    ctx = Context(path, cmd, gh)

    pr_list = merged_pull_requests(ctx)
    for pr in pr_list:
        logger.info(f"To be released: #{pr.number} {pr.title}")

    release_pr = release_pull_request(ctx)
    if release_pr is None:
        release_pr = create_release_pull_request(ctx, pr_list)
        logger.info(f"Created pull request: {release_pr.url}")
    else:
        update_release_pull_request(ctx, release_pr, pr_list)
        logger.info(f"Updated pull request: {release_pr.url}")
