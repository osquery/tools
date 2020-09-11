package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/davecgh/go-spew/spew"
)

type requestBodyType struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type graphResponseType struct {
	Data   interface{}
	Errors []graphErr
}

func Request(ctx context.Context, query string, variables map[string]interface{}) error {
	requestBodyObj := requestBodyType{
		Query:     query,
		Variables: variables,
	}

	var requestBody bytes.Buffer
	if err := json.NewEncoder(&requestBody).Encode(requestBodyObj); err != nil {
		return fmt.Errorf("encode body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", &requestBody)
	if err != nil {
		return fmt.Errorf("create http request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Authorization", os.Getenv("GITHUB_TOKEN"))

	res, err := c.httpClient.Do(r)
	if err != nil {
		return fmt.Errorf("fetch http request: %v", err)
	}
	defer res.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, res.Body); err != nil {
		return fmt.Errorf("reading body: %v", err)
	}

	gr := &graphResponseType{
		//Data: interface{},
	}

	if err := json.NewDecoder(&buf).Decode(&gr); err != nil {
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("graphql: server returned a non-200 status code: %v", res.StatusCode)
		}
		return fmt.Errorf("decoding response: %v", err)
	}
	if len(gr.Errors) > 0 {
		// return first error
		return gr.Errors[0]
	}
	return nil

	spew.Dump(gr)
}
