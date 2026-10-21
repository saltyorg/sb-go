#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=commit.sh
source "$script_dir/commit.sh"
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
checkout="$test_dir/saltbox"
mkdir -p "$checkout/.github" "$checkout/requirements"
printf '3.12.13\n' > "$checkout/.python-version"
printf '0.12.9\n' > "$checkout/.uv-version"
printf '{}\n' > "$checkout/.github/renovate.json"
printf '# baseline\n' > "$checkout/requirements/requirements-saltbox.txt"
git -C "$checkout" init --initial-branch=master --quiet
git -C "$checkout" add .
git -C "$checkout" -c user.name=fixture -c user.email=fixture@example.invalid \
    -c commit.gpgsign=false commit --quiet -m fixture
base=$(git -C "$checkout" rev-parse HEAD)
printf '3.12.14\n' > "$checkout/.python-version"
printf '0.12.10\n' > "$checkout/.uv-version"
fixture_sha=cccccccccccccccccccccccccccccccccccccccc
scenario=success

# shellcheck disable=SC2317 # The sourced helper calls this API fixture.
gh() {
    if [[ $2 == graphql ]]; then
        printf 'mutation\n' >> "$test_dir/calls"
        cp "$4" "$test_dir/request.json"
        if [[ $scenario == lost || $scenario == stale ]]; then
            return 1
        fi
        jq -n --arg sha "$fixture_sha" '{data:{createCommitOnBranch:{commit:{oid:$sha}}}}'
    elif [[ $2 == */git/ref/heads/master ]]; then
        printf '%s\n' "$fixture_sha"
    elif [[ $2 == */commits/"$fixture_sha" ]]; then
        local parent=$base verified=true
        [[ $scenario != stale ]] || parent=dddddddddddddddddddddddddddddddddddddddd
        [[ $scenario != unsigned ]] || verified=false
        jq -n --arg base "$parent" --arg tree "$(git -C "$checkout" write-tree)" \
            --argjson verified "$verified" '{parents:[{sha:$base}],commit:{
              tree:{sha:$tree},message:"chore(deps): update python toolchain",
              verification:{verified:$verified}
            }}'
    else
        echo "Unexpected API request" >&2
        return 1
    fi
}

for scenario in success lost; do
    : > "$test_dir/calls"
    commit_toolchain_update "$checkout" "$base" saltyorg/Saltbox fixture-token "$test_dir"
    [[ $(wc -l < "$test_dir/calls") -eq 1 ]]
    jq -e --arg base "$base" '
        .variables.input.expectedHeadOid == $base and
        .variables.input.branch == {repositoryNameWithOwner:"saltyorg/Saltbox",refName:"master"} and
        (.variables.input.fileChanges.additions | length) == 4 and
        (.variables.input.fileChanges.additions[0].contents | @base64d) == "3.12.14\n"
    ' "$test_dir/request.json" > /dev/null
    [[ $(git -C "$checkout" rev-parse HEAD) == "$base" ]]
done
for scenario in stale unsigned; do
    if commit_toolchain_update "$checkout" "$base" saltyorg/Saltbox fixture-token "$test_dir"; then
        echo "Unexpected success for $scenario" >&2
        exit 1
    fi
done
echo 'Signed toolchain commit tests passed'
