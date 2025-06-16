#!/bin/bash

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
ROOT_DIR="$(dirname "${SCRIPT_DIR}")"

cd "$ROOT_DIR" || exit

dependency_matches=$(grep -r --include "go.mod" github.com/deviceinsight/kafkactl/)
dependency_versions=$(echo "$dependency_matches" | grep -oP 'v\d+\.\d+\.\d+' | uniq)

if [[ $(echo "$dependency_versions" |wc -l) -ne 1 ]];then
   echo "all plugins should have the same kafkactl dependency:"
   echo "$dependency_matches"
   exit 1
fi

kafkactl_version="$dependency_versions"

echo "kafkactl version: $kafkactl_version"

readme_matches=$(grep -r --include "*.adoc" -P 'deviceinsight/kafkactl-\w+:latest')
readme_versions=$(echo "$readme_matches" | grep -oP 'v\d+\.\d+\.\d+' | uniq)
if [[ "$readme_versions" != "$kafkactl_version" ]];then
   echo "wrong kafkactl version in readme:"
   echo "$readme_matches"
   exit 1
fi

release_workflow=.github/workflows/release.yml
release_version=$(grep KAFKACTL_VERSION $release_workflow | grep -oP 'v\d+\.\d+\.\d+')
if [[ "$release_version" != "$kafkactl_version" ]];then
   echo "wrong kafkactl release version in $release_workflow: $release_version"
   exit 1
fi

echo "all versions ok."
