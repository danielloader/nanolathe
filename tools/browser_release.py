"""Publish main's asset-free browser build as its own GitHub release.

Every push to main gets one prerelease, tagged "<prefix><workflow run
number>", carrying the packaged distribution, its build.json and SHA256SUMS.
The website takes the highest run number whose assets are all present
(nanolathe-gg.github.io scripts/fetch-play.py), so the newest build wins even
when an older run finishes later. Once the new release is complete, the other
"<prefix>*" releases and their tags are pruned, keeping the newest few so a
website build that is still downloading finds its files.

The repository uses GitHub's immutable releases: publishing locks a release's
assets and tag, and a tag name can never be reused, even after the release is
deleted. So nothing here moves a tag or uploads to a published release. gh
creates each release as a draft, uploads the assets and publishes it once; a
re-run of the same workflow run finds its complete release and leaves it
alone. The flow is the same when immutability is off.
"""
import argparse
import json
import os
import subprocess
import sys

ASSETS = ("nanolathe-browser.tar.gz", "build.json", "SHA256SUMS")


def run_number(tag, prefix):
    """The workflow run number a release tag names, or None for any other tag."""
    if not isinstance(tag, str) or not tag.startswith(prefix):
        return None
    rest = tag[len(prefix):]
    return int(rest) if rest.isdigit() else None


def complete(release, names=ASSETS):
    """A published release carrying every distribution asset."""
    present = {asset.get("name") for asset in release.get("assets", [])}
    return not release.get("draft") and all(name in present for name in names)


def plan(releases, prefix, number, keep, names=ASSETS):
    """Decide what to do with the repository's "<prefix>*" releases.

    Returns (current, prune). current is this run's existing complete release,
    or None when one must be created. prune lists every "<prefix>*" release to
    delete once the current one exists: stale drafts, incomplete releases, tags
    without a run number, and complete releases older than the newest keep-1
    other runs. Releases outside the prefix are never touched.
    """
    tag = f"{prefix}{number}"
    current = None
    others = []
    prune = []
    for release in releases:
        name = release.get("tag_name", "")
        if not name.startswith(prefix):
            continue
        if name == tag and complete(release, names):
            current = release
        elif name == tag and not release.get("draft"):
            raise RuntimeError(f"{tag} is published without all of its assets and cannot be completed; "
                               "its tag cannot be reused, so publish again from a new workflow run")
        elif complete(release, names) and run_number(name, prefix) is not None:
            others.append(release)
        else:
            prune.append(release)
    others.sort(key=lambda release: run_number(release["tag_name"], prefix), reverse=True)
    prune.extend(others[max(keep - 1, 0):])
    return current, prune


def gh(*args):
    result = subprocess.run(["gh", *args], check=True, capture_output=True, text=True)
    return result.stdout


def list_releases(repository):
    pages = json.loads(gh("api", f"repos/{repository}/releases?per_page=100", "--paginate", "--slurp"))
    return [release for page in pages for release in page]


def release_by_tag(repository, tag):
    return json.loads(gh("api", f"repos/{repository}/releases/tags/{tag}"))


def delete_release(repository, release):
    gh("api", "-X", "DELETE", f"repos/{repository}/releases/{release['id']}")
    if not release.get("draft"):
        gh("api", "-X", "DELETE", f"repos/{repository}/git/refs/tags/{release['tag_name']}")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--repository", default=os.environ.get("GH_REPO"), help="owner/name, default $GH_REPO")
    parser.add_argument("--revision", required=True, help="the main commit the release targets")
    parser.add_argument("--run-number", required=True, type=int, help="the workflow run number, which names the tag")
    parser.add_argument("--prefix", default="browser-")
    parser.add_argument("--keep", default=3, type=int, help="complete releases to keep, counting this run's")
    parser.add_argument("--dry-run", action="store_true", help="list releases and print the plan; change nothing")
    parser.add_argument("files", nargs="+", help=f"the distribution assets: {', '.join(ASSETS)}")
    args = parser.parse_args(argv)
    if not args.repository:
        parser.error("--repository or $GH_REPO is required")
    names = tuple(os.path.basename(path) for path in args.files)
    if sorted(names) != sorted(ASSETS):
        parser.error(f"expected exactly the assets {', '.join(ASSETS)}")
    for path in args.files:
        if not os.path.isfile(path):
            parser.error(f"{path}: not a file")
    tag = f"{args.prefix}{args.run_number}"
    title = f"Browser build {args.revision[:7]}"
    notes = (f"Asset-free browser build of main at {args.revision}, workflow run {args.run_number}. "
             "Published automatically on every push to main; the website serves the newest complete "
             f"{args.prefix}* release at https://nanolathe.gg/play/ (docs/DESIGN_BROWSER_HOST.md §6).")

    current, prune = plan(list_releases(args.repository), args.prefix, args.run_number, args.keep, names)
    if current:
        print(f"browser-publish: {tag} is already published with its assets")
    elif args.dry_run:
        print(f"browser-publish: would publish {tag} at {args.revision} with {', '.join(names)}")
    else:
        gh("release", "create", tag, *args.files, "--repo", args.repository, "--target", args.revision,
           "--prerelease", "--latest=false", "--title", title, "--notes", notes)
        if not complete(release_by_tag(args.repository, tag), names):
            raise SystemExit(f"browser-publish: {tag} was published without all of its assets")
        print(f"browser-publish: published {tag} at {args.revision}")
    for release in prune:
        kind = "draft" if release.get("draft") else "release"
        if args.dry_run:
            print(f"browser-publish: would delete {kind} {release.get('tag_name')}")
            continue
        delete_release(args.repository, release)
        print(f"browser-publish: deleted {kind} {release.get('tag_name')}")
    output = os.environ.get("GITHUB_OUTPUT")
    if output and not args.dry_run:
        with open(output, "a") as handle:
            handle.write(f"tag={tag}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
