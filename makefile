# start project configuration
name := evergreen
buildDir := bin
packages := $(name) agent db cli archive remote command taskrunner util plugin plugin-builtin-git 
packages += plugin-builtin-gotest plugin-builtin-attach plugin-builtin-manifest plugin-builtin-archive
packages += plugin-builtin-shell plugin-builtin-s3copy plugin-builtin-expansions plugin-builtin-s3
packages += notify thirdparty alerts auth scheduler model hostutil validator service monitor repotracker
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration


# start rules for binding agents
define buildBinary = 
	$(vendorGopath) go build -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" -o $@ "./$<" 
endef
$(buildDir)/cli:cli/main/cli.go $(srcFiles)
	$(buildBinary)
$(buildDir)/evergreen_api_server:service/api_main/apiserver.go $(srcFiles)
	$(buildBinary)
$(buildDir)/evergreen_ui_server:service/ui_main/ui.go $(srcFiles)
	$(buildBinary)
$(buildDir)/evergreen_runner:runner/main/runner.go $(srcFiles)
	$(buildBinary)

define buildRaceBinary = 
	$(vendorGopath) go build -race -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" -o $@ "./$<"
endef
$(buildDir)/evergreen_api_server.race:service/api_main/apiserver.go $(srcFiles) 
	$(buildRaceBinary)
$(buildDir)/evergreen_runner.race:runner/main/runner.go $(srcFiles) 
	$(buildRaceBinary)
$(buildDir)/evergreen_ui_server.race:service/ui_main/ui.go $(srcFiles) 
	$(buildRaceBinary)
$(buildDir)/cli.race:cli/main/cli.go $(srcFiles)
	$(buildRaceBinary)
binaries := $(buildDir)/cli $(buildDir)/evergreen_ui_server $(buildDir)/evergreen_runner $(buildDir)/evergreen_api_server
raceBinaries := $(foreach bin,$(binaries),$(bin).race)
phony += $(binaries) $(raceBinares) 
# end rules for building server binaries 

goos := $(shell go env GOOS)
goarch := $(shell go env GOARCH)
xcPlatforms := windows_amd64 windows_386 linux_386 linux_s390x linux_arm64 linux_ppc64le solaris_amd64 darwin_amd64
$(buildDir)/build-cross-compile:scripts/build-cross-compile.go
	@mkdir -p $(buildDir)
	go build -o $@ $<


# start rules for building agents 
agentBuildDir := executables
$(agentBuildDir)/$(goos)_$(goarch) $(agentBuildDir):
	mkdir -p $@
$(agentBuildDir)/version:
	@mkdir -p $(dir $@)
	git rev-parse HEAD >| $@

agentSource := agent/main/agent.go
agent:$(agentBuildDir)/$(goos)_$(goarch)/main
agents:$(foreach platform,$(xcPlatforms),$(agentBuildDir)/$(platform)/main)
$(agentBuildDir)/%/main:$(buildDir)/build-cross-compile $(agentBuildDir)/version $(srcFiles)
	@$(vendorGopath) ./$< -buildName=$* -directory=$(agentBuildDir) -ldflags="-X=github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" -source="$(agentSource)"

phony += agents agent $(agentBuildDir)/version 

clientBuildDir := clients
cliSource := cli/main/cli.go
cli:$(clientBuildDir)/$(goos)_$(goarch)/cli
clis:$(foreach platform,$(xcPlatforms),$(clientBuildDir)/$(platform)/cli)
$(clientBuildDir)/%/cli:$(buildDir)/build-cross-compile $(srcFiles)
	@$(vendorGopath) ./$< -buildName=$* -directory=$(clientBuildDir) -ldflags="-X=github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" -source="$(cliSource)"
phony += cli clis
# end rules for building agetn

# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=1m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gas"
lintArgs += --skip="$(buildDir)" --skip="scripts"
#  add and configure additional linters
lintArgs += --enable="go fmt -s" --enable="goimports" --enable="misspell" 
lintargs += --enable="lll" --enable"unused"
lintArgs += --line-length=100 --dupl-threshold=175
#  two similar functions triggered the duplicate warning, but they're not.
lintArgs += --exclude="file is not goimported" # test files aren't imported
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
# end lint suppressions


######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
gopath := $(shell go env GOPATH)
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" )
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*")
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
testBin := $(foreach target,$(packages),$(buildDir)/test.$(target))
raceBin := $(foreach target,$(packages),$(buildDir)/race.$(target))
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
# $(gopath)/src/%:
# 	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
# 	go get $(subst $(gopath)/src/,,$@)
# end dependency installation tools

list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
list-race:
	@echo -e "test (race detector) targets:" $(foreach target,$(packages),\\n\\trace-$(target))

# implementation details for building the binary and creating a
# convienent link in the working directory
$(name):$(buildDir)/$(name)
	@[ -e $@ ] || ln -s $<
$(buildDir)/$(name):$(srcFiles)
	$(vendorGopath) go build -o $@ main/$(name).go
$(buildDir)/$(name).race:$(srcFiles)
	$(vendorGopath) go build -race -o $@ main/$(name).go
phony += $(buildDir)/$(name)
# end main build


