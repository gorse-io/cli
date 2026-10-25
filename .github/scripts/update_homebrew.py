"""Complete Linux URLs/checksums in the PR opened by the Homebrew bump action."""

import base64
import hashlib
import json
import os
import re
import subprocess
from urllib.parse import quote
from urllib.request import urlopen

TARGETS = ("darwin_arm64", "linux_amd64", "linux_arm64")
RELEASE_URL = "https://github.com/gorse-io/cli/releases/download"
PAIR = re.compile(
    r'(?P<prefix>\burl ")(?P<url>https://github\.com/gorse-io/cli/releases/download/[^"/]+/gorse-cli_(?P<target>darwin_arm64|linux_amd64|linux_arm64))'
    r'(?P<middle>",\s*using: :nounzip\s*sha256 ")(?P<hash>[0-9a-f]{64})(?P<suffix>")'
)


def update_formula(content, tag, hashes):
    matches = list(PAIR.finditer(content))
    if sorted(m["target"] for m in matches) != sorted(TARGETS):
        raise ValueError("Expected exactly one URL/checksum pair per supported target")
    version = tag.removeprefix("v")
    if re.findall(r'^  version "([^"]+)"$', content, re.MULTILINE) != [version]:
        raise ValueError("The action must update the formula version first")
    for target in TARGETS:
        if not re.fullmatch(r"[0-9a-f]{64}", hashes[target]):
            raise ValueError(f"Invalid checksum for {target}")
    macos = next(m for m in matches if m["target"] == "darwin_arm64")
    if macos["url"] != f"{RELEASE_URL}/{tag}/gorse-cli_darwin_arm64" or macos["hash"] != hashes["darwin_arm64"]:
        raise ValueError("The action must update the macOS URL/checksum first")
    return PAIR.sub(
        lambda m: f'{m["prefix"]}{RELEASE_URL}/{tag}/gorse-cli_{m["target"]}{m["middle"]}{hashes[m["target"]]}{m["suffix"]}',
        content,
    )


def gh_api(endpoint, *, method="GET", payload=None, paginate=False):
    command = ["gh", "api", endpoint, "--method", method]
    if paginate:
        command += ["--paginate", "--slurp"]
    if payload is not None:
        command += ["--input", "-"]
    result = subprocess.run(
        command, input=json.dumps(payload) if payload is not None else None,
        text=True, stdout=subprocess.PIPE, check=True,
    )
    return json.loads(result.stdout)


def checksum(tag, target):
    with urlopen(f"{RELEASE_URL}/{tag}/gorse-cli_{target}", timeout=120) as response:
        return hashlib.file_digest(response, "sha256").hexdigest()


def complete_update(tag):
    if not re.fullmatch(r"v?\d+\.\d+\.\d+", tag):
        raise ValueError(f"Expected a stable release tag, got {tag!r}")
    repo = "repos/gorse-io/homebrew-tap"
    pages = gh_api(f"{repo}/pulls?state=open&per_page=100", paginate=True)
    candidates = [
        pr for page in pages for pr in page
        if pr["title"] == f"gorse-cli {tag.removeprefix('v')}"
        and (pr["head"].get("repo") or {}).get("full_name") == "gorse-io/homebrew-tap"
        and pr["head"]["ref"].startswith("update-gorse-cli.rb-")
    ]
    if len(candidates) != 1:
        raise ValueError(f"Expected one action-created formula PR, found {len(candidates)}")
    branch = candidates[0]["head"]["ref"]
    endpoint = f"{repo}/contents/Formula/gorse-cli.rb"
    read_endpoint = f"{endpoint}?ref={quote(branch, safe='')}"
    blob = gh_api(read_endpoint)
    content = base64.b64decode(blob["content"]).decode()
    hashes = {target: checksum(tag, target) for target in TARGETS}
    updated = update_formula(content, tag, hashes)
    if updated != content:
        gh_api(endpoint, method="PUT", payload={
            "branch": branch,
            "sha": blob["sha"],
            "message": f"gorse-cli {tag.removeprefix('v')}: update Linux binaries",
            "content": base64.b64encode(updated.encode()).decode(),
        })
        verified = gh_api(read_endpoint)
        if base64.b64decode(verified["content"]).decode() != updated:
            raise RuntimeError("Remote formula does not match the completed update")
    print(f"Verified all platform URLs/checksums on {branch}")


if __name__ == "__main__":
    complete_update(os.environ["RELEASE_TAG"])
