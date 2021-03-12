package main

import (
	"context"

	"github.com/machinebox/graphql"
)

const query = `
query ($after: String, $timestamp: GitTimestamp!) {
  repository(owner: "osquery", name: "osquery") {
    nameWithOwner
    object(expression: "master") {
      ... on Commit {
        oid
        history(first: 100, after: $after, since: $timestamp) {
          pageInfo {
            endCursor
            hasNextPage
          }
          nodes {
            oid
            messageHeadline
            committedDate
            authors(first: 100) {
              nodes {
                user {
                  login
                }
                name
                email
              }
            }
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
}`

// responseType holds the graphsql response. It must match the query structure.
type responseType struct {
	Repository struct {
		Name   string `json:"nameWithOwner"`
		Object struct {
			History struct {
				PageInfo struct {
					EndCursor   string
					HasNextPage bool
				}
				Nodes []struct {
					Sha             string `json:"oid"`
					Timestamp       string `json:"committedDate"`
					MessageHeadline string
					Authors         struct {
						Nodes []struct {
							Email string
							Name  string
							User  struct {
								Login string
							}
						}
					}
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

func fetchCommits(ctx context.Context, graphqlClient *graphql.Client, token string, timestamp string) ([]responseType, error) {
	responses := []responseType{}

	cursor := ""

	for {
		var resp responseType

		req := graphql.NewRequest(query)
		req.Header.Add("Authorization", "token "+token)

		// Empiracally, we can always have timestamp. The
		// after cursor still has the desired effect.
		req.Var("timestamp", timestamp)

		// Set pagination
		if cursor != "" {
			req.Var("after", cursor)
		}

		if err := graphqlClient.Run(ctx, req, &resp); err != nil {
			return nil, err
		}

		responses = append(responses, resp)

		//fmt.Printf("Got %d entries\n", len(resp.Repository.Object.History.Nodes))

		if !resp.Repository.Object.History.PageInfo.HasNextPage {
			break
		}

		cursor = resp.Repository.Object.History.PageInfo.EndCursor
	}

	return responses, nil
}