# userfacing targets for basic build and development operations
lint:$(lintDeps)
	$(gopath)/bin/gometalinter $(lintArgs) ./... | sed 's%$</%%' | grep -v "$(gopath)"
build:$(binaries)
build-race:$(raceBinaries)
test:$(foreach target,$(packages),test-$(target))
race:$(foreach target,$(packages),race-$(target))
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS: $(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS: $(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS: $(foreach target,$(packages),$(buildDir)/race.$(target))
# end front-ends


# distribution targets and implementation
$(buildDir)/make-tarball:scripts/make-tarball.go $(buildDir)/render-gopath
	$(vendorGopath) go build -o $@ $<
dist:$(buildDir)/dist.tar.gz
dist-test:$(buildDir)/dist-test.tar.gz
dist-source:$(buildDir)/dist-source.tar.gz
$(buildDir)/dist.tar.gz:$(buildDir)/make-tarball agents $(binaries) 
	./$< --name $@ --prefix $(name) $(foreach bin,$(binaries),--item $(bin)) --item ./public --item ./executables --item ./client
$(buildDir)/dist-test.tar.gz:makefile $(binaries) $(raceBinaries)
	tar -czvf $@ $^
$(buildDir)/dist-source.tar.gz:$(buildDir)/make-tarball $(srcFiles) $(testSrcFiles) makefile
	./$< --name $@ --prefix $(name) $(subst $(name),,$(foreach pkg,$(packages),--item ./$(pkg))) --item ./scripts --item makefile --exclude "$(name)" --exclude "^.git/" --exclude "$(buildDir)/"
# end main build


# convenience targets for runing tests and coverage tasks on a
# specific package.
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $<
html-coverage-%:$(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $<
# end convienence targets


# start vendoring configuration
#    begin with configuration of dependencies
vendorDeps := github.com/Masterminds/glide
vendorDeps := $(addprefix $(gopath)/src/,$(vendorDeps))
vendor-deps:$(vendorDeps)
#   this allows us to store our vendored code in vendor and use
#   symlinks to support vendored code both in the legacy style and with
#   new-style vendor directories. When this codebase can drop support
#   for go1.4, we can delete most of this. 
-include $(buildDir)/makefile.vendor
#   nested vendoring is used to support projects that have 
# nestedVendored := foo
# nestedVendored := $(foreach project,$(nestedVendored),$(project)/build/vendor)
$(buildDir)/makefile.vendor:$(buildDir)/render-gopath makefile
	@mkdir -p $(buildDir)
	@echo "vendorGopath := \$$(shell \$$(buildDir)/render-gopath $(nestedVendored))" >| $@
#   targets for the directory components and manipulating vendored files.
vendor-sync:$(vendorDeps)
	./vendor.sh
# vendor-clean:
# 	rm -rf vendor/github.com/stretchr/testify/vendor/
# 	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" | xargs rm -f
change-go-version:
	rm -rf $(buildDir)/make-vendor $(buildDir)/render-gopath
	@$(MAKE) $(makeArgs) vendor > /dev/null 2>&1
vendor:$(buildDir)/vendor/src
$(buildDir)/vendor/src:$(buildDir)/make-vendor $(buildDir)/render-gopath
	@./$(buildDir)/make-vendor
#   targets to build the small programs used to support vendoring.
$(buildDir)/make-vendor:scripts/make-vendor.go
	@mkdir -p $(buildDir)
	go build -o $@ $<
$(buildDir)/render-gopath:scripts/render-gopath.go
	@mkdir -p $(buildDir)
	go build -o $@ $<

#   define dependencies for scripts
scripts/make-vendor.go:scripts/vendoring/vendoring.go
scripts/render-gopath.go:scripts/vendoring/vendoring.go
#   add phony targets
phony += vendor vendor-deps vendor-clean vendor-sync change-go-version
# end vendoring tooling configuration


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testRunDeps := $(name)
testArgs := -test.v --test.timeout=20m
#  targets to compile
$(buildDir)/test.%:$(testSrcFiles)
	$(vendorGopath) go test $(if $(DISABLE_COVERAGE),,-covermode=count) -c -o $@ ./$*
$(buildDir)/race.%:$(testSrcFiles)
	$(vendorGopath) go test -race -c -o $@ ./$*
#  targets to run any tests in the top-level package
$(buildDir)/test.$(name):$(testSrcFiles)
	$(vendorGopath) go test $(if $(DISABLE_COVERAGE),,-covermode=count) -c -o $@ ./
$(buildDir)/race.$(name):$(testSrcFiles)
	$(vendorGopath) go test -race -c -o $@ ./
#  targets to run the tests and report the output
$(buildDir)/output.%.test:$(buildDir)/test.% .FORCE
	./$< $(testArgs) | tee $@
$(buildDir)/output.%.race:$(buildDir)/race.% .FORCE
	./$< $(testArgs) | tee $@
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage:$(buildDir)/test.% .FORCE
	./$< $(testArgs) -test.coverprofile=$@
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(vendorGopath) go tool cover -html=$< -o $@
# end test and coverage artifacts


# clean and other utility targets
clean:
	rm -rf $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.* $(name) $(buildDir)/$(name)
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
