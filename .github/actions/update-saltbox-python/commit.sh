#!/usr/bin/env bash

# Commit only the preflighted toolchain files, signed by GitHub as the App.
commit_toolchain_update() {
    local checkout=$1 base_commit=$2 repository=$3 token=$4 scratch=$5
    local request="$scratch/commit-request.json" response sha tree metadata
    metadata="$scratch/commit-response.json"
    git -C "$checkout" add -- .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt
    git -C "$checkout" diff --cached --check || return 1
    tree=$(git -C "$checkout" write-tree) || return 1
    jq -n \
        --arg repository "$repository" --arg base "$base_commit" \
        --rawfile python "$checkout/.python-version" \
        --rawfile uv "$checkout/.uv-version" \
        --rawfile renovate "$checkout/.github/renovate.json" \
        --rawfile requirements "$checkout/requirements/requirements-saltbox.txt" '
        {
          query: "mutation($input: CreateCommitOnBranchInput!) { createCommitOnBranch(input: $input) { commit { oid } } }",
          variables: {input: {
            branch: {repositoryNameWithOwner: $repository, refName: "master"},
            expectedHeadOid: $base,
            message: {headline: "chore(deps): update python toolchain"},
            fileChanges: {additions: [
              {path: ".python-version", contents: ($python | @base64)},
              {path: ".uv-version", contents: ($uv | @base64)},
              {path: ".github/renovate.json", contents: ($renovate | @base64)},
              {path: "requirements/requirements-saltbox.txt", contents: ($requirements | @base64)}
            ]}
          }}
        }' > "$request" || return 1

    if response=$(GH_TOKEN="$token" gh api graphql --input "$request"); then
        sha=$(jq -er '.data.createCommitOnBranch.commit.oid' <<< "$response") || return 1
    else
        # A lost response may follow a successful commit. Never replay blindly.
        echo "Commit response unavailable; checking master for the exact update" >&2
        sha=$(GH_TOKEN="$token" gh api "repos/$repository/git/ref/heads/master" --jq .object.sha) || return 1
    fi
    [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
    GH_TOKEN="$token" gh api "repos/$repository/commits/$sha" > "$metadata" || return 1
    if ! jq -e --arg base "$base_commit" --arg tree "$tree" '
        (.parents | length) == 1 and .parents[0].sha == $base and
        .commit.tree.sha == $tree and
        .commit.message == "chore(deps): update python toolchain" and
        .commit.verification.verified == true
    ' "$metadata" > /dev/null; then
        echo "Could not confirm the exact signed toolchain commit; refusing to overwrite master" >&2
        return 1
    fi
    echo "Created verified Saltbox toolchain commit $sha"
}
