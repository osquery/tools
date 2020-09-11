package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"text/template"

	"github.com/machinebox/graphql"
)

func main() {
	ctx := context.Background()

	graphqlClient := graphql.NewClient("https://api.github.com/graphql")
	//graphqlClient.Log = func(s string) { log.Println(s) }

	timestamp, err := getGitTimeStamp(ctx, graphqlClient, "4.3.0")
	if err != nil {
		log.Fatal(err)
	}

	commits, err := getGitCommits(ctx, graphqlClient, timestamp)
	if err != nil {
		log.Fatal(err)
	}

	if err := changelogSnippet(commits); err != nil {
		log.Fatal(err)
	}

}

func changelogSnippet(commits []*Commit) error {
	// This has a lot of stupid formatting

	// some PRs were merged via rebase, not squash. So track what we've seen.
	seen, err := parseChangelogForSeen("/Users/seph/checkouts/osquery/osquery/CHANGELOG.md")
	if err != nil {
		return err
	}
	changelog := map[clSection][]string{
		clToFix:       make([]string, 0, len(commits)),
		clBugFixes:    make([]string, 0, len(commits)),
		clBuild:       make([]string, 0, len(commits)),
		clHardening:   make([]string, 0, len(commits)),
		clNewFeatures: make([]string, 0, len(commits)),
		clSecurity:    make([]string, 0, len(commits)),
		clTable:       make([]string, 0, len(commits)),
	}

	for _, c := range commits {
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
<a name="4.4.0"></a>
## [4.4.0](https://github.com/osquery/osquery/releases/tag/4.4.0)

[Git Commits](https://github.com/osquery/osquery/compare/4.3.0...4.4.0)

{{ range $i, $section := .Changelog }}
### {{ $section.Name }}
{{ range $i, $line := $section.Lines }}
- {{ $line -}}
{{ end }}
{{ end -}}

`

	var data = struct {
		Changelog []changelogTypeForTemplate
	}{
		Changelog: changelogFlat,
	}

	t, err := template.New("changelog").Parse(changelogTemplate)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(os.Stdout, "changelog", data)

}

type clSection string

const (
	clToFix       clSection = "FIXME: Please Categorize"
	clBugFixes              = "Bug Fixes"
	clBuild                 = "Build"
	clHardening             = "Hardening"
	clNewFeatures           = "New Features / Under the Hood improvements"
	clSecurity              = "Security Issues"
	clTable                 = "Table Changes"
	clDocs                  = "Documentation"
)

var sectionOrder = []clSection{
	clToFix,
	clNewFeatures,
	clTable,
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
	PRLabels        []string
}

func (c *Commit) ChangeLine() string {
	return fmt.Sprintf("%s ([#%d](https://github.com/osquery/osquery/pull/%d))", c.PRTitle, c.PRNumber, c.PRNumber)
}

func (c *Commit) ChangeSection() clSection {
	return clToFix
}

func getGitCommits(ctx context.Context, graphqlClient *graphql.Client, timestamp string) ([]*Commit, error) {
	req := graphql.NewRequest(`
query ($timestamp: GitTimestamp!) {
  repository(owner: "osquery", name: "osquery") {
    nameWithOwner
    object(expression: "master") {
      ... on Commit {
        oid
        history(first: 100, since: $timestamp) {
          nodes {
            oid
            messageHeadline
            committedDate
            associatedPullRequests(first: 10) {
              nodes {
                number
                title
                labels(first: 10) {
                  nodes {
                    name
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
`)

	var respData struct {
		Repository struct {
			Name   string `json:"nameWithOwner"`
			Object struct {
				History struct {
					Nodes []struct {
						Sha                    string `json:"oid"`
						Timestamp              string `json:"committedDate"`
						MessageHeadline        string
						AssociatedPullRequests struct {
							Nodes []struct {
								Number int
								Title  string
								Labels struct {
									Nodes []struct {
										Name string
									}
								}
							}
						}
					}
				}
			}
		}
	}

	req.Var("timestamp", timestamp)
	req.Header.Add("Authorization", "token "+os.Getenv("GITHUB_TOKEN"))

	if err := graphqlClient.Run(ctx, req, &respData); err != nil {
		return nil, err
	}

	commits := make([]*Commit, len(respData.Repository.Object.History.Nodes))

	for i, rawCommit := range respData.Repository.Object.History.Nodes {
		pr := rawCommit.AssociatedPullRequests.Nodes[0]
		prLabels := make([]string, len(pr.Labels.Nodes))
		for i, label := range pr.Labels.Nodes {
			prLabels[i] = label.Name
		}

		commits[i] = &Commit{
			Sha:             rawCommit.Sha,
			MessageHeadline: rawCommit.MessageHeadline,
			Timestamp:       rawCommit.Timestamp,
			PRNumber:        pr.Number,
			PRTitle:         pr.Title,
			PRLabels:        prLabels,
		}
	}

	return commits, nil
}

func getGitTimeStamp(ctx context.Context, graphqlClient *graphql.Client, lastVersion string) (string, error) {
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
	req.Header.Add("Authorization", "token "+os.Getenv("GITHUB_TOKEN"))

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
