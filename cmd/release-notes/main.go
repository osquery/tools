package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"text/template"

	"github.com/machinebox/graphql"
	"github.com/peterbourgon/ff"
	"golang.org/x/text/message"
)

func main() {
	fs := flag.NewFlagSet("release-notes", flag.ExitOnError)
	var (
		flLastRelease       = fs.String("last", "", "Last Release")
		flNewRelease        = fs.String("new", "", "New Release")
		flExistingChangelog = fs.String("changelog", "", "Existing changelog file. This is used as a basis to skip existing items")
		flGithubToken       = fs.String("github-token", os.Getenv("GITHUB_TOKEN"), "Github authentication token. Defaults to ENV[GITHUB_TOKEN]")
	)

	if err := ff.Parse(fs, os.Args[1:],
		ff.WithConfigFileParser(ff.PlainParser),
	); err != nil {
		fmt.Fprintf(os.Stderr, "Flag parsing failure: %v\n", err)
		os.Exit(1)
	}

	// Look for missing options
	missingOpt := false
	for fl, val := range map[string]string{
		"last":         *flLastRelease,
		"new":          *flNewRelease,
		"changelog":    *flExistingChangelog,
		"github-token": *flGithubToken,
	} {
		if val == "" {
			fmt.Fprintf(os.Stderr, "Missing required flag: %s\n", fl)
			missingOpt = true
		}
	}

	if missingOpt {
		os.Exit(1)
	}

	ctx := context.Background()

	graphqlClient := graphql.NewClient("https://api.github.com/graphql")
	//graphqlClient.Log = func(s string) { log.Println(s) }

	timestamp, err := getGitTimeStamp(ctx, graphqlClient, *flGithubToken, *flLastRelease)
	if err != nil {
		log.Fatal(err)
	}

	commits, err := getGitCommits(ctx, graphqlClient, *flGithubToken, timestamp)
	if err != nil {
		log.Fatal(err)
	}

	if err := changelogSnippet(commits, *flExistingChangelog, *flLastRelease, *flNewRelease); err != nil {
		log.Fatal(err)
	}

}

func changelogSnippet(commits []*Commit, existingChangelog, lastVersion, newVersion string) error {
	// This has a lot of stupid formatting

	// some PRs were merged via rebase, not squash. So track what we've seen.
	seen, err := parseChangelogForSeen(existingChangelog)
	if err != nil {
		return err
	}
	changelog := map[clSection][]string{
		clToFix:        make([]string, 0, len(commits)),
		clBugFixes:     make([]string, 0, len(commits)),
		clBuild:        make([]string, 0, len(commits)),
		clHardening:    make([]string, 0, len(commits)),
		clNewFeatures:  make([]string, 0, len(commits)),
		clUnderTheHood: make([]string, 0, len(commits)),
		clSecurity:     make([]string, 0, len(commits)),
		clTable:        make([]string, 0, len(commits)),
	}

	authorSet := make(map[string]bool)

	for _, c := range commits {
		for _, a := range c.Authors {
			authorSet[a] = true
		}

		// If we're already seen this PR, don't add it to the print list
		if _, ok := seen[c.PRNumber]; ok {
			continue
		}

		changelog[c.ChangeSection()] = append(changelog[c.ChangeSection()], c.ChangeLine())

		seen[c.PRNumber] = true
	}

	// templates are always attrocious. This is a giant ball of hackery.

	type changelogTypeForTemplate struct {
		Name  string
		Lines []string
	}

	changelogFlat := []changelogTypeForTemplate{}

	for _, name := range sectionOrder {
		changelogFlat = append(changelogFlat, changelogTypeForTemplate{
			Name:  string(name),
			Lines: changelog[name],
		})
	}

	changelogTemplate := `
<a name="{{ .NewVersion }}"></a>
## [{{ .NewVersion }}](https://github.com/osquery/osquery/releases/tag/{{ .NewVersion }})

[Git Commits](https://github.com/osquery/osquery/compare/{{ .LastVersion }}...{{ .NewVersion }})

Representing commits from {{ .AuthorCount }} contributors! Thank you all.

{{ range $i, $section := .Changelog }}
### {{ $section.Name }}
{{ range $i, $line := $section.Lines }}
- {{ $line -}}
{{ end }}
{{ end -}}

`

	var data = struct {
		Changelog   []changelogTypeForTemplate
		LastVersion string
		NewVersion  string
		AuthorCount string
	}{
		Changelog:   changelogFlat,
		LastVersion: lastVersion,
		NewVersion:  newVersion,
		AuthorCount: message.NewPrinter(message.MatchLanguage("en")).Sprintf("%d", len(authorSet)),
	}

	t, err := template.New("changelog").Parse(changelogTemplate)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(os.Stdout, "changelog", data)

}

type clSection string

