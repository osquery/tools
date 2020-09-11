# packaging-tools

Tools for packaging and signing osquery. 

As these deal with accessing key materials, they're in a separate
repository to make PRs more easily scrutinized.

## Release Notes & Changelog Generation

`cmd/release-notes` is a simple wrapper around github to generate a
CHANGELOG. Examines an existing CHANGELOG file, and enumerates commits
via graphql to generate list of things to add to the changelog. 

Output will be displayed to stdout.

The expected workflow is to have a clean checkout on a branch ready
for the PR. Run this cmd, take the output, and update the CHANGELOG
file. As you edit and categorize, you can re-run the script to see the
omissions.

The underlying implementation uses timestamps, because that's what git
supports.
