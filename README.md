# packaging-tools

Tools for maintaining osquery releases

## Release Notes & Changelog Generation

`cmd/release-notes` is a simple wrapper around github to generate a
CHANGELOG. Examines an existing CHANGELOG file, and enumerates commits
via graphql to generate list of things to add to the changelog.

Output will be displayed to stdout.

The expected workflow is to have a clean checkout on a branch ready
for the PR. Run this cmd, take the output, and update the CHANGELOG
file. As you edit and categorize, you can re-run the script to see the
omissions.

The underlying implementation uses timestamps to generate the diff
between the labels, because that's what git supports.

### How to use

This is written in `go` and you will need the go tool chain installed.

It may be run in place with a command like:

``` shell
go run ./cmd/release-notes --help
```

As an example:

``` shell
export GITHUB_TOKEN=`gh config get -h github.com oauth_token`

go run ./cmd/release-notes --changelog ~/checkouts/osquery/osquery/CHANGELOG.md  --last 5.12.1 --new 5.12.2
```