const (
	clToFix        clSection = "FIXME: Please Categorize"
	clBugFixes     clSection = "Bug Fixes"
	clBuild        clSection = "Build"
	clHardening    clSection = "Hardening"
	clNewFeatures  clSection = "New Features"
	clUnderTheHood clSection = "Under the Hood improvements"
	clSecurity     clSection = "Security Issues"
	clTable        clSection = "Table Changes"
	clDocs         clSection = "Documentation"
)

var sectionOrder = []clSection{
	clToFix,
	clNewFeatures,
	clTable,
	clUnderTheHood,
	clBugFixes,
	clDocs,
	clBuild,
	clSecurity,
	clHardening,
}

type Commit struct {
	Sha             string
	MessageHeadline string
	Timestamp       string
	PRNumber        int
	PRTitle         string
	PRLabels        map[string]bool
	Authors         []string
}

func (c *Commit) ChangeLine() string {
	return fmt.Sprintf("%s ([#%d](https://github.com/osquery/osquery/pull/%d))", c.PRTitle, c.PRNumber, c.PRNumber)
}

// ChangeSection attempts to analyze the commit, and return what
// section of the changelog it is for.
func (c *Commit) ChangeSection() clSection {
	switch {
	case c.labelsInclude("documentation"):
		return clDocs
	}

	return clToFix
}

// labelsInclude checks the labels on this PR for whether all
// requested labels are applied.
func (c *Commit) labelsInclude(labels ...string) bool {
	for _, l := range labels {
		if _, ok := c.PRLabels[l]; !ok {
			return false
		}
	}

	return true
}

func getGitCommits(ctx context.Context, graphqlClient *graphql.Client, token string, timestamp string) ([]*Commit, error) {
	responses, err := fetchCommits(ctx, graphqlClient, token, timestamp)
	if err != nil {
		return nil, err
	}

	allCommits := []*Commit{}

	for _, respData := range responses {
		commits := make([]*Commit, len(respData.Repository.Object.History.Nodes))

		for i, rawCommit := range respData.Repository.Object.History.Nodes {
			var prNumber int
			prTitle := fmt.Sprintf("%s (MISSING PR for commit %s)", rawCommit.MessageHeadline, rawCommit.Sha)
			prLabels := make(map[string]bool)

			if len(rawCommit.AssociatedPullRequests.Nodes) > 0 {
				pr := rawCommit.AssociatedPullRequests.Nodes[0]
				prTitle = pr.Title
				prNumber = pr.Number
				for _, label := range pr.Labels.Nodes {
					prLabels[label.Name] = true
				}
			}

			authorSet := make(map[string]bool)
			if len(rawCommit.Authors.Nodes) > 0 {
				for _, a := range rawCommit.Authors.Nodes {
					switch {
					case a.User.Login != "":
						authorSet[a.User.Login] = true
					case a.Email != "":
						authorSet[a.Email] = true
					}
				}
			} else {
				fmt.Printf("WARNING: Commit %s is missing authors\n", rawCommit.Sha)
			}

			authors := make([]string, len(authorSet))
			for a := range authorSet {
				authors = append(authors, a)
			}

			commits[i] = &Commit{
				Sha:             rawCommit.Sha,
				MessageHeadline: rawCommit.MessageHeadline,
				Timestamp:       rawCommit.Timestamp,
				PRNumber:        prNumber,
				PRTitle:         prTitle,
				PRLabels:        prLabels,
				Authors:         authors,
			}
		}
		allCommits = append(allCommits, commits...)
	}

	return allCommits, nil
}

func getGitTimeStamp(ctx context.Context, graphqlClient *graphql.Client, token string, lastVersion string) (string, error) {
	req := graphql.NewRequest(`
query ($lastVer: String!) {
  repository(owner: "osquery", name: "osquery") {
    object(expression: $lastVer) {
      ... on Commit {
        oid
        committedDate
      }
    }
  }
}
`)

	var respData struct {
		Repo struct {
			Object struct {
				Sha       string `json:"oid"`
				Timestamp string `json:"committedDate"`
			} `json:"object"`
		} `json:"repository"`
	}

	req.Var("lastVer", lastVersion)
	req.Header.Add("Authorization", "token "+token)

	if err := graphqlClient.Run(ctx, req, &respData); err != nil {
		return "", err
	}

	return respData.Repo.Object.Timestamp, nil

}

func parseChangelogForSeen(filePath string) (map[int]bool, error) {
	seenPRs := make(map[int]bool)

	prRegex := regexp.MustCompile(`https://github.com/osquery/osquery/pull/(\d+)`)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		matches := prRegex.FindAllStringSubmatch(scanner.Text(), -1)
		for _, m := range matches {
			prNum, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			seenPRs[prNum] = true
		}
	}

	return seenPRs, nil
}
