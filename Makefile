.DEFAULT_GOAL := all

.PHONY: all
all: fmt lint cve-check test build

.PHONY: fmt
fmt:
	$(MAKE) -C aws fmt
	$(MAKE) -C azure fmt
	$(MAKE) -C gcp fmt

.PHONY: lint
lint:
	$(MAKE) -C aws lint
	$(MAKE) -C azure lint
	$(MAKE) -C gcp lint

.PHONY: cve-check
cve-check:
	$(MAKE) -C aws cve-check
	$(MAKE) -C azure cve-check
	$(MAKE) -C gcp cve-check

.PHONY: test
test:
	$(MAKE) -C aws test
	$(MAKE) -C azure test
	$(MAKE) -C gcp test

.PHONY: build
build:
	$(MAKE) -C aws build
	$(MAKE) -C azure build
	$(MAKE) -C gcp build

.PHONY: update-dependencies
update-dependencies:
	$(MAKE) -C aws update-dependencies
	$(MAKE) -C azure update-dependencies
	$(MAKE) -C gcp update-dependencies

.PHONY: kafkactl-version-check
kafkactl-version-check:
	./.github/kafkactl-version-check.sh

# usage make version=1.0.0 release
#
# manually executing goreleaser:
# export GITHUB_TOKEN=xyz
# export AUR_SSH_PRIVATE_KEY=$(cat /path/to/id_aur)
# export KAFKACTL_VERSION=v5.1.0
# docker login
# goreleaser --clean (--skip-validate)
#
.PHONY: release
release:
	current_date=`date "+%Y-%m-%d"`; eval "sed -i 's/## \[Unreleased\].*/## [Unreleased]\n\n## $$version - $$current_date/g' aws/CHANGELOG.md"
	git add "aws/CHANGELOG.md"
	current_date=`date "+%Y-%m-%d"`; eval "sed -i 's/## \[Unreleased\].*/## [Unreleased]\n\n## $$version - $$current_date/g' azure/CHANGELOG.md"
	git add "azure/CHANGELOG.md"
	current_date=`date "+%Y-%m-%d"`; eval "sed -i 's/## \[Unreleased\].*/## [Unreleased]\n\n## $$version - $$current_date/g' gcp/CHANGELOG.md"
	git add "gcp/CHANGELOG.md"
	git commit -m "releases $(version)"
	git tag -a v$(version) -m "release v$(version)"
	git push origin
	git push origin v$(version)
