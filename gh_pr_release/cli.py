import click
import logging
import git
import github
import operator

from .gh_pr_release import (
    Context,
    github_token,
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
    token = github_token(path)
    gh = github.Github(token, per_page=100)
    ctx = Context(path, gh)

    pr_list = merged_pull_requests(ctx)
    if not pr_list:
        logger.info(f"No commits between {ctx.base} and {ctx.head}")
        return
    for pr in sorted(pr_list, key=operator.attrgetter("created_at")):
        logger.info(f"To be released: #{pr.number} {pr.title}")

    release_pr = release_pull_request(ctx)
    if release_pr is None:
        release_pr = create_release_pull_request(ctx, pr_list)
        logger.info(f"Created pull request: {release_pr.url}")
    else:
        update_release_pull_request(ctx, release_pr, pr_list)
        logger.info(f"Updated pull request: {release_pr.url}")
